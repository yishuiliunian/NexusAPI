# shellcheck shell=bash
# compose.sh — docker compose 生命周期 + 健康等待。
#
# 所有命令都隐式吃 SCRIPT_DIR/.env，COMPOSE_PROJECT_NAME 已经在那里。
# wait_for_postgres 使用容器内的 pg_isready，避免 host 未装 psql 客户端。

docker_compose_up() {
    info "启动基础设施容器（postgres / redis / upstream-mock / adminer）..."
    (cd "$SCRIPT_DIR" && docker compose up -d) || {
        error "docker compose up 失败"
        return 1
    }
    success "容器已启动"
}

docker_compose_down() {
    info "停止并清理容器与卷..."
    (cd "$SCRIPT_DIR" && docker compose down -v) || true
    success "容器已清理"
}

# wait_for_postgres 用容器内 pg_isready 探测，最多等 max 秒。
wait_for_postgres() {
    local max="${1:-30}"
    info "等待 postgres ready..."
    for ((i=1; i<=max; i++)); do
        if (cd "$SCRIPT_DIR" && docker compose exec -T postgres \
            pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" -q) 2>/dev/null; then
            success "postgres ready"
            return 0
        fi
        sleep 1
    done
    error "postgres 健康检查超时（${max}s）"
    return 1
}

# wait_for_upstream upstream-mock 的健康端点是 /healthz。
wait_for_upstream() {
    local max="${1:-20}"
    info "等待 upstream-mock ready..."
    for ((i=1; i<=max; i++)); do
        if curl -sf "http://127.0.0.1:${UPSTREAM_PORT}/healthz" >/dev/null 2>&1; then
            success "upstream-mock ready (:${UPSTREAM_PORT})"
            return 0
        fi
        sleep 1
    done
    error "upstream-mock 健康检查超时（${max}s）"
    return 1
}

# shellcheck shell=bash
# lifecycle.sh — 全栈生命周期编排（seed / clean / status）。

# run_seed 调 bazel run //backend/cmd/e2e-seed 灌入 admin/user/channel/prices/redemption/plan。
# --reset 清空业务表，保证 dev.sh 反复跑时种子幂等。
run_seed() {
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    local repo_root="$SCRIPT_DIR/../.."
    info "种子数据（admin / user / channel / prices / redemption / plan）..."
    (
        cd "$repo_root"
        bazel run //backend/cmd/e2e-seed -- \
            --postgres-dsn "postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable" \
            --upstream-url "http://127.0.0.1:${UPSTREAM_PORT}" \
            --reset
    ) || {
        error "seed 失败"
        return 1
    }
    success "种子完成"
}

# show_result 打印关键 URL，方便复制到浏览器。
show_result() {
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    echo "" >&2
    echo "  ════════ NexusAPI Dev Ready ════════" >&2
    echo "  Worktree:    $WORKTREE_NAME (offset $PORT_OFFSET)" >&2
    echo "  Backend:     http://127.0.0.1:${BACKEND_PORT}" >&2
    echo "  Web User:    http://127.0.0.1:${WEB_USER_PORT}" >&2
    echo "  Web Admin:   http://127.0.0.1:${WEB_ADMIN_PORT}" >&2
    echo "  Upstream:    http://127.0.0.1:${UPSTREAM_PORT}" >&2
    echo "  Adminer:     http://127.0.0.1:${ADMINER_PORT}  (server=postgres, db=${POSTGRES_DB}, user=${POSTGRES_USER})" >&2
    echo "  Postgres:    127.0.0.1:${POSTGRES_PORT}" >&2
    echo "  Redis:       127.0.0.1:${REDIS_PORT}" >&2
    echo "" >&2
    echo "  Seed accounts:" >&2
    echo "    admin@e2e.test / admin12345" >&2
    echo "    alice@e2e.test / user12345" >&2
    echo "" >&2
    echo "  日志: deploy/dev/runtime/<service>/<service>.log" >&2
    echo "  停止: ./deploy/dev/dev.sh --clean" >&2
    echo "  ════════════════════════════════════" >&2
}

# clean 全量清理（host 进程 + docker 容器卷 + .env + runtime）。
clean() {
    info "清理 dev 环境..."
    stop_host_services
    docker_compose_down
    rm -rf "$(_runtime_dir)"
    rm -f "$ENV_FILE"
    rm -f "$SCRIPT_DIR/../../web-user/.env.local"
    rm -f "$SCRIPT_DIR/../../web-admin/.env.local"
    success "已清理完毕"
}

print_usage() {
    cat >&2 <<EOF
NexusAPI dev environment

Usage:
  ./deploy/dev/dev.sh                 启动完整 dev 环境（infra + backend + 双前端）
  ./deploy/dev/dev.sh --backend-only  只起 infra + backend（给 CI / 跑 e2e 用）
  ./deploy/dev/dev.sh --clean         清理所有进程 / 容器 / 卷 / .env
  ./deploy/dev/dev.sh --help          本帮助

Worktree 隔离：
  每个 git worktree 自动获得一个端口段（main → 20000+，其他 → 哈希分配），
  和独立的 COMPOSE_PROJECT_NAME，多个 worktree 可并行跑 dev 环境互不干扰。
EOF
}

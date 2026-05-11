# shellcheck shell=bash
# host_services.sh — host 端业务进程启停（backend / web-user / web-admin）。
#
# 进程都走 `bazel run`，与项目其它地方保持一致（root package.json scripts
# 也是 `bazel run` 入口）。bazel run 的好处：
#   - rules_go 增量编译比 go run 更快
#   - rules_js + next 用 .next/cache 共享，热重载稳定
#   - 全仓库构建口径一致，CI / 本地相同
#
# bazel run 会拉起 bazel server 守护 + 子进程；停掉时用 setsid 启动给一个
# 独立 pgid，再 `kill -- -<pgid>` 群组杀整链，避免孤儿进程。
#
# 进程信息（pid / pgid / log）落 runtime/<service>/。

_wait_http() {
    local url="$1" name="$2" max="${3:-60}"
    for ((i=1; i<=max; i++)); do
        if curl -sf "$url" >/dev/null 2>&1; then return 0; fi
        sleep 1
    done
    error "$name 健康检查超时 ($url)"
    return 1
}

# _launch 后台启动一个进程：
#   - 用 perl 调 POSIX::setsid 给一个新 pgid（macOS 没有 util-linux 的 setsid 命令，
#     但 macOS 自带 perl 5.x，POSIX::setsid 是 POSIX 标准接口）
#   - 写 pid / log 到 runtime/<name>/；setsid 后 pgid == pid，停止时按 pgid 群组杀
#   - 旧 pgid 文件存在且进程仍活则先 TERM 掉，保证幂等
# 参数：name svc_cwd cmd [args...]
_launch() {
    local name="$1" svc_cwd="$2"; shift 2
    local rt_dir
    rt_dir="$(_runtime_dir)/$name"
    mkdir -p "$rt_dir"
    local pid_file="$rt_dir/$name.pid"
    local log_file="$rt_dir/$name.log"

    # 先清旧
    _kill_pid_tree "$pid_file" "$name" >/dev/null 2>&1 || true
    rm -f "$pid_file"

    info "启动 $name (cwd: $svc_cwd, cmd: $*)"
    (
        cd "$svc_cwd"
        # perl POSIX::setsid 跨 macOS / Linux 都有效；setsid 之后子进程的 pgid 等于 pid，
        # 停止时 kill -- -<pid> 一次性收掉整条 bazel + go/next 子进程链。
        perl -e 'use POSIX qw(setsid); setsid or die; exec @ARGV' \
            sh -c "exec \"\$@\" >\"$log_file\" 2>&1" sh "$@" &
        local pid=$!
        echo "$pid" > "$pid_file"
        disown
    )
}

# _kill_pid_tree 按 pid 文件群组杀进程树；额外按 listening port 兜底。
# 单靠 pgid 不够稳：bazel run 经 rules_go wrapper 会 fork+exec 出新 binary，
# 这个 binary 可能脱离原 process group（PPID=1，PGID 重置），群组杀打不到。
# 所以再用 lsof 找端口持有者作为权威，二次 kill。
_kill_pid_tree() {
    local pid_file="$1" name="${2:-?}"

    # 1) 群组杀（命中 perl→sh→bazel client；可能不命中 detach 的 binary）
    if [[ -f "$pid_file" ]]; then
        local pid
        pid=$(cat "$pid_file")
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            info "停止 $name (pid=$pid, group)"
            kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
            sleep 1
            kill -KILL -- "-$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null || true
        fi
        rm -f "$pid_file"
    fi

    # 2) 端口兜底：用 worktree 端口段确定 service 端口，杀掉端口持有者
    local port
    case "$name" in
        backend)   port="${BACKEND_PORT:-}" ;;
        web-user)  port="${WEB_USER_PORT:-}" ;;
        web-admin) port="${WEB_ADMIN_PORT:-}" ;;
        *) port="" ;;
    esac
    if [[ -n "$port" ]]; then
        local holders
        holders=$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null || true)
        if [[ -n "$holders" ]]; then
            info "停止 $name 端口持有者 (:$port pids=$holders)"
            # shellcheck disable=SC2086
            kill -TERM $holders 2>/dev/null || true
            sleep 1
            # shellcheck disable=SC2086
            kill -KILL $holders 2>/dev/null || true
        fi
    fi
}

# start_backend_host 启动 Go backend：`bazel run //backend/cmd/server`。
# 注入完整 NEXUSAPI_* 环境变量。
start_backend_host() {
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    local repo_root="$SCRIPT_DIR/../.."

    export NEXUSAPI_APP_ENV=development
    export NEXUSAPI_SERVER_HOST=127.0.0.1
    export NEXUSAPI_SERVER_PORT="$BACKEND_PORT"
    export NEXUSAPI_DATABASE_DRIVER=postgres
    export NEXUSAPI_DATABASE_DSN="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable"
    export NEXUSAPI_REDIS_ADDR="127.0.0.1:${REDIS_PORT}"
    export NEXUSAPI_LOG_LEVEL=info
    export NEXUSAPI_LOG_FORMAT=console
    export NEXUSAPI_SECURITY_ENCRYPTION_KEY=""
    export NEXUSAPI_SITE_BASE_URL="http://127.0.0.1:${WEB_USER_PORT}"

    # OAuth：全部指向 upstream-mock，让 e2e 的 github/google 流程跑通。
    # 不暴露真实 ClientID/Secret；mock 端不校验。
    export NEXUSAPI_OAUTH_POST_LOGIN_URL="http://127.0.0.1:${WEB_USER_PORT}/dashboard"
    export NEXUSAPI_OAUTH_GITHUB_ENABLED=true
    export NEXUSAPI_OAUTH_GITHUB_CLIENT_ID=mock-gh-id
    export NEXUSAPI_OAUTH_GITHUB_CLIENT_SECRET=mock-gh-secret
    export NEXUSAPI_OAUTH_GITHUB_AUTHORIZE_URL="http://127.0.0.1:${UPSTREAM_PORT}/oauth/github/authorize"
    export NEXUSAPI_OAUTH_GITHUB_TOKEN_URL="http://127.0.0.1:${UPSTREAM_PORT}/oauth/github/token"
    export NEXUSAPI_OAUTH_GITHUB_API_BASE="http://127.0.0.1:${UPSTREAM_PORT}"
    export NEXUSAPI_OAUTH_GOOGLE_ENABLED=true
    export NEXUSAPI_OAUTH_GOOGLE_CLIENT_ID=mock-gg-id
    export NEXUSAPI_OAUTH_GOOGLE_CLIENT_SECRET=mock-gg-secret
    export NEXUSAPI_OAUTH_GOOGLE_AUTHORIZE_URL="http://127.0.0.1:${UPSTREAM_PORT}/oauth/google/authorize"
    export NEXUSAPI_OAUTH_GOOGLE_TOKEN_URL="http://127.0.0.1:${UPSTREAM_PORT}/oauth/google/token"
    export NEXUSAPI_OAUTH_GOOGLE_API_BASE="http://127.0.0.1:${UPSTREAM_PORT}/oauth/google/userinfo"

    # Stripe mock：让 payment-subscription e2e 跑通 webhook 路径（不接真 Stripe）
    export NEXUSAPI_PAYMENT_STRIPE_ENABLED=true
    export NEXUSAPI_PAYMENT_STRIPE_SECRET_KEY=sk_test_e2e_fake
    export NEXUSAPI_PAYMENT_STRIPE_WEBHOOK_SECRET=whsec_e2e_fixed
    export NEXUSAPI_PAYMENT_STRIPE_SUCCESS_URL="http://127.0.0.1:${WEB_USER_PORT}/billing?paid=1"
    export NEXUSAPI_PAYMENT_STRIPE_CANCEL_URL="http://127.0.0.1:${WEB_USER_PORT}/billing?canceled=1"
    export NEXUSAPI_PAYMENT_STRIPE_PRODUCT_NAME="NexusAPI Dev Credits"
    export NEXUSAPI_PAYMENT_STRIPE_API_BASE="http://127.0.0.1:${UPSTREAM_PORT}"
    export NEXUSAPI_PAYMENT_MICRO_PER_CENT=10000

    # 限流默认值
    export NEXUSAPI_RATE_LIMIT_DEFAULT_RPM=1000
    export NEXUSAPI_RATE_LIMIT_DEFAULT_TPM=0
    export NEXUSAPI_RELAY_FAILOVER_ATTEMPTS=1
    export NEXUSAPI_AUTH_SESSION_TTL_HOURS=24

    _launch backend "$repo_root" bazel run //backend/cmd/server

    if ! _wait_http "http://127.0.0.1:${BACKEND_PORT}/healthz" backend 180; then
        error "Backend 启动失败，查看日志: $(_runtime_dir)/backend/backend.log"
        tail -40 "$(_runtime_dir)/backend/backend.log" >&2 2>/dev/null || true
        return 1
    fi
    success "Backend 已就绪 (:$BACKEND_PORT)"
}

# start_web_user / start_web_admin: `bazel run //web-user:next_dev` 等。
start_web_user() {
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    local repo_root="$SCRIPT_DIR/../.."

    export NEXUSAPI_BACKEND_URL="http://127.0.0.1:${BACKEND_PORT}"
    _launch web-user "$repo_root" \
        bazel run //web-user:next_dev -- --port "$WEB_USER_PORT"

    if ! _wait_http "http://127.0.0.1:${WEB_USER_PORT}/login" web-user 120; then
        warn "web-user 健康端点未响应（可能仍在编译），日志: $(_runtime_dir)/web-user/web-user.log"
    else
        success "web-user 已就绪 (:$WEB_USER_PORT)"
    fi
}

start_web_admin() {
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    local repo_root="$SCRIPT_DIR/../.."

    export NEXUSAPI_BACKEND_URL="http://127.0.0.1:${BACKEND_PORT}"
    _launch web-admin "$repo_root" \
        bazel run //web-admin:next_dev -- --port "$WEB_ADMIN_PORT"

    if ! _wait_http "http://127.0.0.1:${WEB_ADMIN_PORT}/login" web-admin 120; then
        warn "web-admin 健康端点未响应（可能仍在编译），日志: $(_runtime_dir)/web-admin/web-admin.log"
    else
        success "web-admin 已就绪 (:$WEB_ADMIN_PORT)"
    fi
}

# stop_host_services 群组杀所有 host 进程。lifecycle.sh clean 会调。
# source .env 让 _kill_pid_tree 能拿到端口（用于按端口兜底杀进程）。
stop_host_services() {
    # shellcheck disable=SC1090
    [[ -f "$ENV_FILE" ]] && source "$ENV_FILE"
    local rt_root
    rt_root="$(_runtime_dir)"
    [[ -d "$rt_root" ]] || return 0
    for svc in backend web-user web-admin; do
        _kill_pid_tree "$rt_root/$svc/$svc.pid" "$svc"
    done
}

# is_backend_running 探测 backend 是否已经在跑（给 e2e global-setup 用）。
is_backend_running() {
    # shellcheck disable=SC1090
    source "$ENV_FILE" 2>/dev/null
    [[ -n "${BACKEND_PORT:-}" ]] || return 1
    curl -sf "http://127.0.0.1:${BACKEND_PORT}/healthz" >/dev/null 2>&1
}

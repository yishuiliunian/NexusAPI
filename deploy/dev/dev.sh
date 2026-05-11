#!/usr/bin/env bash
# =============================================================================
# NexusAPI dev environment — entry point.
# =============================================================================
#
# 一键拉起本地 dev：
#   ./dev.sh                  完整：docker infra + host backend + 双前端
#   ./dev.sh --backend-only   仅 infra + backend（e2e/CI 用）
#   ./dev.sh --clean          全部清理
#   ./dev.sh --help           帮助
#
# 同一个仓库的多个 git worktree 可以并行跑：每个 worktree 用哈希分配的端口段，
# docker compose project 也带 worktree 后缀，互不冲突。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/.env"

# 依赖顺序：log（叶子）→ worktree → config_gen / compose / host_services → lifecycle（编排者）
# shellcheck source=lib/log.sh
source "$SCRIPT_DIR/lib/log.sh"
# shellcheck source=lib/worktree.sh
source "$SCRIPT_DIR/lib/worktree.sh"
# shellcheck source=lib/config_gen.sh
source "$SCRIPT_DIR/lib/config_gen.sh"
# shellcheck source=lib/compose.sh
source "$SCRIPT_DIR/lib/compose.sh"
# shellcheck source=lib/host_services.sh
source "$SCRIPT_DIR/lib/host_services.sh"
# shellcheck source=lib/lifecycle.sh
source "$SCRIPT_DIR/lib/lifecycle.sh"

main() {
    cd "$SCRIPT_DIR"

    case "${1:-}" in
        --clean|-c)
            clean
            exit 0
            ;;
        --help|-h)
            print_usage
            exit 0
            ;;
    esac

    local backend_only=false
    [[ "${1:-}" == "--backend-only" ]] && backend_only=true

    print_banner

    # Phase 1: configs（无 docker）
    generate_env
    generate_web_env

    # Phase 2: docker infra + 健康等待
    docker_compose_up
    wait_for_postgres
    wait_for_upstream

    # Phase 3: backend（依赖 postgres + redis ready）+ seed
    start_backend_host
    run_seed

    # Phase 4: frontends（CI/e2e 可选跳过）
    if [[ "$backend_only" == "true" ]]; then
        info "--backend-only：跳过前端启动"
    else
        start_web_user
        start_web_admin
    fi

    show_result
}

main "$@"

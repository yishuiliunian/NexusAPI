#!/usr/bin/env bash
# clean_runner.sh — `bazel run //e2e:clean` 入口。
#
# 步骤：
#   1. 杀 e2e 启动留下的前端进程（按 .env 里的 WEB_USER_PORT / WEB_ADMIN_PORT）
#   2. 调 bazel run //deploy/dev:clean 清掉 backend/docker/.env

set -euo pipefail

if [[ -z "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then
    echo "ERROR: BUILD_WORKSPACE_DIRECTORY 未设置 — 请用 'bazel run //e2e:clean' 调用。" >&2
    exit 1
fi
cd "$BUILD_WORKSPACE_DIRECTORY"

ENV_FILE="deploy/dev/.env"

# 1) 端口杀前端 —— .env 可能不存在（dev 没起过），那就走默认 20002/20003
WEB_USER_PORT="20002"
WEB_ADMIN_PORT="20003"
if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$ENV_FILE"
fi

for port in "$WEB_USER_PORT" "$WEB_ADMIN_PORT"; do
    holders=$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null || true)
    if [[ -n "$holders" ]]; then
        echo "[e2e:clean] 杀掉 :$port 端口持有者 (pids=$holders)"
        # shellcheck disable=SC2086
        kill -9 $holders 2>/dev/null || true
    fi
done

# 2) 委托 deploy/dev:clean 完成 backend/docker/.env 清理
exec bazel run //deploy/dev:clean

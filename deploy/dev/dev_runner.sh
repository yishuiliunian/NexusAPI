#!/usr/bin/env bash
# dev_runner.sh — `bazel run //deploy/dev:up` 与同族 target 的入口。
#
# `bazel run` 把 cwd 设到 runfiles 树（只读副本），但 dev.sh 必须在源码树里跑：
#   - 要写 .env（gitignored）
#   - 要让 docker compose 拿到正确的 build context
#   - 要让 lib/*.sh 内的 SCRIPT_DIR 指向源码树
# `bazel run` 会设置 BUILD_WORKSPACE_DIRECTORY 为源码树根路径，借此切回去。

set -euo pipefail

if [[ -z "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then
    echo "ERROR: BUILD_WORKSPACE_DIRECTORY 未设置 — 请用 'bazel run //deploy/dev:up' 调用，而非直接执行此脚本。" >&2
    exit 1
fi

cd "$BUILD_WORKSPACE_DIRECTORY/deploy/dev"
exec bash dev.sh "$@"

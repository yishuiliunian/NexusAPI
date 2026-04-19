#!/bin/bash
# Bazel workspace status script
#
# 被 bazel build --stamp 调用，输出 key-value 对用于 OCI 镜像标签等场景。
# 输出：STABLE_* 变更会触发重新构建；其他 key 不会。
#
# 参考：https://bazel.build/docs/user-manual#workspace-status

set -euo pipefail

# Git commit hash（短）
GIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_SHA_LONG=$(git rev-parse HEAD 2>/dev/null || echo "unknown")

# Git branch
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

# Git 是否有未提交修改
GIT_DIRTY="clean"
if [ -n "$(git status --porcelain 2>/dev/null || true)" ]; then
  GIT_DIRTY="dirty"
fi

# 构建时间（ISO 8601 UTC）
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# 构建用户
BUILD_USER=$(whoami)

# ---- Stable keys（影响缓存）----
echo "STABLE_GIT_SHA ${GIT_SHA}"
echo "STABLE_GIT_SHA_LONG ${GIT_SHA_LONG}"
echo "STABLE_GIT_BRANCH ${GIT_BRANCH}"
echo "STABLE_GIT_DIRTY ${GIT_DIRTY}"

# ---- Volatile keys（不影响缓存）----
echo "BUILD_TIME ${BUILD_TIME}"
echo "BUILD_USER ${BUILD_USER}"

# shellcheck shell=bash
# worktree.sh — git worktree 探测 + 哈希端口分配。
#
# 设计目标：同一个仓库的多个 git worktree 可以同时跑 dev 环境，
# 端口不冲突、docker-compose project 不冲突、.env 不冲突。
#
# 端口分配规则：
#   port = base_slot + offset * 50
#   main / master worktree → offset = 0
#   其他 worktree → md5(name) % 500 + 1
#   步长 50，覆盖 [20000, 45000)，可容纳 500 个并发 worktree。

# get_worktree_name 返回当前所在 worktree 的规范化名字。
# - 在 worktree 里：git_dir 形如 `<repo>/.git/worktrees/<name>`，取 basename
# - 在主 checkout：取当前 branch 名（main / master / feature-xxx）
# - 非法字符全部替换为 -，统一小写
get_worktree_name() {
    local git_dir
    git_dir=$(git rev-parse --git-dir 2>/dev/null)

    if [[ "$git_dir" == *".git/worktrees/"* ]]; then
        basename "$git_dir"
    else
        git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "main"
    fi | sed 's/[^a-zA-Z0-9-]/-/g' | tr '[:upper:]' '[:lower:]'
}

# calculate_port_offset 根据 worktree 名计算端口偏移量。
# main / master 固定 0；其他用 md5 前 6 位 hex 取模 500 再 +1。
# 兼容 macOS（md5）与 Linux（md5sum）。
calculate_port_offset() {
    local name="$1"
    if [[ "$name" == "main" || "$name" == "master" ]]; then
        echo 0
        return
    fi
    local hash
    if command -v md5sum &>/dev/null; then
        hash=$(echo -n "$name" | md5sum | cut -c1-6)
    else
        hash=$(echo -n "$name" | md5 | cut -c1-6)
    fi
    echo $(( (16#$hash % 500) + 1 ))
}

# _runtime_dir 返回 worktree 内的可变运行时目录（pid / log 等），
# gitignored。所有调用方共用同一个路径。
_runtime_dir() { echo "$SCRIPT_DIR/runtime"; }

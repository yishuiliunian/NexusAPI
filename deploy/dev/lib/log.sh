# shellcheck shell=bash
# log.sh — 终端彩色日志输出。
#
# 提供 info / success / warn / error / banner，所有输出走 stderr，
# 让 stdout 留给真正需要被脚本捕获的内容。

_NC='\033[0m'
_RED='\033[0;31m'
_GREEN='\033[0;32m'
_YELLOW='\033[0;33m'
_BLUE='\033[0;34m'
_CYAN='\033[0;36m'
_BOLD='\033[1m'

info()    { echo -e "${_CYAN}ℹ${_NC} $*" >&2; }
success() { echo -e "${_GREEN}✓${_NC} $*" >&2; }
warn()    { echo -e "${_YELLOW}⚠${_NC} $*" >&2; }
error()   { echo -e "${_RED}✗${_NC} $*" >&2; }

print_banner() {
    echo -e "${_BOLD}${_BLUE}" >&2
    echo "  ╔═══════════════════════════════════════╗" >&2
    echo "  ║   NexusAPI · Dev Environment Boot     ║" >&2
    echo "  ╚═══════════════════════════════════════╝" >&2
    echo -e "${_NC}" >&2
}

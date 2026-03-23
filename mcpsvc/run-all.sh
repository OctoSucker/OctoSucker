#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -f "$ROOT/.env" ]]; then
	set -a
	# shellcheck disable=SC1091
	source "$ROOT/.env"
	set +a
fi

TELEGRAM_ADDR="${MCP_TELEGRAM_LISTEN:-:8765}"
EXEC_ADDR="${MCP_EXEC_LISTEN:-:8766}"

mcp_http_base() {
	local a="$1"
	if [[ "$a" == http://* ]]; then
		printf '%s' "$a"
	elif [[ "$a" == :* ]]; then
		printf 'http://127.0.0.1:%s' "${a#:}"
	elif [[ "$a" == *:* ]]; then
		printf 'http://%s' "$a"
	else
		printf 'http://127.0.0.1:%s' "$a"
	fi
}

pids=()
cleanup() {
	local pid
	for pid in "${pids[@]+"${pids[@]}"}"; do
		kill "$pid" 2>/dev/null || true
	done
}

trap cleanup EXIT
trap 'cleanup; trap - EXIT; exit 130' INT TERM

started=0

if [[ -n "${MCP_RUN_ALL_SKIP_TELEGRAM:-}" ]]; then
	echo "mcp-run-all: skip mcp-telegram (MCP_RUN_ALL_SKIP_TELEGRAM set)" >&2
elif [[ -z "${TELEGRAM_BOT_TOKEN:-}" ]]; then
	echo "mcp-run-all: skip mcp-telegram (set TELEGRAM_BOT_TOKEN or export MCP_RUN_ALL_SKIP_TELEGRAM=1)" >&2
else
	echo "mcp-run-all: mcp-telegram -> go run ... -listen $TELEGRAM_ADDR" >&2
	go run ./mcpsvc/telegram/cmd/mcp-telegram -listen "$TELEGRAM_ADDR" &
	pids+=("$!")
	((++started)) || true
fi

if [[ -n "${MCP_RUN_ALL_SKIP_EXEC:-}" ]]; then
	echo "mcp-run-all: skip mcp-exec (MCP_RUN_ALL_SKIP_EXEC set)" >&2
elif [[ -z "${EXEC_WORKSPACE_DIRS:-}" ]]; then
	echo "mcp-run-all: skip mcp-exec (set EXEC_WORKSPACE_DIRS or MCP_RUN_ALL_SKIP_EXEC=1)" >&2
else
	echo "mcp-run-all: mcp-exec -> go run ... -listen $EXEC_ADDR" >&2
	go run ./mcpsvc/exec/cmd/mcp-exec -listen "$EXEC_ADDR" &
	pids+=("$!")
	((++started)) || true
fi

if [[ "$started" -eq 0 ]]; then
	echo "mcp-run-all: nothing to run; configure env or unset SKIP flags" >&2
	exit 1
fi

echo "mcp-run-all: pids ${pids[*]} — Ctrl+C stops all. Agent example:" >&2
TG_URL="$(mcp_http_base "$TELEGRAM_ADDR")"
echo "  export OCTOPLUS_MCP_ENDPOINT=$TG_URL" >&2
if [[ "$started" -gt 1 ]]; then
	EX_URL="$(mcp_http_base "$EXEC_ADDR")"
	echo "  # multi: OCTOPLUS_MCP_ENDPOINT=$TG_URL,$EX_URL" >&2
fi

rc=0
for pid in "${pids[@]}"; do
	wait "$pid" || rc=$?
done
trap - EXIT
exit "$rc"

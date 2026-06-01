#!/usr/bin/env bash
# SubagentStop hook: when ANY subagent finishes, move the Multica issue created
# at SubagentStart (correlated by agent_id) to in_review. No-op if we never
# created an issue for this agent_id.
# Best-effort: never blocks the agent (always exits 0).
set -uo pipefail

INPUT="$(cat)"

AGENT_ID="$(printf '%s' "$INPUT" | jq -r '.agent_id // empty')"
CWD="$(printf '%s' "$INPUT" | jq -r '.cwd // empty')"
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-${CWD:-$PWD}}"
MAP_FILE="$PROJECT_DIR/.claude/.multica-issuemap/$AGENT_ID"

[ -n "$AGENT_ID" ] && [ -f "$MAP_FILE" ] || exit 0
ISSUE_ID="$(cat "$MAP_FILE" 2>/dev/null)"
[ -n "$ISSUE_ID" ] || exit 0

multica issue status "$ISSUE_ID" in_review >/dev/null 2>&1 || true
rm -f "$MAP_FILE" 2>/dev/null || true

exit 0

#!/usr/bin/env bash
# SubagentStart hook: when ANY subagent is dispatched, create an in_progress
# issue in the local Multica and remember its id so the matching SubagentStop
# can (a) set the exact task title and (b) move it to in_review.
#
# Why the title is provisional here: at SubagentStart the dispatched task is NOT
# reliably available. Claude Code 2.1.x does NOT include it in the hook payload
# (`task_description` is null — verified empirically), and the parent's Task/Agent
# tool_use is usually not yet flushed to the transcript .jsonl, so scanning the
# transcript here finds nothing — or worse, a STALE match from an earlier
# same-type dispatch. So we create with a provisional title and let SubagentStop
# fill the exact title by matching agent_id in the (by-then-flushed) tool_result.
# Best-effort: never blocks the agent (always exits 0).
set -uo pipefail

INPUT="$(cat)"

AGENT_TYPE="$(printf '%s' "$INPUT" | jq -r '.agent_type // empty')"
[ -n "$AGENT_TYPE" ] || exit 0

AGENT_ID="$(printf '%s' "$INPUT" | jq -r '.agent_id // empty')"
SESSION_ID="$(printf '%s' "$INPUT" | jq -r '.session_id // empty')"
CWD="$(printf '%s' "$INPUT" | jq -r '.cwd // empty')"

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-${CWD:-$PWD}}"
ASSIGNEE_ID="61c0928a-aee6-4230-ba27-46c7f5d12616"   # BLIPS IFRS9 agent
MAP_DIR="$PROJECT_DIR/.claude/.multica-issuemap"
mkdir -p "$MAP_DIR" 2>/dev/null || true

# Future-proof: if a future Claude Code version starts sending the task in the
# payload, use it for an immediate accurate title (currently null on 2.1.x).
TASK="$(printf '%s' "$INPUT" | jq -r '.task_description // empty')"

STAMP="$(date '+%Y-%m-%d %H:%M %Z')"
if [ -n "$TASK" ]; then
  TITLE="[$AGENT_TYPE] $(printf '%s' "$TASK" | tr '\n' ' ' | head -c 80)"
else
  TITLE="[$AGENT_TYPE] run ${AGENT_ID:0:8}"   # provisional; SubagentStop sets exact title
fi

DESC="Auto-created by SubagentStart hook on subagent dispatch.

subagent: ${AGENT_TYPE}
Task: ${TASK:-(pending — finalized by SubagentStop)}
agent_id: ${AGENT_ID}
session_id: ${SESSION_ID}
cwd: ${CWD}
started: ${STAMP}"

ISSUE_JSON="$(multica issue create \
  --title "$TITLE" \
  --description "$DESC" \
  --status in_progress \
  --assignee-id "$ASSIGNEE_ID" \
  --output json 2>/dev/null)" || exit 0

ISSUE_ID="$(printf '%s' "$ISSUE_JSON" | jq -r '.id // .issue.id // empty' 2>/dev/null)"
if [ -n "$ISSUE_ID" ] && [ -n "$AGENT_ID" ]; then
  printf '%s' "$ISSUE_ID" > "$MAP_DIR/$AGENT_ID"
fi

exit 0

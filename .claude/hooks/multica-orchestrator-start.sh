#!/usr/bin/env bash
# SubagentStart hook: when ANY subagent is dispatched, create an in_progress
# issue in the local Multica and remember its id so the matching SubagentStop
# can move it to in_review.
# Best-effort: never blocks the agent (always exits 0).
set -uo pipefail

INPUT="$(cat)"

AGENT_TYPE="$(printf '%s' "$INPUT" | jq -r '.agent_type // empty')"
[ -n "$AGENT_TYPE" ] || exit 0

AGENT_ID="$(printf '%s' "$INPUT" | jq -r '.agent_id // empty')"
SESSION_ID="$(printf '%s' "$INPUT" | jq -r '.session_id // empty')"
CWD="$(printf '%s' "$INPUT" | jq -r '.cwd // empty')"
TRANSCRIPT="$(printf '%s' "$INPUT" | jq -r '.transcript_path // empty')"

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-${CWD:-$PWD}}"
ASSIGNEE_ID="61c0928a-aee6-4230-ba27-46c7f5d12616"   # BLIPS IFRS9 agent
MAP_DIR="$PROJECT_DIR/.claude/.multica-issuemap"
mkdir -p "$MAP_DIR" 2>/dev/null || true

# Best-effort: recover the task description handed to this subagent. The
# transcript_path given at SubagentStart often does NOT contain the parent's
# Task/Agent tool_use (it may be the subagent's own fresh transcript), so try
# that file first, then scan the project's other transcripts newest-first.
jq_extract_task() {  # $1 = transcript file
  jq -rs --arg at "$AGENT_TYPE" '
    [ .[] | (.message.content? // []) | .[]?
      | select(.type=="tool_use" and (.name=="Task" or .name=="Agent")
               and (.input.subagent_type==$at))
      | (.input.description // .input.prompt // empty) ]
    | last // empty' "$1" 2>/dev/null
}

TASK=""
SEARCH=""
[ -n "$TRANSCRIPT" ] && [ -f "$TRANSCRIPT" ] && SEARCH="$TRANSCRIPT"
if [ -n "$TRANSCRIPT" ]; then
  TDIR="$(dirname "$TRANSCRIPT" 2>/dev/null)"
  [ -d "$TDIR" ] && SEARCH="$SEARCH $(ls -t "$TDIR"/*.jsonl 2>/dev/null)"
fi
# Last resort (empty transcript_path): derive the project transcript dir from cwd.
SAN="$(printf '%s' "$PROJECT_DIR" | sed 's#/#-#g')"
PTDIR="$HOME/.claude/projects/$SAN"
[ -d "$PTDIR" ] && SEARCH="$SEARCH $(ls -t "$PTDIR"/*.jsonl 2>/dev/null)"
for f in $SEARCH; do
  [ -f "$f" ] || continue
  TASK="$(jq_extract_task "$f")"
  [ -n "$TASK" ] && break
done

STAMP="$(date '+%Y-%m-%d %H:%M %Z')"
if [ -n "$TASK" ]; then
  TITLE="[$AGENT_TYPE] $(printf '%s' "$TASK" | tr '\n' ' ' | head -c 80)"
else
  TITLE="[$AGENT_TYPE] run ${AGENT_ID:0:8}"
fi

DESC="Auto-created by SubagentStart hook on subagent dispatch.

subagent: ${AGENT_TYPE}
Task: ${TASK:-(not captured from transcript)}
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

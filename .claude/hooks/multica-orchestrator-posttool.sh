#!/usr/bin/env bash
# PostToolUse hook (matcher: Task|Agent): when a subagent-dispatch tool returns,
# set the EXACT title on the Multica issue created at SubagentStart and move it to
# in_review, then drop the agent_id->issue map entry.
#
# Why PostToolUse and not the transcript: the dispatch's task is NOT available
# race-free from either SubagentStart or SubagentStop — at both points the parent
# has not yet flushed the Task/Agent tool_use / tool_result to the transcript
# .jsonl (the hook is synchronous and blocks that write). The PostToolUse payload,
# by contrast, carries everything inline (verified on Claude Code 2.1.x):
#   tool_input.description / tool_input.prompt -> the task handed to the subagent
#   tool_input.subagent_type                   -> agent type for the title prefix
#   tool_response.agentId                      -> == SubagentStart agent_id (the map key)
# Best-effort: never blocks the agent (always exits 0).
set -uo pipefail

INPUT="$(cat)"

# Only act on subagent-dispatch tools (defensive — matcher already scopes this).
TOOL="$(printf '%s' "$INPUT" | jq -r '.tool_name // empty')"
case "$TOOL" in Task|Agent) ;; *) exit 0 ;; esac

AGENT_ID="$(printf '%s' "$INPUT" | jq -r '.tool_response.agentId // .tool_response.agent_id // empty')"
[ -n "$AGENT_ID" ] || exit 0

CWD="$(printf '%s' "$INPUT" | jq -r '.cwd // empty')"
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-${CWD:-$PWD}}"
MAP_FILE="$PROJECT_DIR/.claude/.multica-issuemap/$AGENT_ID"
[ -f "$MAP_FILE" ] || exit 0          # no issue was created for this agent_id
ISSUE_ID="$(cat "$MAP_FILE" 2>/dev/null)"
[ -n "$ISSUE_ID" ] || exit 0

SUBTYPE="$(printf '%s' "$INPUT" | jq -r '.tool_input.subagent_type // .agent_type // "subagent"')"
TASK="$(printf '%s' "$INPUT" | jq -r '.tool_input.description // .tool_input.prompt // .tool_response.prompt // empty')"

STAMP="$(date '+%Y-%m-%d %H:%M %Z')"
if [ -n "$TASK" ]; then
  TITLE="[$SUBTYPE] $(printf '%s' "$TASK" | tr '\n' ' ' | head -c 80)"
  NEWDESC="Auto-created by SubagentStart; title/task finalized by PostToolUse.

subagent: ${SUBTYPE}
Task: ${TASK}
agent_id: ${AGENT_ID}
finished: ${STAMP}"
  multica issue update "$ISSUE_ID" --title "$TITLE" --description "$NEWDESC" --status in_review >/dev/null 2>&1 \
    || multica issue status "$ISSUE_ID" in_review >/dev/null 2>&1 || true
else
  multica issue status "$ISSUE_ID" in_review >/dev/null 2>&1 || true
fi

rm -f "$MAP_FILE" 2>/dev/null || true
exit 0

#!/usr/bin/env bash
#
# Conversation logger plugin for pigo.
# Appends JSONL entries to .pigo/logs/conversation.jsonl
# using PIGO_* environment variables set by the hook system.

set -euo pipefail

LOG_DIR="$HOME/.pigo/logs"
LOG_FILE="${LOG_DIR}/conversation.jsonl"

mkdir -p "${LOG_DIR}"

timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Escape a string for safe JSON embedding
json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  s="${s//$'\r'/\\r}"
  s="${s//$'\t'/\\t}"
  printf '%s' "$s"
}

event="${PIGO_EVENT:-unknown}"

case "${event}" in
  turn_start)
    user_msg=$(json_escape "${PIGO_USER_MESSAGE:-}")
    printf '{"ts":"%s","event":"%s","user_message":"%s"}\n' \
      "$timestamp" "$event" "$user_msg" >> "${LOG_FILE}"
    ;;
  turn_end)
    assistant_msg=$(json_escape "${PIGO_ASSISTANT_MESSAGE:-}")
    printf '{"ts":"%s","event":"%s","assistant_message":"%s"}\n' \
      "$timestamp" "$event" "$assistant_msg" >> "${LOG_FILE}"
    ;;
  tool_start)
    tool_name="${PIGO_TOOL_NAME:-}"
    tool_input=$(json_escape "${PIGO_TOOL_INPUT:-}")
    printf '{"ts":"%s","event":"%s","tool":"%s","input":"%s"}\n' \
      "$timestamp" "$event" "$tool_name" "$tool_input" >> "${LOG_FILE}"
    ;;
  tool_end)
    tool_name="${PIGO_TOOL_NAME:-}"
    tool_output=$(json_escape "${PIGO_TOOL_OUTPUT:-}")
    tool_error=$(json_escape "${PIGO_TOOL_ERROR:-}")
    printf '{"ts":"%s","event":"%s","tool":"%s","output":"%s","error":"%s"}\n' \
      "$timestamp" "$event" "$tool_name" "$tool_output" "$tool_error" >> "${LOG_FILE}"
    ;;
  *)
    printf '{"ts":"%s","event":"%s"}\n' \
      "$timestamp" "$event" >> "${LOG_FILE}"
    ;;
esac

#!/usr/bin/env bash
# check-commit-msg.sh — Validate Conventional Commits format for BLIPS IFRS9.
#
# Dipasang sebagai pre-commit hook via .pre-commit-config.yaml (stage: commit-msg).
# Dibaca oleh: pre-commit framework (file commit message diteruskan sebagai argumen $1).
#
# Format yang diterima:
#   <type>(<scope>): <subject>
#
# Types yang valid:
#   feat, fix, perf, refactor, docs, test, chore, build, revert, breaking
#
# BLIPS scopes yang valid (modul + cross-cutting):
#   app-a, app-b, app-c, app-d, app-e
#   sec, db, api, web, worker, integ, ci, infra, deps, repo
#
# Subject rules:
#   - Imperative mood (add, fix, remove — bukan added/fixes/removing)
#   - Lowercase huruf pertama
#   - Tidak diakhiri titik
#   - Maksimal 72 karakter termasuk type+scope prefix
#
# Referensi: .claude/memory/git-conventions.md

set -euo pipefail

COMMIT_MSG_FILE="${1}"

if [[ -z "${COMMIT_MSG_FILE}" ]]; then
  echo "ERROR: path ke file commit message tidak diberikan." >&2
  exit 1
fi

COMMIT_MSG=$(cat "${COMMIT_MSG_FILE}")

# Abaikan merge commit dan revert commit yang di-generate otomatis git.
if echo "${COMMIT_MSG}" | grep -qE "^(Merge|Revert) "; then
  exit 0
fi

# Abaikan baris komentar (dimulai #).
FIRST_LINE=$(echo "${COMMIT_MSG}" | grep -v "^#" | head -n1)

if [[ -z "${FIRST_LINE}" ]]; then
  echo "ERROR: commit message kosong." >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Pattern: <type>(<scope>): <subject>
# Scope bersifat opsional (feat: tanpa scope masih valid), tapi di BLIPS
# sangat dianjurkan. Hook ini hanya warn untuk missing scope, tidak block.
# ---------------------------------------------------------------------------
TYPES="feat|fix|perf|refactor|docs|test|chore|build|revert|breaking"
SCOPES="app-a|app-b|app-c|app-d|app-e|sec|db|api|web|worker|integ|ci|infra|deps|repo"

# Regex: type(scope): subject  -atau-  type: subject  -atau-  type!: subject (breaking)
PATTERN="^(${TYPES})(\((${SCOPES})\))?!?: .+"

if ! echo "${FIRST_LINE}" | grep -qE "${PATTERN}"; then
  echo ""
  echo "ERROR: Commit message tidak sesuai Conventional Commits BLIPS." >&2
  echo ""
  echo "  Baris pertama: ${FIRST_LINE}" >&2
  echo ""
  echo "  Format yang benar:" >&2
  echo "    <type>(<scope>): <subject>" >&2
  echo ""
  echo "  Types valid  : ${TYPES}" >&2
  echo "  Scopes valid : ${SCOPES}" >&2
  echo ""
  echo "  Contoh bagus :" >&2
  echo "    feat(app-c): implement SICR trigger for Stage 2 transition" >&2
  echo "    fix(sec): correct argon2id salt generation" >&2
  echo "    chore(deps): upgrade minio-go to v7.2.0" >&2
  echo ""
  echo "  Contoh buruk  :" >&2
  echo "    fix bug        (no type, no scope, vague)" >&2
  echo "    Fixed ECL      (past tense, no scope)" >&2
  echo "    wip            (not informative)" >&2
  echo ""
  echo "  Referensi: .claude/memory/git-conventions.md" >&2
  echo ""
  exit 1
fi

# Panjang baris pertama tidak boleh lebih dari 72 karakter.
LINE_LEN=${#FIRST_LINE}
if [[ ${LINE_LEN} -gt 72 ]]; then
  echo ""
  echo "ERROR: Baris pertama commit message terlalu panjang (${LINE_LEN} karakter, max 72)." >&2
  echo "  ${FIRST_LINE}" >&2
  echo ""
  exit 1
fi

# Subject tidak boleh diakhiri titik.
if echo "${FIRST_LINE}" | grep -qE "\.$"; then
  echo ""
  echo "ERROR: Subject commit tidak boleh diakhiri titik ('.')." >&2
  echo "  ${FIRST_LINE}" >&2
  echo ""
  exit 1
fi

# Warning (non-blocking): scope tidak dipakai.
if ! echo "${FIRST_LINE}" | grep -qE "^(${TYPES})\("; then
  echo "WARNING: Scope BLIPS tidak dipakai. Sangat dianjurkan menyertakan scope (mis. feat(app-c): ...)." >&2
fi

exit 0

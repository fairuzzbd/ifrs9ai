---
description: Generate cross-module status / standup report dari git log + plan docs
argument-hint: <range hari, contoh "kemarin" atau "minggu ini">
allowed-tools: Read, Grep, Glob, Bash
---

Generate status report singkat untuk BLIPS IFRS9.

**Range:** $ARGUMENTS

Langkah:
1. `!git log --since="$ARGUMENTS" --oneline --all` — recent commits
2. `!ls -lt docs/plans/ | head -10` — recent plan docs
3. Baca `docs/plans/PLAN-*.md` terbaru — extract goal + status
4. Cek `docs/stories/` modifications
5. Group output per modul (APP-A..E)

Output format:
```
## BLIPS Status — {range}

### APP-A Master Data + SPPI/BM
- ✅ Done: ...
- 🚧 In progress: ... (owner: agent-name)
- ⏳ Blocked: ... (reason)

### APP-B Transaction Lifecycle
...

### Cross-cutting
- Pending compliance review: ...
- Pending security review: ...
- Open Decision Log items: ...
```

Tidak panggil agent — ini ringkasan deterministik dari artifact yang ada.

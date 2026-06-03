# PLAN-20260602 — P0-1: Signed Commits Setup (workspace)

**Orchestrator**: tech-lead-orchestrator
**Tanggal**: 2026-06-02
**Sumber**: `docs/handoff/phase-0-to-phase-2.md` §4a P0-1 (BLOCKER, per MDA APPROVED #5)
**Klasifikasi request**: infra/workspace setup (non-regulated — tidak menyentuh ECL/EIR/SPPI/BM/PII)

---

## Goal
Workspace bisa menghasilkan **signed commit** sehingga PR `feature/*`/`chore/*` bisa landing ke
`develop` (branch protection mensyaratkan signed merge commit per
`.github/` branch protection + `git-conventions.md` §"Signed commits").

## Affected
- **Schema/kode**: tidak ada (config-only).
- **File**: repo-local `.git/config` (TIDAK tracked) + `~/.ssh` atau GPG keyring (di luar repo).
- **Modul**: cross-cutting (`repo`), bukan APP-A..E.

## Agents involved + handoff
- **tech-lead-orchestrator** (saya): plan + wiring repo-local config + verifikasi + update handoff doc.
- **devops-engineer**: owner runbook ops bila perlu diformalkan ke `docs/runbooks/`.
- **security-engineer**: TIDAK blocking di sini (tidak menyentuh auth/PII/audit code), tapi
  enforce policy "private key never in repo" — sudah dipatuhi.

> Catatan governance (handoff §0.1): Phase 2+ wajib routing ke subagent. P0-1 owner by-design
> "per-developer" + porsi key-gen/GitHub-upload hanya bisa dieksekusi manusia di shell. Bagian
> yang bisa diotomasi (repo-local `git config`) dikerjakan orchestrator; bagian human di-flag.

## Blocking dependencies
- Tidak ada dependency ke agent lain. Self-contained.

## Pilihan teknis
SSH signing (DEC tidak melarang; `git-conventions.md` sebut "SSH signing alternative, lebih simpel",
effort 2 menit) **direkomendasikan** dibanding GPG (10 menit). Rasionil: lebih cepat, key reuse
dengan auth, cukup untuk requirement "signed merge commit".

## Langkah (human portion — jalankan di shell)
```bash
# 1. Generate SSH key khusus signing (kalau belum ada id_ed25519)
ssh-keygen -t ed25519 -C "fairuzzbd@gmail.com" -f ~/.ssh/id_ed25519

# 2. Upload PUBLIC key ke GitHub sebagai *Signing Key* (BUKAN Authentication Key)
#    Settings → SSH and GPG keys → New SSH key → Key type: "Signing Key"
cat ~/.ssh/id_ed25519.pub   # paste isi ini ke GitHub

# 3. (opsional, kalau mau gh CLI)
gh ssh-key add ~/.ssh/id_ed25519.pub --type signing --title "ifrs9ai-signing"
```

## Langkah (orchestrator portion — repo-local config)
Setelah kunci ADA di `~/.ssh/id_ed25519.pub`, set repo-local config:
```bash
git config gpg.format ssh
git config user.signingkey ~/.ssh/id_ed25519.pub
git config commit.gpgsign true
git config tag.gpgsign true
# Identity dipertahankan: fairuzzbd@gmail.com (per keputusan user — debt #7 TIDAK diubah)
```
> Repo-local (`.git/config`) sengaja dipilih agar tidak menyentuh `~/.gitconfig` global pengguna
> lain di mesin yang sama. Aman: `.git/config` tidak ter-track Git.

## Risk + rollback
- **Risk**: set `commit.gpgsign=true` SEBELUM kunci valid → semua `git commit` GAGAL.
  Mitigasi: aktifkan `gpgsign` HANYA setelah langkah 1–2 selesai & terverifikasi.
- **Rollback**: `git config --unset commit.gpgsign` (revert instan, no data loss).
- **Security**: private key TIDAK boleh masuk repo (security-baseline). Hanya `.pub` di-upload.

## Verifikasi (definition of done)
1. `git commit --allow-empty -m "chore(repo): verify signed commit"` sukses (tidak error).
2. `git log --show-signature -1` → tampil `Good "git" signature`.
3. Commit muncul **Verified** badge di GitHub.
4. (akhir) Update `docs/handoff/phase-0-to-phase-2.md` §4a: P0-1 status → RESOLVED.
   (Debt #7 tetap open — identity sengaja `fairuzzbd@gmail.com` per keputusan user.)

## Status — ✅ RESOLVED 2026-06-02
- [x] Human: generate key + upload signing key ke GitHub
- [x] Orchestrator: wiring repo-local `git config` (`.git/config` — gpg.format ssh, signingkey, commit/tag gpgsign)
- [x] Verifikasi 1–3 (signed commit sukses, `git log --show-signature` = Good signature) — user confirmed "verified"
- [x] Update handoff doc (P0-1 RESOLVED; debt #7 tetap open — identity sengaja gmail)

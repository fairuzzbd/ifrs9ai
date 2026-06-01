# UX Patterns — BLIPS

Detail teknis 3 mandatory UX rules dari CLAUDE.md. Semua agent (backend/frontend/UX) wajib follow saat implementasi.

---

## §1. List / Tabel Data — Sort + Paging + Filter + Export

### 1.1 Backend contract (REST)

**Endpoint pattern:**
```
GET /api/v1/{resource}?cursor=...&limit=50&sort=col:asc,col2:desc&q=...&filter[col]=val
```

**Query parameters:**

| Param | Format | Notes |
|---|---|---|
| `cursor` | opaque base64 | dari `pagination.nextCursor` di response sebelumnya |
| `limit` | int 1-200, default 50 | hard cap 200 |
| `sort` | `col1:asc,col2:desc` | multi-column, max 3 cols |
| `q` | string | text search global (server decide kolom mana di-scan) |
| `filter[<col>]` | string atau `op:val` | mis. `filter[stage]=2`, `filter[ead]=gte:1000000` |
| `filter[<col>][]` | string | multi-value: `filter[bank][]=BCA&filter[bank][]=BNI` |

**Filter operators** (untuk numeric/date kolom):
- `eq:val` (default if no prefix)
- `ne:val`
- `gt:val`, `gte:val`, `lt:val`, `lte:val`
- `between:val1,val2`
- `in:val1,val2,val3`
- `like:%substr%` (text only)
- `is_null` / `is_not_null`

**Response envelope:**
```json
{
  "data": [...],
  "pagination": {
    "nextCursor": "eyJ...==",
    "hasMore": true,
    "totalEstimate": 12340,
    "limit": 50
  },
  "appliedSort": [{"col":"created_at","dir":"desc"}],
  "appliedFilter": {"stage":"2","bank":["BCA"]},
  "meta": {"traceId":"..."}
}
```

### 1.2 Backend implementation pattern (Go)

```go
// internal/common/listquery/listquery.go
type Query struct {
    Cursor   string
    Limit    int
    Sort     []SortSpec    // [{Col:"created_at", Dir:"desc"}]
    Search   string        // ?q=
    Filters  []FilterSpec  // [{Col:"stage", Op:"eq", Value:"2"}]
}

type SortSpec   struct { Col string; Dir string }
type FilterSpec struct { Col string; Op string; Value any }

func ParseFromRequest(r *http.Request, allowedCols []string) (Query, error) { ... }
func (q Query) ToSQL(tableAlias string) (whereClause string, args []any, orderBy string) { ... }
```

**Wajib server-side validation:**
- Kolom di sort/filter HARUS di `allowedCols` whitelist per endpoint. Reject `INVALID_SORT_COL` jika tidak.
- `limit` clamp ke max 200.
- Filter operator validated per kolom type (mis. `like` tidak boleh di kolom NUMERIC).
- SQL pakai parameterized query — **never** string concat (SQLi).

**Repository pattern:**
```go
func (r *Repo) List(ctx context.Context, q listquery.Query) ([]Entity, listquery.Pagination, error) {
    allowed := []string{"id", "created_at", "stage", "bank_id", "ead"}
    where, args, orderBy := q.WithAllowed(allowed).ToSQL("t")
    // SELECT ... FROM mst.instrumen t WHERE ... ORDER BY ... LIMIT $N+1
    // Pakai limit+1 trick untuk detect hasMore tanpa COUNT(*) yang mahal
}
```

### 1.3 Frontend implementation pattern (Next.js + TanStack Table)

Komponen wajib: `components/blips/DataTable.tsx`

```tsx
<DataTable
  endpoint="/api/v1/master/instrumen"
  columns={[
    { id: "kode_instrumen",   header: "Kode",      sortable: true, filterable: true },
    { id: "nama",             header: "Nama",      sortable: true, filterable: true },
    { id: "stage",            header: "Stage",     sortable: true, filterable: { type: "select", options: [1,2,3] } },
    { id: "ead",              header: "EAD (IDR)", sortable: true, align: "right", format: "idr" },
    { id: "created_at",       header: "Dibuat",    sortable: true, format: "datetime" },
  ]}
  defaultSort={[{ id: "created_at", desc: true }]}
  pageSize={50}
  exportFormats={["csv", "xlsx"]}
  exportFilename={(filters) => `instrumen-${dayjs().format('YYYYMMDD')}.csv`}
  permission="instrumen.read"
/>
```

**Yang harus tampil di UI:**
- Header kolom sortable: panah ↑↓ icon, ketuk toggle asc/desc/none, multi-sort via shift+click.
- Filter bar: text search + per-column filter chips (modal "Filter" untuk kolom kompleks). Clear-all button.
- Pagination footer: `Prev | Page 3 of ~247 | Next` + dropdown limit (25/50/100/200).
- Action bar atas: **Export ▾** dropdown (CSV / XLSX / PDF opsional), tombol "Refresh", info "Last updated: ...".
- Empty state: ilustrasi + pesan "Tidak ada data yang cocok" + "Clear filter" CTA jika filter aktif.
- Loading state: skeleton row, bukan blank screen.
- Error state: pesan + retry button.

**Deep-link friendly URL state** (pakai `nuqs` atau Next.js searchParams):
```
/master/instrumen?sort=created_at:desc&filter[stage]=2&filter[bank]=BCA&q=deposito
```
User bisa bookmark / share URL → state restored.

### 1.4 Export pattern

**Inline export** (< 10k row, < 5MB output):
- Klik tombol Export → backend stream langsung file ke browser.
- Backend: `GET /api/v1/{resource}/export?format=csv&...same params...`
- Content-Disposition: `attachment; filename="..."`
- Header `X-Total-Rows` untuk validasi client-side.

**Async export** (≥ 10k row atau heavy report):
- Klik Export → submit Asynq job → `202 Accepted { jobId }`.
- Frontend: tampilkan progress (rule §3).
- Worker: stream rows ke CSV/XLSX di MinIO `exports/{tenant}/{user}/{yyyy/mm/dd}/{jobid}.csv`.
- Setelah selesai: notification dengan signed download URL (TTL 24 jam).

**Format spec:**
- CSV: UTF-8 with BOM (untuk Excel ID), delimiter `,`, quote `"`, escape `""`, line ending `\r\n`.
- XLSX: pakai `excelize` (Go), header bold + freeze, money formatted `#,##0.0000`, date `YYYY-MM-DD`.
- Header row: nama kolom human-readable (Bahasa Indonesia), bukan snake_case DB.
- Footer row (opsional): timestamp export + filter aktif sebagai metadata.

**Audit:**
```go
audit.Write(ctx, audit.Event{
    Action: "INSTRUMEN.EXPORT",
    EntityType: "mst.instrumen",
    EntityID: uuid.Nil, // bulk
    After: map[string]any{
        "format": "csv",
        "row_count": 1234,
        "filters": q.Filters,
        "filename": "instrumen-20260602.csv",
    },
})
```

### 1.5 Anti-patterns
- ❌ Offset pagination — pakai cursor (lihat @.claude/memory/api-conventions.md).
- ❌ Sort by un-indexed column tanpa explicit ALLOW di repo.
- ❌ Filter di-build via string concat → SQLi.
- ❌ Export semua data tanpa respect filter (data leak ke user yang harusnya tidak akses semua).
- ❌ Export sync untuk dataset besar → hang server thread + browser timeout.
- ❌ Tabel tanpa empty state / loading state / error state.

---

## §2. Form Submission Notification (Sukses / Gagal)

### 2.1 Notification levels & UX rules

| Level | Color | Auto-dismiss | When |
|---|---|---|---|
| `success` | green | 4 detik | Action sukses |
| `info` | blue | 4 detik | Informational (e.g. "Data sudah ter-cache") |
| `warning` | amber | 8 detik | Sukses tapi ada perhatian (e.g. "Tersimpan, tapi rate FX hari ini belum tersedia") |
| `error` | red | **persistent** (manual close) | Gagal — user perlu tahu |
| `progress` | blue spinner | until job done | Long-running, jadi ke §3 |

**Posisi**: top-right, stack dari atas. Max 5 toast bersamaan, FIFO push out yang lama.

**Konten toast (struktur):**
```
[icon] [Judul singkat]                              [×]
        [Pesan spesifik, ≤ 140 char]
        [Optional: action link]
        [Optional: traceId di footer, kecil]
```

### 2.2 Pesan — wajib spesifik & actionable

**❌ Generic, dilarang:**
- "Berhasil"
- "Gagal"
- "Terjadi kesalahan"
- "Error 500"

**✅ Spesifik:**
- "Instrumen INST-001234 berhasil dibuat. Menunggu review oleh Treasury Approver."
- "Penempatan deposito DP-0042 berhasil di-approve. Jurnal otomatis di-post."
- "Submit gagal: Idempotency-Key sudah dipakai dengan payload berbeda (request sebelumnya berhasil). Trace: a1b2c3..."
- "Validation: 3 field bermasalah — lihat highlight di form."
- "Tidak punya permission `ecl_parameter.approve`. Hubungi ROLE-ALCO untuk request akses."

**Action link** kalau relevan:
- Sukses create → "Lihat detail →"
- Sukses submit ke approver → "Lihat status workflow →"
- Sukses upload → "Lihat hasil parsing →"

### 2.3 Implementation pattern (frontend)

Pakai `sonner` atau `react-hot-toast`. Komponen wrapper BLIPS:

```tsx
// lib/notify.ts
import { toast } from "sonner";

export const notify = {
  success: (msg: string, opts?: NotifyOpts) =>
    toast.success(msg, { duration: 4000, action: opts?.action }),

  error: (err: ApiError, opts?: NotifyOpts) =>
    toast.error(formatError(err), {
      duration: Infinity, // persistent
      action: opts?.action ?? { label: "Salin traceId", onClick: () => navigator.clipboard.writeText(err.traceId) },
      description: `${err.code} · trace: ${err.traceId.slice(0,8)}`,
    }),

  warning: (msg: string) => toast.warning(msg, { duration: 8000 }),
  info:    (msg: string) => toast.info(msg, { duration: 4000 }),
};

function formatError(err: ApiError): string {
  // Map known codes ke pesan Bahasa Indonesia
  const map: Record<string,string> = {
    "SOD_VIOLATION":      "Anda tidak bisa menjadi reviewer/approver untuk transaksi yang Anda buat sendiri.",
    "IDEMPOTENCY_REPLAY": "Request ini sudah pernah berhasil sebelumnya.",
    "IDEMPOTENCY_MISMATCH": "Idempotency-Key sudah dipakai dengan payload berbeda.",
    "PERIODE_CLOSED":     "Periode buku sudah hard-closed, tidak bisa di-mutate.",
    "ECL_PARAM_FROZEN":   "Calc run sudah di-seal, parameter tidak bisa diubah.",
    // ...
  };
  return map[err.code] ?? err.message ?? "Terjadi kesalahan tidak diketahui";
}
```

### 2.4 Form submit pattern

```tsx
function PenempatanForm() {
  const form = useForm<PenempatanInput>({ resolver: zodResolver(schema) });
  const [submitting, setSubmitting] = useState(false);

  const onSubmit = async (data: PenempatanInput) => {
    setSubmitting(true);  // disable button + spinner
    try {
      const result = await api.penempatan.create({
        body: data,
        headers: { "Idempotency-Key": uuidv4() }
      });
      notify.success(
        `Penempatan ${result.data.kode} berhasil dibuat. Menunggu review.`,
        { action: { label: "Lihat detail", onClick: () => router.push(`/penempatan/${result.data.id}`) } }
      );
      form.reset();
    } catch (err) {
      if (isValidationError(err)) {
        // Set field errors → highlight inline dengan aria-describedby
        err.details.forEach(d => form.setError(d.field, { message: d.message }));
        notify.error({ ...err, message: `${err.details.length} field bermasalah — lihat form di bawah` });
      } else {
        notify.error(err);
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      {/* fields */}
      <FormSubmitButton submitting={submitting}>Simpan</FormSubmitButton>
    </form>
  );
}
```

### 2.5 Destructive action confirmation

Sebelum action irreversible (delete soft, reject, hard-close, seal calc run, reklasifikasi):

```tsx
<DestructiveActionDialog
  title="Hard-close periode Juni 2026?"
  description="Setelah hard-close, periode tidak bisa di-reopen. Semua jurnal akan final. Lanjut?"
  confirmText="Hard-close (CFO MFA required)"
  confirmVariant="destructive"
  requireMFA={true}
  onConfirm={async (mfaToken) => {
    await api.periode.hardclose({ body: { mfa_token: mfaToken } });
    notify.success("Periode Juni 2026 berhasil hard-closed. CHANGELOG audit log entry: ...");
  }}
/>
```

### 2.6 Anti-patterns
- ❌ Silent success — user tidak tahu apakah kliknya jadi.
- ❌ `alert("Error")` — bukan UX 2026.
- ❌ Generic "Error 500" — tampilkan code + traceId minimum.
- ❌ Toast yang auto-dismiss sebelum user sempat baca (< 3 detik).
- ❌ Error toast yang dismiss sendiri — user mungkin tidak nyadar.
- ❌ Form reset sebelum success konfirmasi → user kehilangan input kalau actually gagal.

---

## §3. Long-Running Process — Progress Notification

### 3.1 Apa yang termasuk "long-running"

> Default rule: jika operasi **> 2 detik** rata-rata, treat sebagai long-running.

Operasi yang HARUS pakai pattern ini di BLIPS:
- ECL calc run (per periode, all instruments)
- EIR amendment re-estimation untuk portofolio
- File upload + parsing (Pefindo XLSX, IBPA CSV, KSEI NAB, BEI closing)
- Bulk import master data
- Export besar (> 10k row)
- Refresh materialized view (`rpt.mv_*`)
- Batch journal posting (end-of-day, end-of-period)
- Reklasifikasi bulk
- Recompute roll-forward CKPN

### 3.2 Backend pattern

**1. Job submit endpoint:**
```
POST /api/v1/ecl/calc-runs
Authorization: Bearer ...
Idempotency-Key: ...

→ 202 Accepted
{
  "data": {
    "jobId": "job_01HXYZ...",
    "type": "ECL_CALC_RUN",
    "statusUrl": "/api/v1/jobs/job_01HXYZ...",
    "streamUrl": "/api/v1/jobs/job_01HXYZ.../stream"
  }
}
```

**2. Job status endpoint:**
```
GET /api/v1/jobs/{jobId}

→ 200 OK
{
  "data": {
    "jobId": "job_01HXYZ...",
    "type": "ECL_CALC_RUN",
    "status": "running",          // queued | running | completed | failed | cancelled
    "progress": 47,                // 0-100
    "currentStep": "Calculating Stage 2 instruments (1234 of 2600)",
    "startedAt": "2026-06-02T10:30:00+07:00",
    "estimatedCompletionAt": "2026-06-02T10:35:00+07:00",
    "result": null,                // populated saat completed
    "error": null,                 // populated saat failed
    "canCancel": true,
    "createdBy": "user-uuid"
  }
}
```

**3. SSE stream endpoint:**
```
GET /api/v1/jobs/{jobId}/stream
Accept: text/event-stream

→ event: progress
  data: {"progress":47,"currentStep":"..."}

  event: progress
  data: {"progress":62,"currentStep":"..."}

  event: completed
  data: {"result":{"calcRunId":"...","totalECL":"1234567.8900"}}

  event: close
```

**4. Cancel endpoint** (opsional, jika cancellable):
```
POST /api/v1/jobs/{jobId}/cancel
→ 200 { "status": "cancelled" }
```

### 3.3 Worker pattern (Asynq Go)

```go
// internal/worker/ecl_calc.go
func (h *Handler) HandleECLCalcRun(ctx context.Context, t *asynq.Task) error {
    var p ECLCalcPayload
    if err := json.Unmarshal(t.Payload(), &p); err != nil {
        return err
    }

    job := h.jobs.MustLoad(ctx, p.JobID)

    // Report progress berkala via Redis pub/sub atau direct table update
    progress := newProgressReporter(h.redis, p.JobID)
    defer progress.Close()

    progress.Update(0, "Membaca daftar instrumen yang aktif...")

    instruments, _ := h.repo.ListActiveInstruments(ctx, p.PeriodeID)
    total := len(instruments)

    for i, inst := range instruments {
        if ctx.Err() != nil {  // check for cancellation
            progress.Cancel("Dibatalkan oleh user")
            return nil
        }

        if err := h.calcInstrument(ctx, inst, p); err != nil {
            progress.Fail(err)
            return err
        }

        // Update progress setiap 1% atau setiap 100 instrument (whichever first)
        if i % max(total/100, 100) == 0 {
            pct := (i * 100) / total
            progress.Update(pct, fmt.Sprintf("Menghitung instrument %d dari %d", i, total))
        }
    }

    progress.Complete(map[string]any{
        "calcRunId": p.CalcRunID,
        "totalInstruments": total,
        "totalECL": totalECL.String(),
    })
    return nil
}

type progressReporter struct {
    redis  *redis.Client
    jobID  string
}

func (p *progressReporter) Update(pct int, step string) {
    p.redis.HSet(ctx, "job:"+p.jobID, map[string]any{
        "progress": pct,
        "currentStep": step,
        "updated_at": time.Now().Unix(),
    })
    p.redis.Publish(ctx, "job-events:"+p.jobID, payload(pct, step))
}
```

**Persisted job state** di tabel `sys.job` (jangan hanya Redis — Redis bisa lost):
```sql
CREATE TABLE sys.job (
    id              TEXT PRIMARY KEY,         -- ULID/UUID
    type            TEXT NOT NULL,
    status          TEXT NOT NULL,            -- queued|running|completed|failed|cancelled
    progress        SMALLINT,
    current_step    TEXT,
    payload_jsonb   JSONB,
    result_jsonb    JSONB,
    error_jsonb     JSONB,
    created_by      UUID NOT NULL,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    can_cancel      BOOLEAN DEFAULT false,
    -- ...audit cols...
);
```

### 3.4 Frontend pattern

**Komponen `<JobProgressPanel>`:**
```tsx
<JobProgressPanel
  jobId={jobId}
  onComplete={(result) => {
    notify.success(`ECL calc run selesai. Total ECL: ${formatIDR(result.totalECL)}`, {
      action: { label: "Lihat detail", onClick: () => router.push(`/ecl/run/${result.calcRunId}`) }
    });
  }}
  onFail={(err) => notify.error(err)}
  showCancel={true}
/>
```

Internal: subscribe via `EventSource` (SSE) dulu, fallback ke polling tiap 2 detik kalau SSE error:

```tsx
function useJobProgress(jobId: string) {
  const [state, setState] = useState<JobState>({status:"queued", progress:0});

  useEffect(() => {
    const es = new EventSource(`/api/v1/jobs/${jobId}/stream`);
    es.addEventListener("progress", (e) => setState(JSON.parse(e.data)));
    es.addEventListener("completed", (e) => { setState(JSON.parse(e.data)); es.close(); });
    es.addEventListener("failed", (e)    => { setState(JSON.parse(e.data)); es.close(); });
    es.onerror = () => {
      es.close();
      // fallback polling
      const id = setInterval(async () => {
        const job = await api.jobs.get(jobId);
        setState(job.data);
        if (job.data.status === "completed" || job.data.status === "failed") clearInterval(id);
      }, 2000);
    };
    return () => es.close();
  }, [jobId]);

  return state;
}
```

**Visual:**
```
┌──────────────────────────────────────────────────┐
│ 🔄  ECL Calc Run — Periode Juni 2026             │
│                                                   │
│  ████████████░░░░░░░░░░  47%                     │
│                                                   │
│  Menghitung Stage 2 instruments (1234 dari 2600) │
│                                                   │
│  Mulai: 10:30:00 · ETA: 10:35:00 (5 menit lagi)  │
│                                                   │
│                        [ Batalkan ]  [ Background]│
└──────────────────────────────────────────────────┘
```

**Background mode**: tombol "Background" tutup panel, user lanjut kerja lain. Global notification badge di top bar nyala saat job selesai. Click badge → lihat job history page `/jobs`.

### 3.5 Job history page

`/jobs` — list semua job user (atau semua jika ROLE-IT-ADMIN), pakai DataTable pattern (§1).

Kolom: ID, Type, Status, Progress, Started, Completed, Duration, Created by, Actions (view detail, download result, retry, cancel).

Filter: status, type, date range, created_by.

### 3.6 Anti-patterns
- ❌ Blocking sync HTTP request 30 detik → browser timeout, server thread habis.
- ❌ Polling 200ms — DOS-in diri sendiri. Minimum interval 2 detik untuk polling fallback.
- ❌ Progress hanya di-store di Redis tanpa DB — lost saat restart, user kehilangan visibility.
- ❌ Job tanpa cancellation handling — user "stuck" jika operasi salah.
- ❌ Toast sukses muncul tapi job belum selesai (race condition pada SSE completed event).
- ❌ "Loading..." spinner tanpa progress percent untuk operasi > 5 detik.
- ❌ Auto-refresh halaman saat selesai → user kehilangan context, kalau form ada draft.

---

## Cross-reference

- Backend implementation: lihat `internal/common/listquery/`, `internal/common/jobs/`, `internal/notification/`.
- Frontend components: `components/blips/DataTable.tsx`, `components/blips/JobProgressPanel.tsx`, `lib/notify.ts`.
- API contract: @.claude/memory/api-conventions.md
- Storage: job result files di MinIO bucket `exports/`, retention 30 hari.
- Audit: setiap export + job submit dengan `aud.audit_log` entry.

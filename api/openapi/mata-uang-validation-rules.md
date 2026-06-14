# Validation Rules — mst.mata_uang

**Author**: system-analyst  
**Tanggal**: 2026-06-03  
**Story**: APP-A-MSTR-002  
**Berlaku untuk**: Go validator (`internal/master/matauang/validator.go`) + Zod schema (`web/lib/schemas/mata-uang.ts`)

---

## 1. Field-Level Validation Rules

| Field | Tipe DB | Rule | Error Code | Error Message ID | Zod | Go |
|---|---|---|---|---|---|---|
| `kodeMataUang` | CHAR(3) NOT NULL | required | `VALIDATION_FAILED` | `ERR_KODE_REQUIRED` | `z.string().min(1)` | `validate:"required"` |
| `kodeMataUang` | CHAR(3) NOT NULL | pattern `^[A-Z]{3}$` | `VALIDATION_FAILED` | `ERR_KODE_FORMAT` | `z.string().regex(/^[A-Z]{3}$/)` | `validate:"len=3,uppercase"` |
| `kodeMataUang` | CHAR(3) NOT NULL | unique (server-side, bukan Zod) | `CONFLICT` | `ERR_KODE_DUPLICATE` | n/a | check DB + return CONFLICT |
| `kodeMataUang` | CHAR(3) NOT NULL | immutable setelah create | `VALIDATION_FAILED` | `ERR_KODE_IMMUTABLE` | field `disabled` di update form | server ignore field saat PUT |
| `namaMataUang` | VARCHAR(60) NOT NULL | required | `VALIDATION_FAILED` | `ERR_NAMA_REQUIRED` | `z.string().min(1)` | `validate:"required"` |
| `namaMataUang` | VARCHAR(60) NOT NULL | minLength 3 | `VALIDATION_FAILED` | `ERR_NAMA_MIN` | `z.string().min(3)` | `validate:"min=3"` |
| `namaMataUang` | VARCHAR(60) NOT NULL | maxLength 60 | `VALIDATION_FAILED` | `ERR_NAMA_MAX` | `z.string().max(60)` | `validate:"max=60"` |
| `simbol` | VARCHAR(5) NOT NULL | required | `VALIDATION_FAILED` | `ERR_SIMBOL_REQUIRED` | `z.string().min(1)` | `validate:"required"` |
| `simbol` | VARCHAR(5) NOT NULL | minLength 1 | `VALIDATION_FAILED` | `ERR_SIMBOL_MIN` | `z.string().min(1)` | `validate:"min=1"` |
| `simbol` | VARCHAR(5) NOT NULL | maxLength 5 | `VALIDATION_FAILED` | `ERR_SIMBOL_MAX` | `z.string().max(5)` | `validate:"max=5"` |
| `decimalPlaces` | SMALLINT NOT NULL | required | `VALIDATION_FAILED` | `ERR_DECIMAL_REQUIRED` | `z.number()` | `validate:"required"` |
| `decimalPlaces` | SMALLINT NOT NULL | min 0 | `VALIDATION_FAILED` | `ERR_DECIMAL_MIN` | `z.number().min(0)` | `validate:"min=0"` |
| `decimalPlaces` | SMALLINT NOT NULL | max 4 | `VALIDATION_FAILED` | `ERR_DECIMAL_MAX` | `z.number().max(4)` | `validate:"max=4"` |
| `sumberKursDefault` | VARCHAR(30) NOT NULL | required | `VALIDATION_FAILED` | `ERR_SUMBER_REQUIRED` | `z.enum([...])` | `validate:"required,oneof=BI_JISDOR BI_KURS_TENGAH INTERNAL"` |
| `sumberKursDefault` | VARCHAR(30) NOT NULL | enum `[BI_JISDOR, BI_KURS_TENGAH, INTERNAL]` | `VALIDATION_FAILED` | `ERR_SUMBER_ENUM` | `z.enum(["BI_JISDOR","BI_KURS_TENGAH","INTERNAL"])` | `validate:"oneof=BI_JISDOR BI_KURS_TENGAH INTERNAL"` |
| `frekuensiUpdate` | VARCHAR(20) NOT NULL | required | `VALIDATION_FAILED` | `ERR_FREKUENSI_REQUIRED` | `z.enum([...])` | `validate:"required,oneof=..."` |
| `frekuensiUpdate` | VARCHAR(20) NOT NULL | enum `[HARIAN, INTRA_DAY, BULANAN]` | `VALIDATION_FAILED` | `ERR_FREKUENSI_ENUM` | `z.enum(["HARIAN","INTRA_DAY","BULANAN"])` | `validate:"oneof=HARIAN INTRA_DAY BULANAN"` |
| `aktifFlag` | BOOLEAN NOT NULL | boolean type | `VALIDATION_FAILED` | `ERR_AKTIF_TYPE` | `z.boolean()` | `validate:"omitempty"` (default true) |
| `tanggalMulaiAktif` | DATE NOT NULL | required | `VALIDATION_FAILED` | `ERR_TGL_REQUIRED` | `z.string().date()` | `validate:"required"` |
| `tanggalMulaiAktif` | DATE NOT NULL | format date ISO 8601 | `VALIDATION_FAILED` | `ERR_TGL_FORMAT` | `z.string().regex(/^\d{4}-\d{2}-\d{2}$/)` | `validate:"dateformat=2006-01-02"` |
| `rowVersion` | BIGINT (update only) | required (hanya pada PUT) | `VALIDATION_FAILED` | `ERR_ROW_VERSION_REQUIRED` | `z.number().int().positive()` | `validate:"required,min=1"` |

---

## 2. Cross-Field Validation Rules

| Rule | Fields Involved | Condition | Error Code | Error Message |
|---|---|---|---|---|
| Tanggal tidak boleh masa depan | `tanggalMulaiAktif` | `tanggalMulaiAktif > today()` | `VALIDATION_FAILED` | "Tanggal mulai aktif tidak boleh di masa depan. Hari ini: {today}" |
| aktif_flag false + ada referensi aktif | `aktifFlag`, `instrumen.*`, `kurs.*` | `aktifFlag=false AND EXISTS referensi aktif` | `ACTIVE_FLAG_REF_CONFLICT` | "Mata uang tidak bisa dinonaktifkan karena masih digunakan oleh {N} instrumen/kurs aktif." |
| Kode immutable saat update | `kodeMataUang` in PUT | field dikirim di body PUT | — | Server ignore silently (tidak return error — field di-drop) |
| is_system_currency immutable | `isSystemCurrency` in PUT | field dikirim di body | — | Server ignore silently |

---

## 3. Business Rule Validation (Service Layer)

| Rule | Trigger | Check | Error Code | Error Message |
|---|---|---|---|---|
| System currency protected (delete) | `DELETE /master/mata-uang/{kode}` | `is_system_currency = true` | `SYSTEM_CURRENCY_PROTECTED` | "Mata uang {kode} adalah currency fungsional sistem (IDR) dan tidak bisa dihapus." |
| System currency protected (kode change) | `PUT /master/mata-uang/{kode}` | Pernah ada `kodeMataUang` di body (ignore silently, tapi log) | — | Tidak error; field di-drop. Log warning untuk auditors. |
| Referensi aktif (delete) | `DELETE /master/mata-uang/{kode}` | `EXISTS (SELECT 1 FROM mst.instrumen WHERE kode_mata_uang={kode} AND deleted_at IS NULL)` OR EXISTS di `mst.kurs` | `ENTITY_IN_USE` | "Mata uang {kode} tidak bisa dihapus karena masih digunakan oleh {N} instrumen. Nonaktifkan dengan mengubah aktif_flag menjadi false." |
| Update hanya pada DRAFT/RETURNED | `PUT /master/mata-uang/{kode}` | `workflow_status NOT IN ('DRAFT','RETURNED')` | `MASTER_APPROVED_NO_EDIT` | "Mata uang {kode} sudah disetujui ({status}) dan tidak bisa diedit langsung. Workflow baru dimulai otomatis saat edit — konfirmasi di dialog." |
| Optimistic lock check | Setiap PUT/POST workflow | `rowVersion ≠ DB row_version` | `CONFLICT` | "Mata uang {kode} telah diubah oleh user lain. Muat ulang halaman." |
| SoD: reviewer bukan maker | POST .../review | `current_user_id = workflow_instance.maker_id` | `SOD_VIOLATION` | "Anda adalah pembuat mata uang ini. Tidak bisa menjadi reviewer sesuai aturan Segregation of Duties." |
| SoD: approver bukan maker/reviewer | POST .../approve | `current_user_id ∈ {maker_id, reviewer_id}` | `SOD_VIOLATION` | "Anda adalah pembuat atau reviewer mata uang ini. Tidak bisa menjadi approver." |
| MFA wajib untuk AKUN-CTL | POST .../review, .../approve | `role ∈ ROLE-AKUN-CTL AND mfa_verified = false` | `MFA_REQUIRED` | "Multi-Factor Authentication wajib untuk Finance Controller. Silakan login ulang dengan MFA." |
| Comment wajib untuk reject | POST .../reject | `len(comment) < 10` | `VALIDATION_FAILED` | "Alasan penolakan wajib diisi (minimum 10 karakter)." |
| Resubmit hanya dari RETURNED | POST .../submit | `workflow_status NOT IN ('DRAFT','RETURNED')` | `WORKFLOW_INVALID_TRANSITION` | "Submit hanya bisa dilakukan dari status DRAFT atau RETURNED. Status saat ini: {status}" |
| Delete hanya jika soft-not-deleted | DELETE .../kode | `deleted_at IS NOT NULL` | `NOT_FOUND` | "Mata uang {kode} tidak ditemukan (sudah dihapus)." |
| Idempotency-Key missing | Semua mutation | Header `Idempotency-Key` tidak ada | `VALIDATION_FAILED` | "Header Idempotency-Key wajib disertakan." |

---

## 4. Zod Schema (Frontend — TypeScript)

```typescript
// web/lib/schemas/mata-uang.ts
import { z } from "zod"

export const sumberKursEnum = z.enum(["BI_JISDOR", "BI_KURS_TENGAH", "INTERNAL"])
export const frekuensiEnum = z.enum(["HARIAN", "INTRA_DAY", "BULANAN"])

export const mataUangCreateSchema = z.object({
  kodeMataUang: z
    .string({ required_error: "Kode mata uang wajib diisi" })
    .regex(/^[A-Z]{3}$/, "Kode mata uang harus 3 huruf kapital sesuai ISO 4217 (contoh: IDR, USD, EUR)"),
  namaMataUang: z
    .string({ required_error: "Nama mata uang wajib diisi" })
    .min(3, "Nama minimal 3 karakter")
    .max(60, "Nama maksimal 60 karakter"),
  simbol: z
    .string({ required_error: "Simbol wajib diisi" })
    .min(1, "Simbol minimal 1 karakter")
    .max(5, "Simbol maksimal 5 karakter"),
  decimalPlaces: z
    .number({ required_error: "Decimal places wajib diisi" })
    .int("Harus bilangan bulat")
    .min(0, "Decimal places minimum 0")
    .max(4, "Decimal places maximum 4"),
  sumberKursDefault: sumberKursEnum,
  frekuensiUpdate: frekuensiEnum,
  tanggalMulaiAktif: z
    .string({ required_error: "Tanggal mulai aktif wajib diisi" })
    .regex(/^\d{4}-\d{2}-\d{2}$/, "Format tanggal harus YYYY-MM-DD")
    .refine(
      (val) => new Date(val) <= new Date(),
      "Tanggal mulai aktif tidak boleh di masa depan"
    ),
  aktifFlag: z.boolean().default(true),
})

export const mataUangUpdateSchema = mataUangCreateSchema
  .omit({ kodeMataUang: true })  // immutable — tidak ada di form update
  .extend({
    rowVersion: z
      .number({ required_error: "rowVersion wajib untuk update" })
      .int()
      .positive(),
  })

export const mataUangSubmitSchema = z.object({
  comment: z.string().max(500).nullish(),
  rowVersion: z.number().int().positive(),
})

export const mataUangApproveSchema = z.object({
  comment: z.string().max(1000).nullish(),
  signatureMethod: z.enum(["JWT_STANDARD", "JWT_STEP_UP"]).default("JWT_STANDARD"),
  rowVersion: z.number().int().positive(),
})

export const mataUangRejectSchema = z.object({
  comment: z
    .string({ required_error: "Alasan penolakan wajib diisi" })
    .min(10, "Alasan penolakan minimal 10 karakter")
    .max(1000, "Alasan penolakan maksimal 1000 karakter"),
  signatureMethod: z.enum(["JWT_STANDARD", "JWT_STEP_UP"]).default("JWT_STANDARD"),
  rowVersion: z.number().int().positive(),
})

// Type exports
export type MataUangCreateInput = z.infer<typeof mataUangCreateSchema>
export type MataUangUpdateInput = z.infer<typeof mataUangUpdateSchema>
export type MataUangSubmitInput = z.infer<typeof mataUangSubmitSchema>
export type MataUangApproveInput = z.infer<typeof mataUangApproveSchema>
export type MataUangRejectInput = z.infer<typeof mataUangRejectSchema>
```

---

## 5. Go Struct + Validation Tags

```go
// internal/master/matauang/dto.go
package matauang

import "time"

// CreateRequest adalah DTO untuk POST /master/mata-uang
type CreateRequest struct {
    KodeMataUang      string `json:"kodeMataUang"      validate:"required,len=3,uppercase,iso4217"`
    NamaMataUang      string `json:"namaMataUang"       validate:"required,min=3,max=60"`
    Simbol            string `json:"simbol"             validate:"required,min=1,max=5"`
    DecimalPlaces     int16  `json:"decimalPlaces"      validate:"required,min=0,max=4"`
    SumberKursDefault string `json:"sumberKursDefault"  validate:"required,oneof=BI_JISDOR BI_KURS_TENGAH INTERNAL"`
    FrekuensiUpdate   string `json:"frekuensiUpdate"    validate:"required,oneof=HARIAN INTRA_DAY BULANAN"`
    AktifFlag         *bool  `json:"aktifFlag"` // pointer untuk distinguish false vs absent; default true
    TanggalMulaiAktif string `json:"tanggalMulaiAktif"  validate:"required,date_format=2006-01-02,not_future"`
}

// UpdateRequest adalah DTO untuk PUT /master/mata-uang/{kode}
// KodeMataUang tidak ada (immutable — server ignore jika dikirim)
type UpdateRequest struct {
    NamaMataUang      *string `json:"namaMataUang"      validate:"omitempty,min=3,max=60"`
    Simbol            *string `json:"simbol"            validate:"omitempty,min=1,max=5"`
    DecimalPlaces     *int16  `json:"decimalPlaces"     validate:"omitempty,min=0,max=4"`
    SumberKursDefault *string `json:"sumberKursDefault" validate:"omitempty,oneof=BI_JISDOR BI_KURS_TENGAH INTERNAL"`
    FrekuensiUpdate   *string `json:"frekuensiUpdate"   validate:"omitempty,oneof=HARIAN INTRA_DAY BULANAN"`
    AktifFlag         *bool   `json:"aktifFlag"`
    TanggalMulaiAktif *string `json:"tanggalMulaiAktif" validate:"omitempty,date_format=2006-01-02,not_future"`
    RowVersion        int64   `json:"rowVersion"        validate:"required,min=1"`
}

// SubmitRequest adalah DTO untuk POST .../submit
type SubmitRequest struct {
    Comment    *string `json:"comment"    validate:"omitempty,max=500"`
    RowVersion int64   `json:"rowVersion" validate:"required,min=1"`
}

// ApproveRequest adalah DTO untuk POST .../review dan .../approve
type ApproveRequest struct {
    Comment         *string `json:"comment"         validate:"omitempty,max=1000"`
    SignatureMethod  string  `json:"signatureMethod" validate:"required,oneof=JWT_STANDARD JWT_STEP_UP"`
    RowVersion      int64   `json:"rowVersion"      validate:"required,min=1"`
}

// RejectRequest adalah DTO untuk POST .../reject
type RejectRequest struct {
    Comment        string `json:"comment"        validate:"required,min=10,max=1000"`
    SignatureMethod string `json:"signatureMethod" validate:"required,oneof=JWT_STANDARD JWT_STEP_UP"`
    RowVersion     int64  `json:"rowVersion"     validate:"required,min=1"`
}

// Custom validators yang perlu didaftarkan:
// "iso4217"   → cek apakah kode ada di tabel lookup ISO 4217 (atau regex ^[A-Z]{3}$ minimal)
// "not_future" → tanggal tidak boleh lebih dari hari ini
// "uppercase" → semua huruf kapital
```

---

## 6. Error Message Lookup Table (i18n IDs)

Bahasa Indonesia — disimpan di `web/lib/i18n/id/mata-uang.ts`:

| Message ID | Pesan Bahasa Indonesia |
|---|---|
| `ERR_KODE_REQUIRED` | "Kode mata uang wajib diisi" |
| `ERR_KODE_FORMAT` | "Kode mata uang harus 3 huruf kapital sesuai ISO 4217 (contoh: IDR, USD, EUR)" |
| `ERR_KODE_DUPLICATE` | "Mata uang {kode} sudah terdaftar di sistem" |
| `ERR_KODE_IMMUTABLE` | "Kode mata uang tidak bisa diubah setelah dibuat. Nonaktifkan mata uang ini dan buat baru jika perlu." |
| `ERR_NAMA_REQUIRED` | "Nama mata uang wajib diisi" |
| `ERR_NAMA_MIN` | "Nama minimal 3 karakter" |
| `ERR_NAMA_MAX` | "Nama maksimal 60 karakter" |
| `ERR_SIMBOL_REQUIRED` | "Simbol mata uang wajib diisi" |
| `ERR_SIMBOL_MIN` | "Simbol minimal 1 karakter" |
| `ERR_SIMBOL_MAX` | "Simbol maksimal 5 karakter" |
| `ERR_DECIMAL_REQUIRED` | "Decimal places wajib diisi" |
| `ERR_DECIMAL_MIN` | "Decimal places minimum 0" |
| `ERR_DECIMAL_MAX` | "Decimal places maximum 4" |
| `ERR_SUMBER_REQUIRED` | "Sumber kurs default wajib dipilih" |
| `ERR_SUMBER_ENUM` | "Sumber kurs tidak valid. Pilih: BI_JISDOR, BI_KURS_TENGAH, atau INTERNAL" |
| `ERR_FREKUENSI_REQUIRED` | "Frekuensi update wajib dipilih" |
| `ERR_FREKUENSI_ENUM` | "Frekuensi tidak valid. Pilih: HARIAN, INTRA_DAY, atau BULANAN" |
| `ERR_TGL_REQUIRED` | "Tanggal mulai aktif wajib diisi" |
| `ERR_TGL_FORMAT` | "Format tanggal harus YYYY-MM-DD (contoh: 2026-06-03)" |
| `ERR_TGL_FUTURE` | "Tanggal mulai aktif tidak boleh di masa depan. Hari ini: {today}" |
| `ERR_ROW_VERSION_REQUIRED` | "rowVersion wajib dikirim untuk update" |
| `ERR_SYSTEM_CURRENCY` | "Mata uang {kode} adalah currency fungsional sistem dan tidak bisa dihapus" |
| `ERR_ENTITY_IN_USE` | "Mata uang {kode} tidak bisa dihapus karena masih digunakan oleh {N} instrumen" |
| `ERR_APPROVED_NO_EDIT` | "Mata uang yang sudah disetujui tidak bisa diedit langsung. Hubungi Finance Controller." |
| `ERR_SOD_REVIEWER` | "Anda adalah pembuat mata uang ini. Tidak bisa menjadi reviewer (Segregation of Duties)." |
| `ERR_SOD_APPROVER` | "Anda adalah pembuat atau reviewer mata uang ini. Tidak bisa menjadi approver (Segregation of Duties)." |
| `ERR_MFA_REQUIRED` | "Multi-Factor Authentication wajib untuk Finance Controller. Login ulang dengan MFA." |
| `ERR_COMMENT_REQUIRED` | "Alasan penolakan wajib diisi (minimal 10 karakter)" |
| `ERR_WORKFLOW_TRANSITION` | "Transisi tidak valid dari status {from}. Status yang diizinkan: {allowed}" |
| `ERR_CONFLICT` | "Mata uang {kode} telah diubah oleh pengguna lain. Muat ulang halaman untuk melihat data terbaru." |
| `ERR_AKTIF_REF_CONFLICT` | "Mata uang tidak bisa dinonaktifkan karena masih digunakan oleh {N} instrumen aktif." |

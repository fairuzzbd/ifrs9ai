"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Upload, FileSpreadsheet, X, AlertTriangle, Info } from "lucide-react";
import { v4 as uuidv4 } from "uuid";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { cn } from "@/lib/utils";
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import { mtmUploadApi } from "@/lib/api/mtm.api";
import {
  mtmUploadFormSchema,
  type MtmUploadFormInput,
  type MtmUploadBatchResponse,
} from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MtmUploadDropzoneProps {
  onSuccess?: (response: MtmUploadBatchResponse) => void;
  onCancel?: () => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S2-AC1, S2-AC2, S2-AC3, S2-AC4 — clone of KursUploadDropzone with MTM columns)
// ---------------------------------------------------------------------------

export function MtmUploadDropzone({
  onSuccess,
  onCancel,
  className,
}: MtmUploadDropzoneProps) {
  const [isDragging, setIsDragging] = React.useState(false);
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());
  const fileInputRef = React.useRef<HTMLInputElement>(null);

  const form = useForm<MtmUploadFormInput>({
    resolver: zodResolver(mtmUploadFormSchema),
    defaultValues: {
      catatanUpload: "",
      tanggalMtmOverride: "",
    },
  });

  const watchedFile = form.watch("file");

  // Refresh idempotency key on each new file selection (DEC-021)
  React.useEffect(() => {
    idempotencyKey.current = uuidv4();
  }, [watchedFile]);

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
    const file = e.dataTransfer.files[0];
    if (file) {
      form.setValue("file", file, { shouldValidate: true });
    }
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      form.setValue("file", file, { shouldValidate: true });
    }
  };

  const handleRemoveFile = () => {
    form.setValue("file", undefined as unknown as File, { shouldValidate: false });
    form.clearErrors("file");
    if (fileInputRef.current) fileInputRef.current.value = "";
    idempotencyKey.current = uuidv4();
  };

  const onSubmit = async (data: MtmUploadFormInput) => {
    setIsSubmitting(true);
    try {
      const result = await mtmUploadApi.uploadBatch(data.file, {
        catatanUpload: data.catatanUpload,
        tanggalMtmOverride: data.tanggalMtmOverride || undefined,
        idempotencyKey: idempotencyKey.current,
      });

      const resp = result.data;

      // Show deviation warnings if any (8-second auto-dismiss warning toasts)
      resp.deviationWarnings.forEach((w) => {
        notify.warning(
          `Peringatan: ${w.instrumenKode} memiliki deviasi ${w.deltaPct.toFixed(2)}% melebihi threshold ${w.thresholdPct.toFixed(2)}%. Finance Controller wajib verifikasi sebelum jurnal diposting.`,
        );
      });

      const tanggalDisplay = data.tanggalMtmOverride || "tanggal di file";
      notify.success(
        `${resp.rowsValid} MTM berhasil di-upload untuk ${tanggalDisplay}. Status: Menunggu approval Finance Controller.`,
        {
          action: {
            label: "Lihat Batch",
            onClick: () => {
              window.location.href = `/mtm/upload/batch/${resp.uploadBatchId}`;
            },
          },
        },
      );

      form.reset();
      if (fileInputRef.current) fileInputRef.current.value = "";
      idempotencyKey.current = uuidv4();
      onSuccess?.(resp);
    } catch (err) {
      if (isApiError(err)) {
        // Set field-level errors from validation details
        if (err.details.length > 0) {
          err.details.forEach((d) => {
            const field = d.field === "tanggalMtmOverride"
              ? "tanggalMtmOverride"
              : d.field === "catatanUpload"
              ? "catatanUpload"
              : "file";
            form.setError(field as keyof MtmUploadFormInput, {
              message: d.message,
            });
          });
        }
        notify.error(err);
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal menghubungi server.", traceId: "" });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className={cn("space-y-5", className)}
      >
        {/* File dropzone */}
        <FormField
          control={form.control}
          name="file"
          render={({ fieldState }) => (
            <FormItem>
              <FormLabel>
                File Harga MTM <span aria-hidden="true">*</span>
              </FormLabel>
              <FormControl>
                <div
                  role="button"
                  tabIndex={0}
                  aria-label="Area upload file harga MTM XLSX atau CSV"
                  className={cn(
                    "relative flex flex-col items-center justify-center rounded-lg border-2 border-dashed p-8 text-center transition-colors cursor-pointer",
                    isDragging
                      ? "border-primary bg-primary/5"
                      : "border-border bg-muted/30 hover:bg-muted/50",
                    fieldState.error && "border-destructive bg-destructive/5",
                  )}
                  onDragOver={handleDragOver}
                  onDragLeave={handleDragLeave}
                  onDrop={handleDrop}
                  onClick={() => !watchedFile && fileInputRef.current?.click()}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      if (!watchedFile) fileInputRef.current?.click();
                    }
                  }}
                >
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept=".xlsx,.csv"
                    className="sr-only"
                    onChange={handleFileSelect}
                    aria-label="Pilih file harga MTM"
                  />

                  {watchedFile ? (
                    // File selected state
                    <div className="flex items-center gap-3">
                      <FileSpreadsheet
                        className="h-8 w-8 text-primary"
                        aria-hidden="true"
                      />
                      <div className="text-left">
                        <p className="text-sm font-medium">{watchedFile.name}</p>
                        <p className="text-xs text-muted-foreground">
                          {(watchedFile.size / 1024).toFixed(1)} KB
                        </p>
                      </div>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="ml-2 h-7 w-7"
                        aria-label="Hapus file yang dipilih"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleRemoveFile();
                        }}
                      >
                        <X className="h-4 w-4" aria-hidden="true" />
                      </Button>
                    </div>
                  ) : (
                    // Empty state
                    <div className="space-y-3">
                      <Upload
                        className="mx-auto h-8 w-8 text-muted-foreground"
                        aria-hidden="true"
                      />
                      <div>
                        <p className="text-sm font-medium">
                          Drag &amp; drop file XLSX atau CSV di sini
                        </p>
                        <p className="text-xs text-muted-foreground">
                          atau klik untuk memilih file
                        </p>
                        <p className="text-xs text-muted-foreground mt-1">
                          XLSX atau CSV · Maks 10 MB · Maks 500 baris per batch
                        </p>
                        <p className="text-xs text-muted-foreground mt-0.5 font-mono">
                          Kolom: kode_instrumen, tanggal_mtm, harga_pasar, [harga_sumber], [catatan]
                        </p>
                      </div>
                      <div className="flex gap-2 justify-center">
                        <a
                          href="/templates/mtm-upload-template.xlsx"
                          download
                          className="text-xs text-primary underline underline-offset-2"
                          onClick={(e) => e.stopPropagation()}
                        >
                          Unduh Template XLSX
                        </a>
                        <span className="text-xs text-muted-foreground">·</span>
                        <a
                          href="/templates/mtm-upload-template.csv"
                          download
                          className="text-xs text-primary underline underline-offset-2"
                          onClick={(e) => e.stopPropagation()}
                        >
                          Unduh Template CSV
                        </a>
                      </div>
                    </div>
                  )}
                </div>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Catatan upload */}
        <FormField
          control={form.control}
          name="catatanUpload"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Catatan Upload</FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  placeholder="Mis: Upload harga OBL-0088 dari Bloomberg 2026-06-18"
                  rows={2}
                  maxLength={1000}
                  aria-describedby="catatan-upload-hint"
                />
              </FormControl>
              <p id="catatan-upload-hint" className="text-xs text-muted-foreground">
                Opsional. Catatan untuk keseluruhan batch upload.
              </p>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Tanggal override (optional) */}
        <FormField
          control={form.control}
          name="tanggalMtmOverride"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Override Tanggal MTM (opsional)</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type="date"
                  aria-label="Override tanggal MTM untuk semua baris (opsional)"
                />
              </FormControl>
              <p className="text-xs text-muted-foreground">
                Opsional. Jika diisi, tanggal ini digunakan untuk semua baris di file.
              </p>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Info notice — AC instrumen + FCY kurs */}
        <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
          <Info
            className="mt-0.5 h-4 w-4 shrink-0 text-blue-600"
            aria-hidden="true"
          />
          <div className="space-y-1">
            <p>
              <strong>Catatan:</strong> Instrumen berklasifikasi <strong>AC</strong> (Amortised Cost){" "}
              tidak dapat di-MTM per PSAK 71 §4.1.2 — baris AC akan ditolak.
            </p>
            <p>
              Instrumen <strong>FCY</strong> (non-IDR) memerlukan kurs BI JISDOR hari ini
              (status <em>APPROVED</em>) di halaman Kurs.
            </p>
          </div>
        </div>

        {/* Deviation warning notice */}
        <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          <AlertTriangle
            className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
            aria-hidden="true"
          />
          <p>
            Baris dengan deviasi harga melebihi threshold akan masuk <strong>PENDING_REVIEW</strong>{" "}
            dan wajib diverifikasi Finance Controller sebelum jurnal diposting.
          </p>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-3">
          {onCancel && (
            <Button
              type="button"
              variant="outline"
              disabled={isSubmitting}
              onClick={onCancel}
            >
              Batal
            </Button>
          )}
          <Button
            type="submit"
            disabled={isSubmitting || !watchedFile}
            aria-label="Upload dan preview harga MTM"
          >
            {isSubmitting ? "Mengupload..." : "Upload & Preview"}
          </Button>
        </div>
      </form>
    </Form>
  );
}

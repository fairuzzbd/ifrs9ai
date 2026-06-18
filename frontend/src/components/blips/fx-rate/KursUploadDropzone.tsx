"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Upload, FileSpreadsheet, X, AlertTriangle } from "lucide-react";
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
import { kursUploadApi } from "@/lib/api/fx-rate.api";
import {
  kursUploadFormSchema,
  type KursUploadFormInput,
  type KursUploadResponse,
} from "@/lib/schemas/fx-rate.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface KursUploadDropzoneProps {
  onSuccess?: (response: KursUploadResponse) => void;
  onCancel?: () => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S2-AC1, S2-AC2, S2-AC3, S2-AC4)
// ---------------------------------------------------------------------------

export function KursUploadDropzone({
  onSuccess,
  onCancel,
  className,
}: KursUploadDropzoneProps) {
  const [isDragging, setIsDragging] = React.useState(false);
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());
  const fileInputRef = React.useRef<HTMLInputElement>(null);

  const form = useForm<KursUploadFormInput>({
    resolver: zodResolver(kursUploadFormSchema),
    defaultValues: {
      catatanUpload: "",
      tanggalBerlakuOverride: "",
    },
  });

  const watchedFile = form.watch("file");

  // Refresh idempotency key on each new upload attempt
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

  const onSubmit = async (data: KursUploadFormInput) => {
    setIsSubmitting(true);
    try {
      const result = await kursUploadApi.upload(data.file, {
        catatanUpload: data.catatanUpload,
        tanggalBerlakuOverride: data.tanggalBerlakuOverride || undefined,
        idempotencyKey: idempotencyKey.current,
      });

      const resp = result.data;

      // Show deviation warnings if any
      if (resp.deviationWarnings.length > 0) {
        resp.deviationWarnings.forEach((w) => {
          notify.warning(
            `Perhatian: Kurs ${w.kodeMataUang} memiliki deviasi ${w.rateDeviationPct.toFixed(2)}% dari hari sebelumnya (IDR ${w.previousKursTengah.toLocaleString("id-ID")} → IDR ${(w.previousKursTengah * (1 + w.rateDeviationPct / 100)).toLocaleString("id-ID")}). Harap verifikasi sebelum approve.`,
          );
        });
      }

      notify.success(
        `${resp.rowsValid} kurs berhasil di-upload untuk ${data.tanggalBerlakuOverride || "tanggal di file"}. Status: Menunggu approval Finance Controller.`,
        {
          action: {
            label: "Lihat antrian",
            onClick: () => {
              window.location.href = `/master/kurs?filter[workflow_status]=PENDING_APPROVAL&filter[upload_batch_id]=${resp.uploadBatchId}`;
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
        if (err.code === "KURS_UPLOAD_VALIDATION_FAILED" && err.details.length > 0) {
          err.details.forEach((d) => {
            // Map detail.field to form field; fallback to root
            const field = d.field === "catatan" ? "catatanUpload" : "file";
            form.setError(field as keyof KursUploadFormInput, {
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
                File Kurs <span aria-hidden="true">*</span>
              </FormLabel>
              <FormControl>
                <div
                  role="button"
                  tabIndex={0}
                  aria-label="Area upload file kurs XLSX atau CSV"
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
                    aria-label="Pilih file kurs"
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
                    <div className="space-y-2">
                      <Upload
                        className="mx-auto h-8 w-8 text-muted-foreground"
                        aria-hidden="true"
                      />
                      <div>
                        <p className="text-sm font-medium">
                          Drag &amp; drop file di sini
                        </p>
                        <p className="text-xs text-muted-foreground">
                          atau klik untuk memilih file
                        </p>
                        <p className="text-xs text-muted-foreground mt-1">
                          XLSX atau CSV · Maks 5 MB · Template:{" "}
                          <code className="font-mono">
                            kode_mata_uang, tanggal_berlaku, kurs_tengah, [kurs_beli], [kurs_jual], [catatan]
                          </code>
                        </p>
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
                  placeholder="Mis: Upload manual karena JISDOR gangguan 2026-06-18"
                  rows={2}
                  maxLength={1000}
                  aria-describedby="catatan-hint"
                />
              </FormControl>
              <p id="catatan-hint" className="text-xs text-muted-foreground">
                Wajib diisi untuk baris dengan deviasi &gt; 20% dari hari sebelumnya (min 20 karakter).
              </p>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Tanggal override (optional) */}
        <FormField
          control={form.control}
          name="tanggalBerlakuOverride"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Override Tanggal Berlaku</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type="date"
                  aria-label="Override tanggal berlaku untuk semua baris (opsional)"
                />
              </FormControl>
              <p className="text-xs text-muted-foreground">
                Opsional. Override tanggal berlaku untuk semua baris di file.
              </p>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Deviation warning notice */}
        <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          <AlertTriangle
            className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
            aria-hidden="true"
          />
          <p>
            Baris dengan deviasi &gt; 20% dari kurs hari sebelumnya{" "}
            <strong>wajib menyertakan catatan</strong> (kolom catatan di file, min 20 karakter).
            Kurs tetap diterima namun Finance Controller akan melihat badge peringatan deviasi.
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
            aria-label="Upload kurs manual"
          >
            {isSubmitting ? "Mengupload..." : "Upload Kurs"}
          </Button>
        </div>
      </form>
    </Form>
  );
}

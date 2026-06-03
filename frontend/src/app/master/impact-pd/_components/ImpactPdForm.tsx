"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { v4 as uuidv4 } from "uuid";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Info } from "lucide-react";

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ReturnedBanner } from "@/components/blips/ReturnedBanner";
import { cn } from "@/lib/utils";
import { impactPdApi } from "@/lib/api/impact-pd.api";
import { periodeBukuApi } from "@/lib/api/periode-buku.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  impactPdCreateSchema,
  impactPdUpdateSchema,
  type ImpactPdCreateInput,
  type ImpactPdUpdateInput,
  type ImpactPdItem,
} from "@/lib/schemas/impact-pd.schema";

// ---------------------------------------------------------------------------
// Warning guidance (informational, not hard reject)
// ---------------------------------------------------------------------------

/** Returns a warning string if value is outside the "typical expected" range. */
function getMultiplierGuidance(value: string): string | null {
  const n = Number(value);
  if (Number.isNaN(n) || value === "") return null;

  // DB hard range 0.5–2.0 is already validated in schema (hard reject)
  // Here we provide an informational nudge for edge values within valid range
  if (n === 1.0) {
    return "Nilai 1.0 berarti tidak ada penyesuaian Forward-Looking (sama dengan tidak menggunakan multiplier). Pastikan ini disengaja.";
  }
  if (n < 0.7) {
    return "Multiplier yang sangat rendah (< 0.7) mengindikasikan perbaikan kondisi ekonomi yang sangat signifikan. Pastikan justifikasi tercantum di catatan.";
  }
  if (n > 1.5) {
    return "Multiplier yang tinggi (> 1.5) mengindikasikan deteriorasi kondisi ekonomi yang sangat signifikan. Pastikan justifikasi tercantum di catatan.";
  }
  return null;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ImpactPdFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<ImpactPdItem>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ImpactPdForm({ mode, defaultValues }: ImpactPdFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<string | null>(null);

  const isEdit = mode === "edit";

  // ---------------------------------------------------------------------------
  // Periode dropdown — preload all periods (small dataset)
  // ---------------------------------------------------------------------------

  const { data: periodeData, isLoading: periodeLoading } = useQuery({
    queryKey: ["periode-buku-dropdown"],
    queryFn: () => periodeBukuApi.list({ limit: 200, sort: "periode_bulan:desc" }),
    staleTime: 5 * 60_000,
  });

  const periodeOptions = periodeData?.data ?? [];

  // ---------------------------------------------------------------------------
  // Form setup
  // ---------------------------------------------------------------------------

  const form = useForm<ImpactPdCreateInput>({
    resolver: zodResolver(impactPdCreateSchema),
    defaultValues: {
      periodeId: defaultValues?.periodeId ?? "",
      impactMultiplier: defaultValues?.impactMultiplier ?? "1.00000000",
      catatan: defaultValues?.catatan ?? "",
    },
  });

  const { isDirty } = form.formState;

  const watchedMultiplier = form.watch("impactMultiplier");
  const multiplierGuidance = getMultiplierGuidance(watchedMultiplier);

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/impact-pd");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/impact-pd");
    }
  };

  const handleConfirmLeave = () => {
    if (pendingNavigation) router.push(pendingNavigation);
    setUnsavedDialogOpen(false);
  };

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const onSubmit = async (values: ImpactPdCreateInput) => {
    const catatan = values.catatan?.trim() || undefined;

    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (isEdit && defaultValues?.id && defaultValues.rowVersion !== undefined) {
        const updateData: ImpactPdUpdateInput = {
          impactMultiplier: values.impactMultiplier,
          catatan,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await impactPdApi.update(defaultValues.id, updateData, idempotencyKey);
        notify.success(
          `Impact PD berhasil diperbarui (multiplier: ${res.data.impactMultiplier}). Menunggu review.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/impact-pd/${res.data.id}`),
            },
          },
        );
        router.push(`/master/impact-pd/${res.data.id}`);
      } else {
        const res = await impactPdApi.create({ ...values, catatan }, idempotencyKey);
        notify.success(
          `Impact PD berhasil dibuat (multiplier: ${res.data.impactMultiplier}). Menunggu review ALCO.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/impact-pd/${res.data.id}`),
            },
          },
        );
        router.push(`/master/impact-pd/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace("body.", "") as keyof ImpactPdCreateInput;
          form.setError(fieldName, { message: d.message });
        });

        const fieldErrorCount = err.details.length;
        if (fieldErrorCount > 0) {
          notify.error({
            ...err,
            message: `${fieldErrorCount} field bermasalah — lihat form di bawah.`,
          });
          setTimeout(() => {
            const firstErrorEl = document.querySelector("[aria-invalid='true']");
            firstErrorEl?.scrollIntoView({ behavior: "smooth", block: "center" });
          }, 100);
        } else {
          notify.error(err);
        }
      } else {
        notify.error({
          code: "INTERNAL",
          message: "Terjadi kesalahan. Coba lagi.",
          traceId: "",
        });
      }
    } finally {
      setSubmitting(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Returned banner
  // ---------------------------------------------------------------------------

  const showReturnedBanner = isEdit && defaultValues?.workflowStatus === "REJECTED";

  return (
    <>
      {showReturnedBanner && (
        <ReturnedBanner
          rejectedBy="ALCO / Risk Officer"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Section: Identitas */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Identitas Parameter
              </h2>

              {/* Periode Buku */}
              <FormField
                control={form.control}
                name="periodeId"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Periode Buku{" "}
                      <span className="text-destructive" aria-hidden>*</span>
                    </FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={field.onChange}
                      disabled={isEdit || periodeLoading}
                    >
                      <FormControl>
                        <SelectTrigger
                          aria-required="true"
                          className={cn(isEdit && "bg-muted cursor-not-allowed")}
                        >
                          <SelectValue
                            placeholder={
                              periodeLoading ? "Memuat periode..." : "Pilih periode buku"
                            }
                          />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {periodeOptions.map((p) => (
                          <SelectItem key={p.id} value={p.id}>
                            {p.label ?? p.periodeBulan}
                            {p.status === "SOFT_CLOSED" && (
                              <span className="ml-1.5 text-xs text-amber-600">(Soft-closed)</span>
                            )}
                          </SelectItem>
                        ))}
                        {periodeOptions.length === 0 && !periodeLoading && (
                          <SelectItem value="_none" disabled>
                            Tidak ada periode tersedia
                          </SelectItem>
                        )}
                      </SelectContent>
                    </Select>
                    <FormDescription>
                      Setiap periode hanya boleh memiliki 1 Impact PD (UNIQUE constraint).
                      {isEdit && (
                        <span className="ml-1 text-muted-foreground">(tidak bisa diubah)</span>
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            {/* Section: Multiplier */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Forward-Looking PD Multiplier
              </h2>

              {/* Range info banner */}
              <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-800">
                <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
                <div>
                  <span className="font-medium">Panduan range (DEC-010):</span>
                  <ul className="mt-1 list-disc pl-4 space-y-0.5">
                    <li>
                      Range yang diperbolehkan: <strong>0.5 – 2.0</strong> (hard constraint DB + validasi form)
                    </li>
                    <li>
                      Default: <strong>1.0</strong> (tidak ada penyesuaian)
                    </li>
                    <li>
                      &lt; 1.0 = kondisi makroekonomi membaik (PD turun) &nbsp;|&nbsp;
                      &gt; 1.0 = kondisi memburuk (PD naik)
                    </li>
                    <li>
                      Disetujui ALCO — perubahan memerlukan 6-eyes approval + step-up MFA.
                    </li>
                  </ul>
                </div>
              </div>

              {/* Impact Multiplier */}
              <FormField
                control={form.control}
                name="impactMultiplier"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Impact Multiplier{" "}
                      <span className="text-destructive" aria-hidden>*</span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder="1.00000000"
                        aria-required="true"
                        inputMode="decimal"
                        className="font-mono max-w-xs"
                      />
                    </FormControl>
                    <FormDescription>
                      Angka desimal dalam rentang 0.5–2.0, presisi 8 desimal. Mis: 0.95 atau 1.15.
                    </FormDescription>
                    {multiplierGuidance && (
                      <div
                        className="mt-1.5 flex items-start gap-1.5 rounded-md border border-amber-200 bg-amber-50 px-2.5 py-2 text-xs text-amber-800"
                        role="alert"
                        aria-live="polite"
                      >
                        <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
                        {multiplierGuidance}
                      </div>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            {/* Section: Catatan */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Catatan
              </h2>

              <FormField
                control={form.control}
                name="catatan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Catatan / Justifikasi</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        value={field.value ?? ""}
                        rows={4}
                        placeholder="Jelaskan dasar penetapan multiplier ini (sumber data makroekonomi, outlook, dll.)..."
                        maxLength={1000}
                      />
                    </FormControl>
                    <FormDescription>
                      Maks 1000 karakter. Sertakan referensi sumber data (Pefindo, BI, BPS, dll.) jika ada.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>

          {/* Footer */}
          <div className="mt-6 flex justify-end gap-3 border-t pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={handleCancelClick}
              disabled={submitting}
            >
              Batal
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? "Menyimpan..." : "Simpan"}
            </Button>
          </div>
        </form>
      </Form>

      {/* Unsaved changes confirm dialog */}
      <Dialog open={unsavedDialogOpen} onOpenChange={setUnsavedDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Yakin ingin meninggalkan halaman?</DialogTitle>
            <DialogDescription>Data yang sudah diisi akan hilang.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setUnsavedDialogOpen(false)}>
              Tetap di Sini
            </Button>
            <Button variant="destructive" onClick={handleConfirmLeave}>
              Keluar Tanpa Menyimpan
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

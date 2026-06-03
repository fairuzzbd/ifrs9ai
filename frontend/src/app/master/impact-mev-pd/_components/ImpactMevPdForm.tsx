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
import { impactMevPdApi } from "@/lib/api/impact-mev-pd.api";
import { periodeBukuApi } from "@/lib/api/periode-buku.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  impactMevPdCreateSchema,
  impactMevPdUpdateSchema,
  type ImpactMevPdCreateInput,
  type ImpactMevPdUpdateInput,
  type ImpactMevPdItem,
} from "@/lib/schemas/impact-mev-pd.schema";

// ---------------------------------------------------------------------------
// Range guidance constants (informational, not hard reject)
// ---------------------------------------------------------------------------

const MULTIPLIER_PLAUSIBLE_MIN = 0.5;
const MULTIPLIER_PLAUSIBLE_MAX = 2.5;

function getMultiplierWarning(skenario: string, value: string): string | null {
  const n = Number(value);
  if (Number.isNaN(n) || value === "") return null;

  if (skenario === "GOOD" && n >= 1.0) {
    return "Multiplier GOOD biasanya < 1.0 (perbaikan kondisi ekonomi). Nilai >= 1.0 tidak diblok, namun perlu justifikasi.";
  }
  if (skenario === "BAD" && n <= 1.0) {
    return "Multiplier BAD biasanya > 1.0 (memburuknya kondisi ekonomi). Nilai <= 1.0 tidak diblok, namun perlu justifikasi.";
  }
  if (n < MULTIPLIER_PLAUSIBLE_MIN || n > MULTIPLIER_PLAUSIBLE_MAX) {
    return `Nilai di luar rentang plausibel ${MULTIPLIER_PLAUSIBLE_MIN}–${MULTIPLIER_PLAUSIBLE_MAX}. Pastikan ini benar.`;
  }
  return null;
}

// ---------------------------------------------------------------------------
// JSON editor inline validation
// ---------------------------------------------------------------------------

function validateJsonObject(val: string): string | null {
  if (!val.trim()) return null;
  try {
    const parsed = JSON.parse(val);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return "Harus berupa JSON object (mis. {\"gdp\": 0.4, \"inflation\": 0.6})";
    }
    return null;
  } catch {
    return "JSON tidak valid. Periksa format penulisan.";
  }
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ImpactMevPdFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<ImpactMevPdItem>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ImpactMevPdForm({ mode, defaultValues }: ImpactMevPdFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<string | null>(null);
  const [jsonError, setJsonError] = React.useState<string | null>(null);

  const isEdit = mode === "edit";

  // ---------------------------------------------------------------------------
  // Periode dropdown — preload all approved/open periods (small dataset)
  // ---------------------------------------------------------------------------

  const { data: periodeData, isLoading: periodeLoading } = useQuery({
    queryKey: ["periode-buku-dropdown"],
    queryFn: () => periodeBukuApi.list({ limit: 200, sort: "periode_bulan:desc" }),
    staleTime: 5 * 60_000, // 5 minutes — periods change infrequently
  });

  const periodeOptions = periodeData?.data ?? [];

  // ---------------------------------------------------------------------------
  // Form setup
  // ---------------------------------------------------------------------------

  const form = useForm<ImpactMevPdCreateInput>({
    resolver: zodResolver(impactMevPdCreateSchema),
    defaultValues: {
      periodeId: defaultValues?.periodeId ?? "",
      skenario: defaultValues?.skenario ?? "GOOD",
      impactMultiplier: defaultValues?.impactMultiplier ?? "",
      mevComponentsJson: defaultValues?.mevComponentsJson ?? "",
      catatan: defaultValues?.catatan ?? "",
    },
  });

  const { isDirty } = form.formState;

  const watchedSkenario = form.watch("skenario");
  const watchedMultiplier = form.watch("impactMultiplier");
  const multiplierWarning = getMultiplierWarning(watchedSkenario, watchedMultiplier);

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/impact-mev-pd");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/impact-mev-pd");
    }
  };

  const handleConfirmLeave = () => {
    if (pendingNavigation) router.push(pendingNavigation);
    setUnsavedDialogOpen(false);
  };

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const onSubmit = async (values: ImpactMevPdCreateInput) => {
    // Re-validate JSON inline before submitting
    if (values.mevComponentsJson && values.mevComponentsJson.trim() !== "") {
      const err = validateJsonObject(values.mevComponentsJson);
      if (err) {
        setJsonError(err);
        return;
      }
    }
    setJsonError(null);

    setSubmitting(true);
    const idempotencyKey = uuidv4();

    // Trim empty optional strings before sending
    const mevJson = values.mevComponentsJson?.trim() || undefined;
    const catatan = values.catatan?.trim() || undefined;

    try {
      if (isEdit && defaultValues?.id && defaultValues.rowVersion !== undefined) {
        const updateData: ImpactMevPdUpdateInput = {
          impactMultiplier: values.impactMultiplier,
          mevComponentsJson: mevJson,
          catatan,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await impactMevPdApi.update(defaultValues.id, updateData, idempotencyKey);
        notify.success(
          `Impact MEV-PD berhasil diperbarui. Menunggu review.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/impact-mev-pd/${res.data.id}`),
            },
          },
        );
        router.push(`/master/impact-mev-pd/${res.data.id}`);
      } else {
        const res = await impactMevPdApi.create(
          { ...values, mevComponentsJson: mevJson, catatan },
          idempotencyKey,
        );
        notify.success(
          `Impact MEV-PD (${res.data.skenario}) berhasil dibuat. Menunggu review ALCO.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/impact-mev-pd/${res.data.id}`),
            },
          },
        );
        router.push(`/master/impact-mev-pd/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace("body.", "") as keyof ImpactMevPdCreateInput;
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

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
                      {isEdit && (
                        <FormDescription>Periode tidak bisa diubah setelah dibuat.</FormDescription>
                      )}
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Skenario */}
                <FormField
                  control={form.control}
                  name="skenario"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Skenario{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                        disabled={isEdit}
                      >
                        <FormControl>
                          <SelectTrigger
                            aria-required="true"
                            className={cn(isEdit && "bg-muted cursor-not-allowed")}
                          >
                            <SelectValue placeholder="Pilih skenario" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="GOOD">GOOD — Kondisi baik</SelectItem>
                          <SelectItem value="BAD">BAD — Kondisi buruk</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        NORMAL tidak disimpan — multiplier-nya selalu 1.0 (DEC-010).
                        {isEdit && (
                          <span className="ml-1 text-muted-foreground">(tidak bisa diubah)</span>
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* Section: Multiplier */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Forward-Looking Multiplier
              </h2>

              {/* Range guidance banner (DEC-010) */}
              <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-800">
                <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
                <div>
                  <span className="font-medium">Panduan range multiplier (DEC-010):</span>
                  <ul className="mt-1 list-disc pl-4 space-y-0.5">
                    <li>
                      <strong>GOOD</strong>: biasanya &lt; 1.0 — kondisi makroekonomi membaik, PD turun
                    </li>
                    <li>
                      <strong>BAD</strong>: biasanya &gt; 1.0 — kondisi makroekonomi memburuk, PD naik
                    </li>
                    <li>
                      Rentang plausibel: 0.5–2.5. Nilai di luar rentang ini tidak diblok tetapi memerlukan justifikasi.
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
                        placeholder="mis. 0.85 (GOOD) atau 1.20 (BAD)"
                        aria-required="true"
                        inputMode="decimal"
                        className="font-mono"
                      />
                    </FormControl>
                    <FormDescription>
                      Angka desimal, mis. 0.85 atau 1.20. Disimpan dengan presisi 8 desimal.
                    </FormDescription>
                    {multiplierWarning && (
                      <div
                        className="mt-1.5 flex items-start gap-1.5 rounded-md border border-amber-200 bg-amber-50 px-2.5 py-2 text-xs text-amber-800"
                        role="alert"
                        aria-live="polite"
                      >
                        <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
                        {multiplierWarning}
                      </div>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            {/* Section: MEV Components JSON */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Komponen MEV (Opsional)
              </h2>

              <FormField
                control={form.control}
                name="mevComponentsJson"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>MEV Components JSON</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        value={field.value ?? ""}
                        rows={6}
                        placeholder={`{\n  "gdp_growth": 0.40,\n  "inflation": 0.30,\n  "unemployment": 0.30\n}`}
                        className="font-mono text-xs"
                        aria-describedby="mev-json-hint mev-json-error"
                        onChange={(e) => {
                          field.onChange(e.target.value);
                          // Live validation for instant feedback
                          const err = validateJsonObject(e.target.value);
                          setJsonError(err);
                        }}
                      />
                    </FormControl>
                    <FormDescription id="mev-json-hint">
                      JSON object berisi bobot komponen MEV (Macroeconomic Variables).
                      Jika kolom &ldquo;weights&rdquo; ada, jumlahnya harus = 1.0.
                      Kosongkan jika tidak diperlukan.
                    </FormDescription>
                    {jsonError && (
                      <p
                        id="mev-json-error"
                        className="text-[0.8rem] font-medium text-destructive"
                        role="alert"
                      >
                        {jsonError}
                      </p>
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
                        rows={3}
                        placeholder="Jelaskan dasar penetapan multiplier ini..."
                        maxLength={1000}
                      />
                    </FormControl>
                    <FormDescription>Maks 1000 karakter. Wajib jika nilai di luar rentang plausibel.</FormDescription>
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
            <Button type="submit" disabled={submitting || !!jsonError}>
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

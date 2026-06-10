"use client";

import * as React from "react";
import { useForm, useWatch, type Control, type Resolver } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { v4 as uuidv4 } from "uuid";
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

import { pdPefindoApi } from "@/lib/api/pd-pefindo.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  pdPefindoCreateSchema,
  pdPefindoUpdateSchema,
  checkMonotonicity,
  PEFINDO_RATINGS,
  type PDPefindoCreateInput,
  type PDPefindoUpdateInput,
  type PDPefindoItem,
} from "@/lib/schemas/pd-pefindo.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface PDPefindoFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<PDPefindoItem> & { rowVersion?: number };
}

// ---------------------------------------------------------------------------
// PD field row helper
// ---------------------------------------------------------------------------

function PDFieldRow({
  label,
  name,
  required,
  description,
  control,
}: {
  label: string;
  name: keyof PDPefindoCreateInput;
  required?: boolean;
  description?: string;
  control: Control<PDPefindoCreateInput>;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>
            {label}
            {required && (
              <span className="text-destructive ml-1" aria-hidden>
                *
              </span>
            )}
          </FormLabel>
          <FormControl>
            <Input
              {...field}
              value={field.value ?? ""}
              placeholder="0.00000000"
              className="font-mono"
              aria-required={required ? "true" : "false"}
            />
          </FormControl>
          {description && (
            <FormDescription>{description}</FormDescription>
          )}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

// ---------------------------------------------------------------------------
// Monotonicity preview banner
// ---------------------------------------------------------------------------

function MonotonicityWarning({ errors }: { errors: string[] }) {
  if (errors.length === 0) return null;
  return (
    <div
      role="alert"
      className="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 px-3 py-2"
    >
      <AlertTriangle
        className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
        aria-hidden
      />
      <div className="text-sm text-amber-800">
        <p className="font-medium">Peringatan monotonicity PD:</p>
        <ul className="mt-1 list-inside list-disc space-y-0.5">
          {errors.map((e, i) => (
            <li key={i}>{e}</li>
          ))}
        </ul>
        <p className="mt-1 text-xs text-amber-700">
          Submit akan gagal jika constraint ini tidak dipenuhi.
        </p>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// idD notice banner
// ---------------------------------------------------------------------------

function IdDNoticeBanner() {
  return (
    <div
      role="note"
      className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2"
    >
      <Info className="mt-0.5 h-4 w-4 shrink-0 text-blue-600" aria-hidden />
      <p className="text-sm text-blue-800">
        Rating <strong>idD</strong> adalah certain default — semua nilai PD harus
        1.0 (atau 1.00000000).
      </p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function PDPefindoForm({ mode, defaultValues }: PDPefindoFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<
    string | null
  >(null);

  const isEdit = mode === "edit";

  const form = useForm<PDPefindoCreateInput>({
    resolver: zodResolver(pdPefindoCreateSchema) as Resolver<PDPefindoCreateInput>,
    defaultValues: {
      rating: (defaultValues?.rating as PDPefindoCreateInput["rating"]) ?? "idAAA",
      pd12Month: defaultValues?.pd12Month ?? "",
      pdLifetime3Y: defaultValues?.pdLifetime3Y ?? "",
      pdLifetime5Y: defaultValues?.pdLifetime5Y ?? "",
      pdLifetime7Y: defaultValues?.pdLifetime7Y ?? "",
      pdLifetime10Y: defaultValues?.pdLifetime10Y ?? "",
      sumber: defaultValues?.sumber ?? "PEFINDO_ANNUAL_DEFAULT_STUDY",
      tanggalPublikasi: defaultValues?.tanggalPublikasi ?? "",
      periodeBerlakuDari: defaultValues?.periodeBerlakuDari ?? "",
      periodeBerlakuSampai: defaultValues?.periodeBerlakuSampai ?? "",
    },
  });

  const { isDirty } = form.formState;

  // Watch PD fields for live monotonicity preview
  const pd12Month = useWatch({ control: form.control, name: "pd12Month" });
  const pdLifetime3Y = useWatch({ control: form.control, name: "pdLifetime3Y" });
  const pdLifetime5Y = useWatch({ control: form.control, name: "pdLifetime5Y" });
  const pdLifetime7Y = useWatch({ control: form.control, name: "pdLifetime7Y" });
  const pdLifetime10Y = useWatch({ control: form.control, name: "pdLifetime10Y" });
  const selectedRating = useWatch({ control: form.control, name: "rating" });

  const monoWarnings = React.useMemo(
    () =>
      checkMonotonicity({
        pd12Month: pd12Month ?? "",
        pdLifetime3Y: pdLifetime3Y ?? "",
        pdLifetime5Y: pdLifetime5Y ?? "",
        pdLifetime7Y: pdLifetime7Y ?? "",
        pdLifetime10Y: pdLifetime10Y ?? "",
      }),
    [pd12Month, pdLifetime3Y, pdLifetime5Y, pdLifetime7Y, pdLifetime10Y],
  );

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/pd-pefindo");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/pd-pefindo");
    }
  };

  const handleConfirmLeave = () => {
    if (pendingNavigation) router.push(pendingNavigation);
    setUnsavedDialogOpen(false);
  };

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const onSubmit = async (values: PDPefindoCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (isEdit && defaultValues?.id && defaultValues.rowVersion !== undefined) {
        const updateData: PDPefindoUpdateInput = {
          pd12Month: values.pd12Month,
          pdLifetime3Y: values.pdLifetime3Y || undefined,
          pdLifetime5Y: values.pdLifetime5Y || undefined,
          pdLifetime7Y: values.pdLifetime7Y || undefined,
          pdLifetime10Y: values.pdLifetime10Y || undefined,
          sumber: values.sumber,
          tanggalPublikasi: values.tanggalPublikasi || undefined,
          periodeBerlakuDari: values.periodeBerlakuDari,
          periodeBerlakuSampai: values.periodeBerlakuSampai || undefined,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await pdPefindoApi.update(
          defaultValues.id,
          updateData,
          idempotencyKey,
        );
        notify.success(
          `PD Pefindo rating ${res.data.rating} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/pd-pefindo/${res.data.id}`),
            },
          },
        );
        router.push(`/master/pd-pefindo/${res.data.id}`);
      } else {
        const res = await pdPefindoApi.create(values, idempotencyKey);
        notify.success(
          `PD Pefindo rating ${res.data.rating} berhasil dibuat. Menunggu review Risk Officer.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/pd-pefindo/${res.data.id}`),
            },
          },
        );
        router.push(`/master/pd-pefindo/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace(
            "body.",
            "",
          ) as keyof PDPefindoCreateInput;
          form.setError(fieldName, { message: d.message });
        });

        if (err.code === "CONFLICT") {
          notify.error(err, {
            action: {
              label: "Muat ulang",
              onClick: () => router.refresh(),
            },
          });
          return;
        }

        if (err.details.length > 0) {
          notify.error({
            ...err,
            message: `${err.details.length} field bermasalah — lihat form di bawah.`,
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

  const showReturnedBanner =
    isEdit && defaultValues?.workflowStatus === "RETURNED";

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <>
      {showReturnedBanner && (
        <ReturnedBanner
          rejectedBy="Risk Officer / Finance Controller"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Section: Rating & Periode */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Identitas & Periode
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Rating */}
                <FormField
                  control={form.control}
                  name="rating"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Rating Pefindo{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                        disabled={isEdit}
                      >
                        <FormControl>
                          <SelectTrigger
                            aria-required="true"
                            className={isEdit ? "bg-muted cursor-not-allowed" : ""}
                          >
                            <SelectValue placeholder="Pilih rating" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {PEFINDO_RATINGS.map((r) => (
                            <SelectItem key={r} value={r}>
                              {r}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {isEdit && (
                        <FormDescription>
                          Rating tidak dapat diubah setelah dibuat.
                        </FormDescription>
                      )}
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Sumber */}
                <FormField
                  control={form.control}
                  name="sumber"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Sumber{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="PEFINDO_ANNUAL_DEFAULT_STUDY"
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>
                        Default: PEFINDO_ANNUAL_DEFAULT_STUDY
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tanggal Publikasi */}
                <FormField
                  control={form.control}
                  name="tanggalPublikasi"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Tanggal Publikasi</FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          {...field}
                          value={field.value ?? ""}
                        />
                      </FormControl>
                      <FormDescription>
                        Tanggal terbit laporan Pefindo Annual Default Study
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Periode Berlaku Dari */}
                <FormField
                  control={form.control}
                  name="periodeBerlakuDari"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Periode Berlaku Dari{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input type="date" {...field} aria-required="true" />
                      </FormControl>
                      <FormDescription>
                        Awal periode efektivitas PD ini untuk kalkulasi ECL
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Periode Berlaku Sampai */}
                <FormField
                  control={form.control}
                  name="periodeBerlakuSampai"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Periode Berlaku Sampai</FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          {...field}
                          value={field.value ?? ""}
                        />
                      </FormControl>
                      <FormDescription>
                        Opsional — kosongkan jika belum ada batas akhir
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* Section: PD Values */}
            <div className="rounded-lg border p-6 space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                  Nilai PD (Probability of Default)
                </h2>
                <span className="text-xs text-muted-foreground">
                  Desimal 0–1 · presisi 8 digit · pd_12m ≤ 3y ≤ 5y ≤ 7y ≤ 10y
                </span>
              </div>

              {selectedRating === "idD" && <IdDNoticeBanner />}

              {monoWarnings.length > 0 && (
                <MonotonicityWarning errors={monoWarnings} />
              )}

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <PDFieldRow
                  control={form.control}
                  name="pd12Month"
                  label="PD 12 Bulan"
                  required
                  description="Stage 1 — 12-month PD"
                />
                <PDFieldRow
                  control={form.control}
                  name="pdLifetime3Y"
                  label="PD Lifetime 3 Tahun"
                  description="Stage 2 — Lifetime PD (3Y)"
                />
                <PDFieldRow
                  control={form.control}
                  name="pdLifetime5Y"
                  label="PD Lifetime 5 Tahun"
                  description="Stage 2 — Lifetime PD (5Y)"
                />
                <PDFieldRow
                  control={form.control}
                  name="pdLifetime7Y"
                  label="PD Lifetime 7 Tahun"
                  description="Stage 2 — Lifetime PD (7Y)"
                />
                <PDFieldRow
                  control={form.control}
                  name="pdLifetime10Y"
                  label="PD Lifetime 10 Tahun"
                  description="Stage 2 — Lifetime PD (10Y)"
                />
              </div>
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
            <Button type="submit" disabled={submitting || monoWarnings.length > 0}>
              {submitting ? "Menyimpan..." : "Simpan"}
            </Button>
          </div>
        </form>
      </Form>

      {/* Unsaved changes guard */}
      <Dialog open={unsavedDialogOpen} onOpenChange={setUnsavedDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Yakin ingin meninggalkan halaman?</DialogTitle>
            <DialogDescription>
              Data yang sudah diisi akan hilang.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setUnsavedDialogOpen(false)}
            >
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

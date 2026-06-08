"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { v4 as uuidv4 } from "uuid";
import { Info } from "lucide-react";
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
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ReturnedBanner } from "@/components/blips/ReturnedBanner";
import { lpsCoverageApi } from "@/lib/api/lps-coverage.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  lpsCoverageCreateSchema,
  lpsCoverageUpdateSchema,
  formatIDR,
  DEFAULT_COVERAGE_AMOUNT,
  type LPSCoverageCreateInput,
  type LPSCoverageUpdateInput,
  type LPSCoverageItem,
} from "@/lib/schemas/lps-coverage.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface LPSCoverageFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<LPSCoverageItem>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function LPSCoverageForm({ mode, defaultValues }: LPSCoverageFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<
    string | null
  >(null);
  // Display value for coverage_amount (formatted) — separate from form value (raw decimal string)
  const [displayAmount, setDisplayAmount] = React.useState<string>("");

  const isEdit = mode === "edit";

  const form = useForm<LPSCoverageCreateInput>({
    resolver: zodResolver(lpsCoverageCreateSchema),
    defaultValues: {
      coverageAmount:
        defaultValues?.coverageAmount ?? DEFAULT_COVERAGE_AMOUNT,
      periodeBerlakuDari:
        defaultValues?.periodeBerlakuDari ??
        new Date().toISOString().split("T")[0],
      periodeBerlakuSampai: defaultValues?.periodeBerlakuSampai ?? null,
      regulasiReferensi: defaultValues?.regulasiReferensi ?? "",
    },
  });

  const { isDirty } = form.formState;

  // Init display amount from form value
  React.useEffect(() => {
    const raw = form.getValues("coverageAmount");
    if (raw) {
      setDisplayAmount(formatIDR(raw));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/lps-coverage");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/lps-coverage");
    }
  };

  const handleConfirmLeave = () => {
    if (pendingNavigation) {
      router.push(pendingNavigation);
    }
    setUnsavedDialogOpen(false);
  };

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const onSubmit = async (values: LPSCoverageCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (
        isEdit &&
        defaultValues?.id &&
        defaultValues.rowVersion !== undefined
      ) {
        const updateData: LPSCoverageUpdateInput = {
          ...values,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await lpsCoverageApi.update(
          defaultValues.id,
          updateData,
          idempotencyKey,
        );
        notify.success(
          `LPS Coverage cap ${formatIDR(res.data.coverageAmount)} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/lps-coverage/${res.data.id}`),
            },
          },
        );
        router.push(`/master/lps-coverage/${res.data.id}`);
      } else {
        const res = await lpsCoverageApi.create(values, idempotencyKey);
        notify.success(
          `LPS Coverage cap ${formatIDR(res.data.coverageAmount)} berhasil dibuat. Menunggu review.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/lps-coverage/${res.data.id}`),
            },
          },
        );
        router.push(`/master/lps-coverage/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace(
            "body.",
            "",
          ) as keyof LPSCoverageCreateInput;
          form.setError(fieldName, { message: d.message });
        });

        if (err.code === "LPS_PERIOD_OVERLAP") {
          form.setError("periodeBerlakuDari", {
            message:
              "Periode berlaku overlap dengan LPS coverage yang sudah aktif. Tutup periode sebelumnya terlebih dahulu.",
          });
          notify.error(err);
          return;
        }

        if (err.code === "CONFLICT" && err.details.length === 0) {
          notify.error(err, {
            action: {
              label: "Muat ulang",
              onClick: () => router.refresh(),
            },
          });
          return;
        }

        const fieldErrorCount = err.details.length;
        if (fieldErrorCount > 0) {
          notify.error({
            ...err,
            message: `${fieldErrorCount} field bermasalah — lihat form di bawah.`,
          });
          setTimeout(() => {
            const firstErrorEl = document.querySelector("[aria-invalid='true']");
            firstErrorEl?.scrollIntoView({
              behavior: "smooth",
              block: "center",
            });
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

  const showReturnedBanner =
    isEdit && defaultValues?.workflowStatus === "RETURNED";

  return (
    <>
      {showReturnedBanner && (
        <ReturnedBanner
          rejectedBy="Finance Controller / Approver"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      {/* DEC-014 info banner */}
      <div
        role="note"
        aria-label="Informasi parameter ECL"
        className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 mb-6"
      >
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden />
        <p className="text-sm text-amber-800">
          <strong>Parameter ECL — DEC-014:</strong> LPS coverage cap mempengaruhi
          LPS Aggregator di ECL Stage 1/2/3. Perubahan cap akan berlaku pada
          periode berjalan setelah disetujui. Koordinasikan dengan Risk Officer
          dan ALCO sebelum mengubah nilai default.
        </p>
      </div>

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Section: Coverage Amount */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Coverage LPS
              </h2>

              {/* Mata Uang — read-only IDR badge */}
              <div className="flex flex-col gap-1.5">
                <span className="text-sm font-medium">Mata Uang</span>
                <div className="flex items-center gap-2">
                  <Badge variant="outline" className="text-sm font-mono font-bold">
                    IDR
                  </Badge>
                  <span className="text-xs text-muted-foreground">
                    Tetap IDR sesuai regulasi LPS (DEC-014)
                  </span>
                </div>
              </div>

              {/* Coverage Amount */}
              <FormField
                control={form.control}
                name="coverageAmount"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Jumlah Coverage Cap{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <FormControl>
                      <div className="relative">
                        <Input
                          type="text"
                          inputMode="numeric"
                          placeholder="2000000000.0000"
                          value={displayAmount}
                          aria-required="true"
                          aria-describedby="coverage-amount-hint"
                          onChange={(e) => {
                            // Allow user to type freely — store raw on blur
                            setDisplayAmount(e.target.value);
                          }}
                          onBlur={(e) => {
                            // Strip any IDR formatting back to raw decimal
                            const raw = e.target.value
                              .replace(/Rp\s?/g, "")
                              .replace(/\./g, "")
                              .replace(/,/g, ".")
                              .trim();
                            field.onChange(raw);
                            // Re-format for display
                            const asFloat = parseFloat(raw);
                            if (!isNaN(asFloat) && asFloat > 0) {
                              setDisplayAmount(
                                new Intl.NumberFormat("id-ID", {
                                  style: "currency",
                                  currency: "IDR",
                                  minimumFractionDigits: 2,
                                  maximumFractionDigits: 2,
                                }).format(asFloat),
                              );
                            } else {
                              // Keep raw for error display
                              field.onChange(raw || "0");
                            }
                          }}
                          onFocus={(e) => {
                            // On focus: show raw decimal for easy editing
                            const raw = field.value ?? "";
                            setDisplayAmount(raw);
                            e.target.select();
                          }}
                        />
                      </div>
                    </FormControl>
                    <FormDescription id="coverage-amount-hint">
                      Format: angka tanpa pemisah, mis.{" "}
                      <code>2000000000.00</code>. Default IDR 2 miliar
                      (DEC-014). Saat ini:{" "}
                      <strong>
                        {field.value ? formatIDR(field.value) : "—"}
                      </strong>
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            {/* Section: Periode Berlaku */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Periode Berlaku
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Periode Berlaku Dari */}
                <FormField
                  control={form.control}
                  name="periodeBerlakuDari"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Berlaku Dari{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          {...field}
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>
                        Tanggal mulai berlaku coverage cap ini
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
                      <FormLabel>Berlaku Sampai</FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          value={field.value ?? ""}
                          onChange={(e) =>
                            field.onChange(e.target.value || null)
                          }
                          min={form.watch("periodeBerlakuDari")}
                        />
                      </FormControl>
                      <FormDescription>
                        Kosongkan jika coverage ini masih berlaku (periode
                        aktif)
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className="rounded-md bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
                Hanya boleh ada satu coverage cap aktif (periode terbuka) pada
                satu waktu. Jika menambah periode baru, tutup periode aktif
                saat ini terlebih dahulu.
              </div>
            </div>

            {/* Section: Referensi Regulasi */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Referensi Regulasi
              </h2>

              <FormField
                control={form.control}
                name="regulasiReferensi"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Referensi Regulasi</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder="Contoh: Peraturan LPS No. 2/PLPS/2010 — Pasal 3 Ayat 1"
                        rows={3}
                        {...field}
                        value={field.value ?? ""}
                      />
                    </FormControl>
                    <FormDescription>
                      Nomor/nama peraturan LPS atau regulasi terkait (opsional,
                      maks 200 karakter)
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

"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { v4 as uuidv4 } from "uuid";
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
import { cn } from "@/lib/utils";
import { periodeBukuApi } from "@/lib/api/periode-buku.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  periodeBukuCreateSchema,
  periodeBukuUpdateSchema,
  type PeriodeBukuCreateInput,
  type PeriodeBukuUpdateInput,
  type PeriodeBukuItem,
  type TipePeriode,
} from "@/lib/schemas/periode-buku.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface PeriodeBukuFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<PeriodeBukuItem>;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const BULAN_OPTIONS = [
  { value: 1, label: "Januari" },
  { value: 2, label: "Februari" },
  { value: 3, label: "Maret" },
  { value: 4, label: "April" },
  { value: 5, label: "Mei" },
  { value: 6, label: "Juni" },
  { value: 7, label: "Juli" },
  { value: 8, label: "Agustus" },
  { value: 9, label: "September" },
  { value: 10, label: "Oktober" },
  { value: 11, label: "November" },
  { value: 12, label: "Desember" },
];

const TRIWULAN_OPTIONS = [
  { value: 1, label: "Q1 (Jan–Mar)" },
  { value: 2, label: "Q2 (Apr–Jun)" },
  { value: 3, label: "Q3 (Jul–Sep)" },
  { value: 4, label: "Q4 (Okt–Des)" },
];

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function PeriodeBukuForm({ mode, defaultValues }: PeriodeBukuFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<
    string | null
  >(null);

  const isEdit = mode === "edit";
  const currentYear = new Date().getFullYear();

  const form = useForm<PeriodeBukuCreateInput>({
    resolver: zodResolver(periodeBukuCreateSchema),
    defaultValues: {
      periodeIdKode: defaultValues?.periodeIdKode ?? "",
      tipePeriode: defaultValues?.tipePeriode ?? "BULANAN",
      tahunBuku: defaultValues?.tahunBuku ?? currentYear,
      bulan: defaultValues?.bulan ?? null,
      triwulan: defaultValues?.triwulan ?? null,
      tanggalMulai: defaultValues?.tanggalMulai ?? "",
      tanggalAkhir: defaultValues?.tanggalAkhir ?? "",
    },
  });

  const { isDirty } = form.formState;
  const watchTipe = form.watch("tipePeriode");

  // ---------------------------------------------------------------------------
  // Unsaved changes guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/periode-buku");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/periode-buku");
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

  const onSubmit = async (values: PeriodeBukuCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (
        isEdit &&
        defaultValues?.id &&
        defaultValues.rowVersion !== undefined
      ) {
        const updateData: PeriodeBukuUpdateInput = {
          tahunBuku: values.tahunBuku,
          bulan: values.bulan ?? null,
          triwulan: values.triwulan ?? null,
          tanggalMulai: values.tanggalMulai,
          tanggalAkhir: values.tanggalAkhir,
          rowVersion: defaultValues.rowVersion,
          tipePeriode: values.tipePeriode,
        };
        const res = await periodeBukuApi.update(
          defaultValues.id,
          updateData,
          idempotencyKey,
        );
        notify.success(
          `Periode ${res.data.periodeIdKode} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/periode-buku/${res.data.id}`),
            },
          },
        );
        router.push(`/master/periode-buku/${res.data.id}`);
      } else {
        const res = await periodeBukuApi.create(values, idempotencyKey);
        notify.success(
          `Periode ${res.data.periodeIdKode} berhasil dibuat. Menunggu submit ke workflow.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/periode-buku/${res.data.id}`),
            },
          },
        );
        router.push(`/master/periode-buku/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace(
            "body.",
            "",
          ) as keyof PeriodeBukuCreateInput;
          form.setError(fieldName, { message: d.message });
        });

        if (err.code === "CONFLICT" && err.details.length === 0) {
          notify.error(err, {
            action: {
              label: "Muat ulang",
              onClick: () => router.refresh(),
            },
          });
          return;
        }

        if (err.code === "CONFLICT") {
          form.setError("periodeIdKode", {
            message: `Kode periode ${values.periodeIdKode} sudah terdaftar di sistem.`,
          });
        }

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

  const showReturnedBanner =
    isEdit && defaultValues?.workflowStatus === "RETURNED";

  return (
    <>
      {showReturnedBanner && (
        <ReturnedBanner
          rejectedBy="Treasury Approver"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Section: Identitas Periode */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Identitas Periode
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Kode Periode */}
                <FormField
                  control={form.control}
                  name="periodeIdKode"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Kode Periode{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="2026-M06"
                          maxLength={20}
                          className={cn(
                            "font-mono",
                            isEdit && "bg-muted cursor-not-allowed",
                          )}
                          disabled={isEdit}
                          aria-required="true"
                          title={
                            isEdit
                              ? "Kode periode tidak bisa diubah setelah dibuat."
                              : undefined
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        Format: <code>2026-M06</code> (bulanan),{" "}
                        <code>2026-Q2</code> (triwulanan), <code>2026-Y</code>{" "}
                        (tahunan)
                        {isEdit && (
                          <span className="ml-1 text-muted-foreground">
                            (tidak bisa diubah)
                          </span>
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tipe Periode */}
                <FormField
                  control={form.control}
                  name="tipePeriode"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tipe Periode{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={(v) => {
                          field.onChange(v as TipePeriode);
                          // Reset conditional fields
                          form.setValue("bulan", null);
                          form.setValue("triwulan", null);
                        }}
                        disabled={isEdit}
                      >
                        <FormControl>
                          <SelectTrigger
                            aria-required="true"
                            className={cn(isEdit && "bg-muted cursor-not-allowed")}
                          >
                            <SelectValue placeholder="Pilih tipe periode" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="BULANAN">Bulanan</SelectItem>
                          <SelectItem value="TRIWULANAN">Triwulanan</SelectItem>
                          <SelectItem value="TAHUNAN">Tahunan</SelectItem>
                        </SelectContent>
                      </Select>
                      {isEdit && (
                        <FormDescription>Tipe tidak bisa diubah</FormDescription>
                      )}
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tahun Buku */}
                <FormField
                  control={form.control}
                  name="tahunBuku"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tahun Buku{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min={2000}
                          max={2099}
                          {...field}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10))
                          }
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>4 digit, contoh: 2026</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Bulan — only visible for BULANAN */}
                {watchTipe === "BULANAN" && (
                  <FormField
                    control={form.control}
                    name="bulan"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Bulan{" "}
                          <span className="text-destructive" aria-hidden>
                            *
                          </span>
                        </FormLabel>
                        <Select
                          value={
                            field.value !== null && field.value !== undefined
                              ? String(field.value)
                              : ""
                          }
                          onValueChange={(v) =>
                            field.onChange(v ? parseInt(v, 10) : null)
                          }
                        >
                          <FormControl>
                            <SelectTrigger aria-required="true">
                              <SelectValue placeholder="Pilih bulan" />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            {BULAN_OPTIONS.map((opt) => (
                              <SelectItem
                                key={opt.value}
                                value={String(opt.value)}
                              >
                                {opt.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}

                {/* Triwulan — only visible for TRIWULANAN */}
                {watchTipe === "TRIWULANAN" && (
                  <FormField
                    control={form.control}
                    name="triwulan"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Triwulan{" "}
                          <span className="text-destructive" aria-hidden>
                            *
                          </span>
                        </FormLabel>
                        <Select
                          value={
                            field.value !== null && field.value !== undefined
                              ? String(field.value)
                              : ""
                          }
                          onValueChange={(v) =>
                            field.onChange(v ? parseInt(v, 10) : null)
                          }
                        >
                          <FormControl>
                            <SelectTrigger aria-required="true">
                              <SelectValue placeholder="Pilih triwulan" />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            {TRIWULAN_OPTIONS.map((opt) => (
                              <SelectItem
                                key={opt.value}
                                value={String(opt.value)}
                              >
                                {opt.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}
              </div>
            </div>

            {/* Section: Tanggal Berlaku */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Tanggal Berlaku
              </h2>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Tanggal Mulai */}
                <FormField
                  control={form.control}
                  name="tanggalMulai"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tanggal Mulai{" "}
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
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tanggal Akhir */}
                <FormField
                  control={form.control}
                  name="tanggalAkhir"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tanggal Akhir{" "}
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
                        Harus sama dengan atau setelah tanggal mulai
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
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

// Re-export Link for use in page
export { Link };

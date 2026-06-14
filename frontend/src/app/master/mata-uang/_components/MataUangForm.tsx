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
import { Switch } from "@/components/ui/switch";
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
import { mataUangApi } from "@/lib/api/mata-uang.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  mataUangCreateSchema,
  mataUangUpdateSchema,
  type MataUangCreateInput,
  type MataUangUpdateInput,
  type MataUangItem,
} from "@/lib/schemas/mata-uang.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface MataUangFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<MataUangItem>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function MataUangForm({ mode, defaultValues }: MataUangFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<string | null>(null);

  const isEdit = mode === "edit";

  const form = useForm<MataUangCreateInput>({
    resolver: zodResolver(mataUangCreateSchema),
    defaultValues: {
      kodeMataUang: defaultValues?.kodeMataUang ?? "",
      namaMataUang: defaultValues?.namaMataUang ?? "",
      simbol: defaultValues?.simbol ?? "",
      decimalPlaces: defaultValues?.decimalPlaces ?? 2,
      sumberKursDefault: defaultValues?.sumberKursDefault ?? "BI_JISDOR",
      frekuensiUpdate: defaultValues?.frekuensiUpdate ?? "HARIAN",
      tanggalMulaiAktif:
        defaultValues?.tanggalMulaiAktif ??
        new Date().toISOString().split("T")[0],
      aktifFlag: defaultValues?.aktifFlag !== undefined ? defaultValues.aktifFlag : true,
    },
  });

  const { isDirty } = form.formState;

  // ---------------------------------------------------------------------------
  // Handle navigation with unsaved changes
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/mata-uang");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/mata-uang");
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

  const onSubmit = async (values: MataUangCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (isEdit && defaultValues?.kodeMataUang && defaultValues.rowVersion !== undefined) {
        const updateData: MataUangUpdateInput = {
          ...values,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await mataUangApi.update(
          defaultValues.kodeMataUang,
          updateData,
          idempotencyKey,
        );
        notify.success(
          `Mata uang ${res.data.kodeMataUang} — ${res.data.namaMataUang} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/mata-uang/${res.data.kodeMataUang}`),
            },
          },
        );
        router.push(`/master/mata-uang/${res.data.kodeMataUang}`);
      } else {
        const res = await mataUangApi.create(values, idempotencyKey);
        notify.success(
          `Mata uang ${res.data.kodeMataUang} — ${res.data.namaMataUang} berhasil dibuat. Menunggu review Finance Controller.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/mata-uang/${res.data.kodeMataUang}`),
            },
          },
        );
        router.push(`/master/mata-uang/${res.data.kodeMataUang}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        // Set field-level errors from server validation
        err.details.forEach((d) => {
          const fieldName = d.field.replace("body.", "") as keyof MataUangCreateInput;
          form.setError(fieldName, { message: d.message });
        });

        if (err.code === "CONFLICT" && err.details.length === 0) {
          // Row version mismatch
          notify.error(err, {
            action: {
              label: "Muat ulang",
              onClick: () => router.refresh(),
            },
          });
          return;
        }

        if (err.code === "CONFLICT") {
          // Kode already exists
          form.setError("kodeMataUang", {
            message: `Mata uang ${values.kodeMataUang} sudah terdaftar di sistem.`,
          });
        }

        const fieldErrorCount = err.details.length;
        if (fieldErrorCount > 0) {
          notify.error({
            ...err,
            message: `${fieldErrorCount} field bermasalah — lihat form di bawah.`,
          });
          // Scroll to first error
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

  // Find last rejection from history (if available via detail page)
  // In form context we show a generic banner; detail page shows full history.

  return (
    <>
      {showReturnedBanner && (
        <ReturnedBanner
          rejectedBy="Finance Controller"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Section: Informasi Dasar */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Informasi Dasar
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Kode Mata Uang */}
                <FormField
                  control={form.control}
                  name="kodeMataUang"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Kode Mata Uang{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="IDR"
                          maxLength={3}
                          className={cn(
                            "font-mono uppercase",
                            isEdit && "bg-muted cursor-not-allowed",
                          )}
                          disabled={isEdit}
                          aria-required="true"
                          title={
                            isEdit
                              ? "Kode mata uang tidak bisa diubah setelah dibuat."
                              : undefined
                          }
                          onChange={(e) =>
                            field.onChange(e.target.value.toUpperCase())
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        3 huruf kapital, ISO 4217
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

                {/* Nama Mata Uang */}
                <FormField
                  control={form.control}
                  name="namaMataUang"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Nama Mata Uang{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="Rupiah Indonesia"
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>Min 3 karakter</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Simbol */}
                <FormField
                  control={form.control}
                  name="simbol"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Simbol{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="Rp"
                          maxLength={5}
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>Mis: Rp, $, €, £, S$</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Decimal Places */}
                <FormField
                  control={form.control}
                  name="decimalPlaces"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Decimal Places{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min={0}
                          max={4}
                          {...field}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10))
                          }
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>
                        0–4 desimal (IDR=0, USD=2)
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Sumber Kurs Default */}
                <FormField
                  control={form.control}
                  name="sumberKursDefault"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Sumber Kurs Default{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih sumber kurs" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="BI_JISDOR">
                            BI JISDOR
                          </SelectItem>
                          <SelectItem value="BI_KURS_TENGAH">
                            BI Kurs Tengah
                          </SelectItem>
                          <SelectItem value="INTERNAL">Internal</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Frekuensi Update */}
                <FormField
                  control={form.control}
                  name="frekuensiUpdate"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Frekuensi Update{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih frekuensi" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="HARIAN">Harian</SelectItem>
                          <SelectItem value="INTRA_DAY">Intra Day</SelectItem>
                          <SelectItem value="BULANAN">Bulanan</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tanggal Mulai Aktif */}
                <FormField
                  control={form.control}
                  name="tanggalMulaiAktif"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tanggal Mulai Aktif{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          max={new Date().toISOString().split("T")[0]}
                          {...field}
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>
                        Tidak boleh di masa depan
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Status Aktif */}
                <FormField
                  control={form.control}
                  name="aktifFlag"
                  render={({ field }) => (
                    <FormItem className="flex flex-row items-center justify-between rounded-md border p-4">
                      <div>
                        <FormLabel>Status Aktif</FormLabel>
                        <FormDescription className="mt-0.5">
                          Jika tidak aktif, mata uang tidak bisa dipilih di
                          form instrumen baru
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          role="switch"
                          aria-checked={field.value}
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
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


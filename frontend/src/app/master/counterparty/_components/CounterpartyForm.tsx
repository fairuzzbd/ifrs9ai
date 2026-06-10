"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import type { Resolver } from "react-hook-form";
import { useRouter } from "next/navigation";
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
import { Checkbox } from "@/components/ui/checkbox";
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
import { counterpartyApi } from "@/lib/api/counterparty.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  counterpartyCreateSchema,
  counterpartyUpdateSchema,
  type CounterpartyCreateInput,
  type CounterpartyUpdateInput,
  type CounterpartyListItem,
  type CounterpartyDetail,
} from "@/lib/schemas/counterparty.schema";

// ---------------------------------------------------------------------------
// Label maps
// ---------------------------------------------------------------------------

const TIPE_LABELS: Record<string, string> = {
  BANK: "Bank",
  PERUSAHAAN_ASURANSI: "Perusahaan Asuransi",
  PERUSAHAAN_SEKURITAS: "Perusahaan Sekuritas",
  MANAJER_INVESTASI: "Manajer Investasi",
  PEMERINTAH: "Pemerintah / Sovereign",
  KORPORASI: "Korporasi",
  LAINNYA: "Lainnya",
};

const EKSPOSUR_LABELS: Record<string, string> = {
  CORPORATE: "Korporat",
  FINANCIAL_INSTITUTION: "Lembaga Keuangan",
  SOVEREIGN: "Sovereign",
  RETAIL: "Retail",
  OTHER: "Lainnya",
};

const STATUS_LABELS: Record<string, string> = {
  AKTIF: "Aktif",
  TIDAK_AKTIF: "Tidak Aktif",
  DIBLOKIR: "Diblokir",
};

const KATEGORI_MI_LABELS: Record<string, string> = {
  REKSA_DANA: "Reksa Dana",
  MANAJER_INVESTASI: "Manajer Investasi",
  NON_MI: "Non-MI",
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface CounterpartyFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<CounterpartyDetail | CounterpartyListItem>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function CounterpartyForm({ mode, defaultValues }: CounterpartyFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<string | null>(null);

  const isEdit = mode === "edit";

  const form = useForm<CounterpartyCreateInput>({
    // Cast needed: Zod v4 + @hookform/resolvers v5 type mismatch (TS2719)
    resolver: zodResolver(counterpartyCreateSchema) as Resolver<CounterpartyCreateInput>,
    defaultValues: {
      kodeCounterparty: defaultValues?.kodeCounterparty ?? "",
      nama: defaultValues?.nama ?? "",
      tipe: defaultValues?.tipe ?? "BANK",
      tipeEksposurBasel: defaultValues?.tipeEksposurBasel ?? "FINANCIAL_INSTITUTION",
      eligibleLpsFlag: defaultValues?.eligibleLpsFlag ?? false,
      status: defaultValues?.status ?? "AKTIF",
      nomorIzinOjk: defaultValues?.nomorIzinOjk ?? "",
      aumTerakhir: ("aumTerakhir" in (defaultValues ?? {}))
        ? (defaultValues as CounterpartyDetail)?.aumTerakhir ?? ""
        : "",
      kategoriMi: defaultValues?.kategoriMi ?? undefined,
      // PII fields start empty on edit (user must re-enter to change)
      npwp: "",
      nomorRekening: "",
      ktp: "",
    },
  });

  const { isDirty } = form.formState;

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/counterparty");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/counterparty");
    }
  };

  const handleConfirmLeave = () => {
    if (pendingNavigation) router.push(pendingNavigation);
    setUnsavedDialogOpen(false);
  };

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const onSubmit = async (values: CounterpartyCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    // Strip empty PII so backend doesn't overwrite with blank
    const cleaned: CounterpartyCreateInput = {
      ...values,
      npwp: values.npwp || undefined,
      nomorRekening: values.nomorRekening || undefined,
      ktp: values.ktp || undefined,
    };

    try {
      if (isEdit && defaultValues?.id && "rowVersion" in (defaultValues ?? {}) && (defaultValues as CounterpartyListItem).rowVersion !== undefined) {
        const updateData: CounterpartyUpdateInput = {
          ...cleaned,
          rowVersion: (defaultValues as CounterpartyListItem).rowVersion,
        };
        const res = await counterpartyApi.update(
          (defaultValues as CounterpartyListItem).id,
          updateData,
          idempotencyKey,
        );
        notify.success(
          `Counterparty ${res.data.kodeCounterparty} — ${res.data.nama} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/counterparty/${res.data.id}`),
            },
          },
        );
        router.push(`/master/counterparty/${res.data.id}`);
      } else {
        const res = await counterpartyApi.create(cleaned, idempotencyKey);
        notify.success(
          `Counterparty ${res.data.kodeCounterparty} — ${res.data.nama} berhasil dibuat. Menunggu review.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/counterparty/${res.data.id}`),
            },
          },
        );
        router.push(`/master/counterparty/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace("body.", "") as keyof CounterpartyCreateInput;
          form.setError(fieldName, { message: d.message });
        });

        if (err.code === "CONFLICT" && err.details.length === 0) {
          notify.error(err, {
            action: { label: "Muat ulang", onClick: () => router.refresh() },
          });
          return;
        }

        if (err.code === "CONFLICT") {
          form.setError("kodeCounterparty", {
            message: `Kode counterparty ${values.kodeCounterparty} sudah terdaftar.`,
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
        notify.error({ code: "INTERNAL", message: "Terjadi kesalahan. Coba lagi.", traceId: "" });
      }
    } finally {
      setSubmitting(false);
    }
  };

  const showReturnedBanner = isEdit && defaultValues?.workflowStatus === "RETURNED";

  return (
    <>
      {showReturnedBanner && (
        <ReturnedBanner
          rejectedBy="Reviewer"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Informasi Dasar */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Informasi Dasar
              </h2>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Kode */}
                <FormField
                  control={form.control}
                  name="kodeCounterparty"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Kode Counterparty{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="BCA-001"
                          disabled={isEdit}
                          className={isEdit ? "bg-muted cursor-not-allowed font-mono" : "font-mono"}
                          aria-required="true"
                          onChange={(e) => field.onChange(e.target.value.toUpperCase())}
                        />
                      </FormControl>
                      <FormDescription>
                        Huruf kapital, angka, - atau _{isEdit && " (tidak bisa diubah)"}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Nama */}
                <FormField
                  control={form.control}
                  name="nama"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Nama{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input {...field} placeholder="PT Bank Central Asia Tbk" aria-required="true" />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tipe */}
                <FormField
                  control={form.control}
                  name="tipe"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tipe Counterparty{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih tipe" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {Object.entries(TIPE_LABELS).map(([val, label]) => (
                            <SelectItem key={val} value={val}>{label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tipe Eksposur Basel */}
                <FormField
                  control={form.control}
                  name="tipeEksposurBasel"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tipe Eksposur Basel / LGD Pool{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih tipe eksposur" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {Object.entries(EKSPOSUR_LABELS).map(([val, label]) => (
                            <SelectItem key={val} value={val}>{label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        Digunakan untuk penentuan LGD pool dalam kalkulasi ECL
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Status */}
                <FormField
                  control={form.control}
                  name="status"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Status{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih status" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {Object.entries(STATUS_LABELS).map(([val, label]) => (
                            <SelectItem key={val} value={val}>{label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Eligible LPS Flag */}
                <FormField
                  control={form.control}
                  name="eligibleLpsFlag"
                  render={({ field }) => (
                    <FormItem className="flex flex-row items-start gap-3 rounded-md border p-4">
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={field.onChange}
                          aria-label="Counterparty eligible untuk proteksi LPS"
                        />
                      </FormControl>
                      <div className="space-y-1 leading-none">
                        <FormLabel>Eligible LPS</FormLabel>
                        <FormDescription>
                          Centang jika counterparty adalah bank yang dijamin LPS (IDR 2 miliar per nasabah)
                        </FormDescription>
                      </div>
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* Informasi Tambahan */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Informasi Tambahan
              </h2>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Nomor Izin OJK */}
                <FormField
                  control={form.control}
                  name="nomorIzinOjk"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Nomor Izin OJK</FormLabel>
                      <FormControl>
                        <Input {...field} value={field.value ?? ""} placeholder="KEP-000/D.01/2024" />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* AUM Terakhir */}
                <FormField
                  control={form.control}
                  name="aumTerakhir"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>AUM Terakhir (IDR)</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ""}
                          placeholder="1000000000000"
                          inputMode="decimal"
                        />
                      </FormControl>
                      <FormDescription>
                        Assets Under Management — untuk Manajer Investasi
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Kategori MI */}
                <FormField
                  control={form.control}
                  name="kategoriMi"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Kategori MI</FormLabel>
                      <Select value={field.value ?? ""} onValueChange={(v) => field.onChange(v || undefined)}>
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue placeholder="Pilih kategori (opsional)" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="">— Tidak ada —</SelectItem>
                          {Object.entries(KATEGORI_MI_LABELS).map(([val, label]) => (
                            <SelectItem key={val} value={val}>{label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* Data PII */}
            <div className="rounded-lg border border-amber-200 bg-amber-50 p-6 space-y-4">
              <div>
                <h2 className="text-sm font-semibold uppercase tracking-wide text-amber-800">
                  Data Pribadi / PII
                </h2>
                <p className="mt-1 text-xs text-amber-700">
                  Data ini dienkripsi at-rest (AES-256-GCM). Hanya pengguna bererizin yang dapat melihat data plaintext.
                  {isEdit && " Kosongkan field jika tidak ingin mengubah PII yang tersimpan."}
                </p>
              </div>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* NPWP */}
                <FormField
                  control={form.control}
                  name="npwp"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>NPWP</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ""}
                          placeholder="XX.XXX.XXX.X-XXX.XXX"
                          autoComplete="off"
                          data-1p-ignore
                        />
                      </FormControl>
                      <FormDescription>Format: 00.000.000.0-000.000</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Nomor Rekening */}
                <FormField
                  control={form.control}
                  name="nomorRekening"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Nomor Rekening</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ""}
                          placeholder="1234567890"
                          autoComplete="off"
                          data-1p-ignore
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* KTP */}
                <FormField
                  control={form.control}
                  name="ktp"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Nomor KTP / NIK</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ""}
                          placeholder="3174xxxxxxxxxxxxxxxx"
                          maxLength={16}
                          autoComplete="off"
                          data-1p-ignore
                        />
                      </FormControl>
                      <FormDescription>16 digit angka</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>
          </div>

          {/* Footer */}
          <div className="mt-6 flex justify-end gap-3 border-t pt-4">
            <Button type="button" variant="outline" onClick={handleCancelClick} disabled={submitting}>
              Batal
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? "Menyimpan..." : "Simpan"}
            </Button>
          </div>
        </form>
      </Form>

      {/* Unsaved changes dialog */}
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

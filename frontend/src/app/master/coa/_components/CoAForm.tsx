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
import { coaApi } from "@/lib/api/coa.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  coaCreateSchema,
  coaUpdateSchema,
  type CoACreateInput,
  type CoAUpdateInput,
  type CoAItem,
} from "@/lib/schemas/coa.schema";

// ---------------------------------------------------------------------------
// Parent akun autocomplete
// ---------------------------------------------------------------------------

interface ParentAkunOption {
  id: string;
  kodeAkun: string;
  namaAkun: string;
}

function ParentAkunCombobox({
  value,
  onChange,
  disabled,
}: {
  value: string | null | undefined;
  onChange: (id: string | null) => void;
  disabled?: boolean;
}) {
  const [open, setOpen] = React.useState(false);
  const [search, setSearch] = React.useState("");
  const [options, setOptions] = React.useState<ParentAkunOption[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [selectedLabel, setSelectedLabel] = React.useState<string>("");

  // Load options when opened or search changes
  React.useEffect(() => {
    if (!open) return;
    setLoading(true);
    const timer = setTimeout(() => {
      coaApi
        .listApproved({ q: search || undefined, limit: 20 })
        .then((res) => {
          setOptions(
            res.data.map((item) => ({
              id: item.id,
              kodeAkun: item.kodeAkun,
              namaAkun: item.namaAkun,
            })),
          );
        })
        .catch(() => setOptions([]))
        .finally(() => setLoading(false));
    }, 300);
    return () => clearTimeout(timer);
  }, [open, search]);

  // On initial load, fetch label for selected value
  React.useEffect(() => {
    if (!value) {
      setSelectedLabel("");
      return;
    }
    coaApi
      .get(value)
      .then((res) => {
        setSelectedLabel(
          `${res.data.kodeAkun} — ${res.data.namaAkun}`,
        );
      })
      .catch(() => setSelectedLabel(value));
  }, [value]);

  return (
    <div className="relative">
      <Button
        type="button"
        variant="outline"
        role="combobox"
        aria-expanded={open}
        aria-label="Pilih akun induk"
        className={cn(
          "w-full justify-between font-normal",
          !value && "text-muted-foreground",
          disabled && "cursor-not-allowed opacity-60",
        )}
        disabled={disabled}
        onClick={() => !disabled && setOpen(true)}
      >
        <span className="truncate">
          {value ? selectedLabel || "Memuat..." : "Pilih akun induk (opsional)"}
        </span>
        {value && (
          <span
            role="button"
            aria-label="Hapus pilihan akun induk"
            className="ml-2 shrink-0 text-muted-foreground hover:text-foreground"
            onClick={(e) => {
              e.stopPropagation();
              onChange(null);
              setSelectedLabel("");
            }}
          >
            &times;
          </span>
        )}
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Pilih Akun Induk</DialogTitle>
            <DialogDescription>
              Hanya akun berstatus APPROVED yang ditampilkan.
            </DialogDescription>
          </DialogHeader>
          <Input
            placeholder="Cari kode atau nama akun..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="mb-2"
            autoFocus
          />
          <div className="max-h-64 overflow-y-auto space-y-1">
            {loading && (
              <p className="py-4 text-center text-sm text-muted-foreground">
                Memuat...
              </p>
            )}
            {!loading && options.length === 0 && (
              <p className="py-4 text-center text-sm text-muted-foreground">
                Tidak ada akun ditemukan.
              </p>
            )}
            {!loading &&
              options.map((opt) => (
                <button
                  key={opt.id}
                  type="button"
                  className={cn(
                    "w-full rounded-md px-3 py-2 text-left text-sm hover:bg-muted transition-colors",
                    value === opt.id && "bg-primary/10 font-medium",
                  )}
                  onClick={() => {
                    onChange(opt.id);
                    setSelectedLabel(`${opt.kodeAkun} — ${opt.namaAkun}`);
                    setOpen(false);
                  }}
                >
                  <span className="font-mono text-xs text-muted-foreground mr-2">
                    {opt.kodeAkun}
                  </span>
                  {opt.namaAkun}
                </button>
              ))}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Batal
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Mata uang options (fetched from approved list)
// ---------------------------------------------------------------------------

const COMMON_CURRENCIES = [
  { kode: "IDR", nama: "Rupiah Indonesia" },
  { kode: "USD", nama: "US Dollar" },
  { kode: "EUR", nama: "Euro" },
  { kode: "SGD", nama: "Singapore Dollar" },
  { kode: "JPY", nama: "Japanese Yen" },
  { kode: "GBP", nama: "British Pound" },
];

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface CoAFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<CoAItem>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function CoAForm({ mode, defaultValues }: CoAFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<
    string | null
  >(null);

  const isEdit = mode === "edit";

  const form = useForm({
    resolver: zodResolver(coaCreateSchema),
    defaultValues: {
      kodeAkun: defaultValues?.kodeAkun ?? "",
      namaAkun: defaultValues?.namaAkun ?? "",
      tipeAkun: defaultValues?.tipeAkun ?? "ASET",
      subTipeAkun: defaultValues?.subTipeAkun ?? undefined,
      kategoriInvestasi: defaultValues?.kategoriInvestasi ?? undefined,
      matauangNative: defaultValues?.matauangNative ?? "IDR",
      posisiNormal: defaultValues?.posisiNormal ?? "DEBIT",
      aktifFlag:
        defaultValues?.aktifFlag !== undefined ? defaultValues.aktifFlag : true,
      parentAkunId: defaultValues?.parentAkunId ?? null,
      sumberCoa: defaultValues?.sumberCoa ?? "",
      tanggalMulaiAktif:
        defaultValues?.tanggalMulaiAktif ??
        new Date().toISOString().split("T")[0],
    },
  });

  const { isDirty } = form.formState;

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/coa");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/coa");
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

  const onSubmit = async (values: CoACreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (
        isEdit &&
        defaultValues?.id &&
        defaultValues.rowVersion !== undefined
      ) {
        const updateData: CoAUpdateInput = {
          ...values,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await coaApi.update(
          defaultValues.id,
          updateData,
          idempotencyKey,
        );
        notify.success(
          `Akun ${res.data.kodeAkun} — ${res.data.namaAkun} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/coa/${res.data.id}`),
            },
          },
        );
        router.push(`/master/coa/${res.data.id}`);
      } else {
        const res = await coaApi.create(values, idempotencyKey);
        notify.success(
          `Akun ${res.data.kodeAkun} — ${res.data.namaAkun} berhasil dibuat. Menunggu review.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/coa/${res.data.id}`),
            },
          },
        );
        router.push(`/master/coa/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace(
            "body.",
            "",
          ) as keyof CoACreateInput;
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
          form.setError("kodeAkun", {
            message: `Kode akun ${values.kodeAkun} sudah terdaftar di sistem.`,
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
          rejectedBy="Finance Controller"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Section: Identifikasi Akun */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Identifikasi Akun
              </h2>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Kode Akun */}
                <FormField
                  control={form.control}
                  name="kodeAkun"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Kode Akun{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="1.1.01.001"
                          className={cn(
                            "font-mono",
                            isEdit && "bg-muted cursor-not-allowed",
                          )}
                          disabled={isEdit}
                          aria-required="true"
                          title={
                            isEdit
                              ? "Kode akun tidak bisa diubah setelah dibuat."
                              : undefined
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        Format bertingkat dengan titik, mis: 1.1.01.001
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

                {/* Nama Akun */}
                <FormField
                  control={form.control}
                  name="namaAkun"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Nama Akun{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="Kas dan Setara Kas"
                          aria-required="true"
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tipe Akun */}
                <FormField
                  control={form.control}
                  name="tipeAkun"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tipe Akun{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih tipe akun" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="ASET">Aset</SelectItem>
                          <SelectItem value="LIABILITAS">Liabilitas</SelectItem>
                          <SelectItem value="EKUITAS">Ekuitas</SelectItem>
                          <SelectItem value="PENDAPATAN">Pendapatan</SelectItem>
                          <SelectItem value="BEBAN">Beban</SelectItem>
                          <SelectItem value="KONTINJEN">Kontinjen</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Sub Tipe Akun */}
                <FormField
                  control={form.control}
                  name="subTipeAkun"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Sub Tipe Akun</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ""}
                          placeholder="Mis: Aset Lancar, Aset Tetap"
                        />
                      </FormControl>
                      <FormDescription>Opsional</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Kategori Investasi */}
                <FormField
                  control={form.control}
                  name="kategoriInvestasi"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Kategori Investasi</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ""}
                          placeholder="Mis: Deposito, Obligasi, Saham"
                        />
                      </FormControl>
                      <FormDescription>
                        Opsional — relevan untuk akun instrumen keuangan
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Mata Uang Native */}
                <FormField
                  control={form.control}
                  name="matauangNative"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Mata Uang Native{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih mata uang" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {COMMON_CURRENCIES.map((c) => (
                            <SelectItem key={c.kode} value={c.kode}>
                              <span className="font-mono font-bold mr-2">
                                {c.kode}
                              </span>
                              {c.nama}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        Default IDR. Mata uang dari daftar yang telah disetujui.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* Section: Klasifikasi & Saldo */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Klasifikasi &amp; Saldo
              </h2>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Posisi Normal */}
                <FormField
                  control={form.control}
                  name="posisiNormal"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Posisi Normal{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <div
                          className="flex gap-4"
                          role="radiogroup"
                          aria-label="Posisi normal akun"
                        >
                          {(["DEBIT", "KREDIT"] as const).map((pos) => (
                            <label
                              key={pos}
                              className={cn(
                                "flex cursor-pointer items-center gap-2 rounded-md border px-4 py-2.5 text-sm font-medium transition-colors",
                                field.value === pos
                                  ? "border-primary bg-primary/10 text-primary"
                                  : "hover:bg-muted",
                              )}
                            >
                              <input
                                type="radio"
                                value={pos}
                                checked={field.value === pos}
                                onChange={() => field.onChange(pos)}
                                className="sr-only"
                                aria-checked={field.value === pos}
                              />
                              {pos === "DEBIT" ? "Debit" : "Kredit"}
                            </label>
                          ))}
                        </div>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Aktif Flag */}
                <FormField
                  control={form.control}
                  name="aktifFlag"
                  render={({ field }) => (
                    <FormItem className="flex flex-row items-center justify-between rounded-md border p-4">
                      <div>
                        <FormLabel>Status Aktif</FormLabel>
                        <FormDescription className="mt-0.5">
                          Akun tidak aktif tidak dapat digunakan dalam jurnal
                          baru
                        </FormDescription>
                      </div>
                      <FormControl>
                        <button
                          type="button"
                          role="switch"
                          aria-checked={field.value}
                          aria-label="Toggle status aktif"
                          className={cn(
                            "relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
                            field.value ? "bg-primary" : "bg-muted",
                          )}
                          onClick={() => field.onChange(!field.value)}
                        >
                          <span
                            className={cn(
                              "pointer-events-none block h-5 w-5 rounded-full bg-white shadow-lg ring-0 transition-transform",
                              field.value
                                ? "translate-x-5"
                                : "translate-x-0",
                            )}
                          />
                        </button>
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* Section: Hierarki & Referensi */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Hierarki &amp; Referensi
              </h2>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Parent Akun */}
                <FormField
                  control={form.control}
                  name="parentAkunId"
                  render={({ field }) => (
                    <FormItem className="sm:col-span-2">
                      <FormLabel>Akun Induk (Parent)</FormLabel>
                      <FormControl>
                        <ParentAkunCombobox
                          value={field.value}
                          onChange={field.onChange}
                        />
                      </FormControl>
                      <FormDescription>
                        Pilih akun induk untuk membentuk hierarki. Kosongkan
                        untuk akun tingkat atas.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Sumber CoA */}
                <FormField
                  control={form.control}
                  name="sumberCoa"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Sumber CoA{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="Mis: PSAK 71, Manual, Import IBPA"
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>
                        Asal atau referensi sumber kode akun ini
                      </FormDescription>
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
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
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

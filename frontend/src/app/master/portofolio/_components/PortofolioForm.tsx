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
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ReturnedBanner } from "@/components/blips/ReturnedBanner";
import { cn } from "@/lib/utils";
import { portofolioApi } from "@/lib/api/portofolio.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  portofolioCreateSchema,
  portofolioUpdateSchema,
  BM_CATEGORY_LABEL,
  BM_CATEGORY_PSAK71,
  type PortofolioCreateInput,
  type PortofolioUpdateInput,
  type PortofolioItem,
} from "@/lib/schemas/portofolio.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface PortofolioFormProps {
  mode: "create" | "edit";
  /** For edit mode: existing record from API */
  defaultValues?: Partial<PortofolioItem>;
  /** When true, render all fields as read-only (APPROVED status) */
  readOnly?: boolean;
}

// ---------------------------------------------------------------------------
// BM category option with PSAK 71 hint
// ---------------------------------------------------------------------------

function BmCategoryHint({ value }: { value: string }) {
  const hint = BM_CATEGORY_PSAK71[value as keyof typeof BM_CATEGORY_PSAK71];
  if (!hint) return null;
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex items-center gap-1 text-xs text-muted-foreground cursor-help">
            <Info className="h-3 w-3" aria-hidden />
            PSAK 71: {hint}
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <p>
            Business Model <strong>{value}</strong> menghasilkan klasifikasi
            PSAK 71 <strong>{hint}</strong> (jika SPPI test lulus).
          </p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function PortofolioForm({
  mode,
  defaultValues,
  readOnly = false,
}: PortofolioFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<
    string | null
  >(null);

  const isEdit = mode === "edit";

  const form = useForm<PortofolioCreateInput>({
    resolver: zodResolver(portofolioCreateSchema),
    defaultValues: {
      kodePortofolio: defaultValues?.kodePortofolio ?? "",
      nama: defaultValues?.nama ?? "",
      tujuanPengelolaan: defaultValues?.tujuanPengelolaan ?? "",
      bmCategoryDefault: defaultValues?.bmCategoryDefault ?? "HTC",
      benchmark: defaultValues?.benchmark ?? "",
      kompensasiManagerBasis: defaultValues?.kompensasiManagerBasis ?? "",
      periodeReviewTerakhir: defaultValues?.periodeReviewTerakhir ?? null,
      aktifFlag:
        defaultValues?.aktifFlag !== undefined
          ? defaultValues.aktifFlag
          : true,
    },
  });

  const { isDirty } = form.formState;

  // Watch bmCategoryDefault to show PSAK 71 hint inline
  const selectedBm = form.watch("bmCategoryDefault");

  // ---------------------------------------------------------------------------
  // Unsaved changes guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty && !readOnly) {
      setPendingNavigation("/master/portofolio");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/portofolio");
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

  const onSubmit = async (values: PortofolioCreateInput) => {
    if (readOnly) return;
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (
        isEdit &&
        defaultValues?.id &&
        defaultValues.rowVersion !== undefined
      ) {
        const updateData: PortofolioUpdateInput = {
          ...values,
          benchmark: values.benchmark || null,
          kompensasiManagerBasis: values.kompensasiManagerBasis || null,
          periodeReviewTerakhir: values.periodeReviewTerakhir || null,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await portofolioApi.update(
          defaultValues.id,
          updateData,
          idempotencyKey,
        );
        notify.success(
          `Portofolio ${res.data.kodePortofolio} — ${res.data.nama} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/portofolio/${res.data.id}`),
            },
          },
        );
        router.push(`/master/portofolio/${res.data.id}`);
      } else {
        const createData: PortofolioCreateInput = {
          ...values,
          benchmark: values.benchmark || null,
          kompensasiManagerBasis: values.kompensasiManagerBasis || null,
          periodeReviewTerakhir: values.periodeReviewTerakhir || null,
        };
        const res = await portofolioApi.create(createData, idempotencyKey);
        notify.success(
          `Portofolio ${res.data.kodePortofolio} — ${res.data.nama} berhasil dibuat. Menunggu review.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/portofolio/${res.data.id}`),
            },
          },
        );
        router.push(`/master/portofolio/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace(
            "body.",
            "",
          ) as keyof PortofolioCreateInput;
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

        if (err.code === "CONFLICT" && isEdit === false) {
          form.setError("kodePortofolio", {
            message: `Kode portofolio ${values.kodePortofolio} sudah terdaftar di sistem.`,
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

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

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

      {readOnly && (
        <div className="mb-4 flex items-center gap-2 rounded-md border border-primary/30 bg-primary/5 px-4 py-3 text-sm text-primary">
          <Info className="h-4 w-4 shrink-0" aria-hidden />
          <span>
            Portofolio ini sudah disetujui dan tidak bisa diedit langsung.
            Hubungi Finance Controller untuk melakukan perubahan.
          </span>
        </div>
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Section: Identitas Portofolio */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Identitas Portofolio
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Kode Portofolio */}
                <FormField
                  control={form.control}
                  name="kodePortofolio"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Kode Portofolio{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="BOND_HTM_IDR"
                          maxLength={20}
                          className={cn(
                            "font-mono uppercase",
                            (isEdit || readOnly) &&
                              "bg-muted cursor-not-allowed",
                          )}
                          disabled={isEdit || readOnly}
                          aria-required="true"
                          title={
                            isEdit
                              ? "Kode portofolio tidak bisa diubah setelah dibuat."
                              : undefined
                          }
                          onChange={(e) =>
                            field.onChange(e.target.value.toUpperCase())
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        Huruf kapital, angka, underscore. Maks 20 karakter.
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

                {/* Nama */}
                <FormField
                  control={form.control}
                  name="nama"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Nama Portofolio{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="Obligasi Hold-to-Maturity IDR"
                          aria-required="true"
                          disabled={readOnly}
                          className={cn(readOnly && "bg-muted cursor-not-allowed")}
                        />
                      </FormControl>
                      <FormDescription>Min 3 karakter</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              {/* Tujuan Pengelolaan */}
              <FormField
                control={form.control}
                name="tujuanPengelolaan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Tujuan Pengelolaan{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        placeholder="Deskripsikan tujuan pengelolaan portofolio ini sesuai Business Model yang ditetapkan..."
                        rows={4}
                        aria-required="true"
                        disabled={readOnly}
                        className={cn(readOnly && "bg-muted cursor-not-allowed")}
                      />
                    </FormControl>
                    <FormDescription>
                      Min 10 karakter, max 2000 karakter. Isi sesuai Business
                      Model yang ditetapkan.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            {/* Section: Klasifikasi Business Model */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Klasifikasi Business Model (PSAK 71)
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* BM Category Default */}
                <FormField
                  control={form.control}
                  name="bmCategoryDefault"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Business Model Default{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                        disabled={readOnly}
                      >
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih Business Model" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {(["HTC", "HTCS", "OTHER"] as const).map((bm) => (
                            <SelectItem key={bm} value={bm}>
                              {BM_CATEGORY_LABEL[bm]}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription className="space-y-1">
                        <span className="block">
                          Kategori default untuk instrumen baru di portofolio
                          ini.
                        </span>
                        {selectedBm && (
                          <BmCategoryHint value={selectedBm} />
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Periode Review Terakhir */}
                <FormField
                  control={form.control}
                  name="periodeReviewTerakhir"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Periode Review Terakhir</FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          value={field.value ?? ""}
                          onChange={(e) =>
                            field.onChange(e.target.value || null)
                          }
                          disabled={readOnly}
                          className={cn(readOnly && "bg-muted cursor-not-allowed")}
                        />
                      </FormControl>
                      <FormDescription>
                        Opsional. Tanggal BM assessment terakhir dilakukan.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* Section: Informasi Tambahan */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Informasi Tambahan
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Benchmark */}
                <FormField
                  control={form.control}
                  name="benchmark"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Benchmark</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ""}
                          placeholder="Mis: IndoBeX, IBPA Bond Index, dll"
                          disabled={readOnly}
                          className={cn(readOnly && "bg-muted cursor-not-allowed")}
                          onChange={(e) =>
                            field.onChange(e.target.value || null)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        Opsional. Indeks acuan kinerja portofolio.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Kompensasi Manager Basis */}
                <FormField
                  control={form.control}
                  name="kompensasiManagerBasis"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Kompensasi Manager Basis</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ""}
                          placeholder="Mis: Fee berbasis AUM, Performance fee, dll"
                          disabled={readOnly}
                          className={cn(readOnly && "bg-muted cursor-not-allowed")}
                          onChange={(e) =>
                            field.onChange(e.target.value || null)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        Opsional. Dasar perhitungan kompensasi manajer investasi.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              {/* Status Aktif */}
              <FormField
                control={form.control}
                name="aktifFlag"
                render={({ field }) => (
                  <FormItem className="flex flex-row items-center justify-between rounded-md border p-4">
                    <div>
                      <FormLabel>Status Aktif</FormLabel>
                      <FormDescription className="mt-0.5">
                        Jika tidak aktif, portofolio tidak bisa dipilih untuk
                        penempatan instrumen baru
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        role="switch"
                        aria-checked={field.value}
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={readOnly}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </div>
          </div>

          {/* Footer */}
          {!readOnly && (
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
          )}

          {readOnly && (
            <div className="mt-6 flex justify-end gap-3 border-t pt-4">
              <Button
                type="button"
                variant="outline"
                onClick={() => router.push("/master/portofolio")}
              >
                Kembali ke Daftar
              </Button>
            </div>
          )}
        </form>
      </Form>

      {/* Unsaved changes dialog */}
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

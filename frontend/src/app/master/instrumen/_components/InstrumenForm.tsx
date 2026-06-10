"use client";

import * as React from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { AlertTriangle, Lock } from "lucide-react";
import { format, parseISO } from "date-fns";

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
import { Separator } from "@/components/ui/separator";
import { ReturnedBanner } from "@/components/blips/ReturnedBanner";
import { cn } from "@/lib/utils";
import { instrumenApi } from "@/lib/api/instrumen.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  instrumenCreateSchema,
  instrumenUpdateSchema,
  TIPE_REQUIRES_KUSTODIAN,
  TIPE_REQUIRES_MANAJER_INVESTASI,
  TIPE_REQUIRES_KUPON,
  type InstrumenCreateInput,
  type InstrumenUpdateInput,
  type InstrumenItem,
  type TipeInstrumen,
} from "@/lib/schemas/instrumen.schema";

// ---------------------------------------------------------------------------
// Section heading component
// ---------------------------------------------------------------------------

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
      {children}
    </h2>
  );
}

// ---------------------------------------------------------------------------
// Locked klasifikasi banner
// ---------------------------------------------------------------------------

function KlasifikasiLockedBanner({
  lockedAt,
  lockedBy,
}: {
  lockedAt: string;
  lockedBy?: string;
}) {
  const formatted = React.useMemo(() => {
    try {
      return format(parseISO(lockedAt), "dd MMM yyyy, HH:mm 'WIB'");
    } catch {
      return lockedAt;
    }
  }, [lockedAt]);

  return (
    <div
      role="alert"
      className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4"
    >
      <Lock className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden />
      <div className="text-sm text-amber-800">
        <p className="font-semibold">Klasifikasi terkunci</p>
        <p>
          Terkunci sejak{" "}
          <span className="font-medium">{formatted}</span>
          {lockedBy && (
            <>
              {" "}
              oleh <span className="font-medium">{lockedBy}</span>
            </>
          )}
          . Edit klasifikasi via workflow SPPI/BM (Phase 4).
        </p>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// IDR formatter / parser helpers
// ---------------------------------------------------------------------------

function formatNominal(raw: string): string {
  const num = parseFloat(raw.replace(/\./g, "").replace(/,/g, "."));
  if (isNaN(num)) return raw;
  return new Intl.NumberFormat("id-ID").format(num);
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface InstrumenFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<InstrumenItem>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function InstrumenForm({ mode, defaultValues }: InstrumenFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<
    string | null
  >(null);

  const isEdit = mode === "edit";
  const isKlasifikasiLocked = isEdit && !!defaultValues?.klasifikasiLockedAt;

  // ---------------------------------------------------------------------------
  // Reference data queries (for FK dropdowns)
  // ---------------------------------------------------------------------------

  const { data: counterpartiesData } = useQuery({
    queryKey: ["counterparties-approved"],
    queryFn: () => instrumenApi.listCounterparties(),
    staleTime: 60_000,
  });

  const { data: portofoliosData } = useQuery({
    queryKey: ["portofolios-approved"],
    queryFn: () => instrumenApi.listPortofolios(),
    staleTime: 60_000,
  });

  const { data: mataUangData } = useQuery({
    queryKey: ["mata-uang-approved"],
    queryFn: () => instrumenApi.listMataUang(),
    staleTime: 300_000,
  });

  const counterparties = counterpartiesData?.data ?? [];
  const portofolios = portofoliosData?.data ?? [];
  const mataUangList = mataUangData?.data ?? [];

  // ---------------------------------------------------------------------------
  // Form
  // ---------------------------------------------------------------------------

  const form = useForm<InstrumenCreateInput>({
    // Cast needed: superRefine changes the output type in zod v4 in ways that
    // confuse RHF generic inference. Runtime behaviour is correct.
    resolver: zodResolver(instrumenCreateSchema) as unknown as import("react-hook-form").Resolver<InstrumenCreateInput>,
    defaultValues: {
      kodeInstrumen: defaultValues?.kodeInstrumen ?? "",
      tipeInstrumen: (defaultValues?.tipeInstrumen as TipeInstrumen) ?? undefined,
      subTipe: defaultValues?.subTipe ?? "",
      nama: defaultValues?.nama ?? "",
      isin: defaultValues?.isin ?? "",
      counterpartyId: defaultValues?.counterpartyId ?? "",
      manajerInvestasiId: defaultValues?.manajerInvestasiId ?? "",
      bankKustodianId: defaultValues?.bankKustodianId ?? "",
      mataUang: defaultValues?.mataUang ?? "IDR",
      portofolioId: defaultValues?.portofolioId ?? "",
      nominal: defaultValues?.nominal ?? "",
      jumlahLot: defaultValues?.jumlahLot ?? "",
      tanggalPenempatan:
        defaultValues?.tanggalPenempatan ??
        new Date().toISOString().split("T")[0],
      tanggalJatuhTempo: defaultValues?.tanggalJatuhTempo ?? "",
      kupon: defaultValues?.kupon ?? "",
      frekuensiBunga:
        (defaultValues?.frekuensiBunga as InstrumenCreateInput["frekuensiBunga"]) ??
        undefined,
      autoRenewalFlag: defaultValues?.autoRenewalFlag ?? false,
      fvociElection: defaultValues?.fvociElection ?? false,
      bmCategory:
        (defaultValues?.bmCategory as InstrumenCreateInput["bmCategory"]) ??
        undefined,
      eirAwal: defaultValues?.eirAwal ?? "",
      premiumDiskonto: defaultValues?.premiumDiskonto ?? "0",
      biayaTransaksi: defaultValues?.biayaTransaksi ?? "0",
      status:
        (defaultValues?.status as InstrumenCreateInput["status"]) ?? "AKTIF",
    },
  });

  const { isDirty } = form.formState;

  // Watch tipe to show/hide conditional fields
  const tipeInstrumen = useWatch({ control: form.control, name: "tipeInstrumen" });

  const showManajerInvestasi = TIPE_REQUIRES_MANAJER_INVESTASI.includes(
    tipeInstrumen as TipeInstrumen,
  );
  const showBankKustodian = TIPE_REQUIRES_KUSTODIAN.includes(
    tipeInstrumen as TipeInstrumen,
  );
  const showKupon = TIPE_REQUIRES_KUPON.includes(tipeInstrumen as TipeInstrumen);
  const showAutoRenewal = tipeInstrumen === "DEPOSITO";

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/instrumen");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/instrumen");
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

  const onSubmit = async (values: InstrumenCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (
        isEdit &&
        defaultValues?.id &&
        defaultValues.rowVersion !== undefined
      ) {
        const updateData: InstrumenUpdateInput = {
          subTipe: values.subTipe,
          nama: values.nama,
          isin: values.isin,
          manajerInvestasiId: values.manajerInvestasiId,
          bankKustodianId: values.bankKustodianId,
          mataUang: values.mataUang,
          kupon: values.kupon,
          frekuensiBunga: values.frekuensiBunga,
          autoRenewalFlag: values.autoRenewalFlag,
          fvociElection: isKlasifikasiLocked ? undefined : values.fvociElection,
          bmCategory: isKlasifikasiLocked ? undefined : values.bmCategory,
          eirAwal: values.eirAwal,
          status: values.status,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await instrumenApi.update(
          defaultValues.id,
          updateData,
          idempotencyKey,
        );
        notify.success(
          `Instrumen ${res.data.kodeInstrumen} — ${res.data.nama} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/instrumen/${res.data.id}`),
            },
          },
        );
        router.push(`/master/instrumen/${res.data.id}`);
      } else {
        const res = await instrumenApi.create(values, idempotencyKey);
        notify.success(
          `Instrumen ${res.data.kodeInstrumen} — ${res.data.nama} berhasil dibuat. Menunggu review.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/instrumen/${res.data.id}`),
            },
          },
        );
        router.push(`/master/instrumen/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace(
            "body.",
            "",
          ) as keyof InstrumenCreateInput;
          form.setError(fieldName, { message: d.message });
        });

        if (err.code === "CONFLICT" && err.details.length === 0) {
          notify.error(err, {
            action: { label: "Muat ulang", onClick: () => router.refresh() },
          });
          return;
        }

        if (err.code === "INSTRUMEN_DUPLICATE_KODE") {
          form.setError("kodeInstrumen", {
            message: `Kode instrumen ${values.kodeInstrumen} sudah terdaftar di sistem.`,
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

  const showReturnedBanner =
    isEdit && defaultValues?.workflowStatus === "RETURNED";

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

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

      {isKlasifikasiLocked && defaultValues?.klasifikasiLockedAt && (
        <KlasifikasiLockedBanner
          lockedAt={defaultValues.klasifikasiLockedAt}
          lockedBy={defaultValues.updatedBy ?? undefined}
        />
      )}

      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          noValidate
          className="space-y-6"
        >
          {/* ================================================================ */}
          {/* Section 1: Identitas Instrumen                                   */}
          {/* ================================================================ */}
          <div className="rounded-lg border p-6 space-y-4">
            <SectionHeading>Identitas Instrumen</SectionHeading>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {/* Kode Instrumen */}
              <FormField
                control={form.control}
                name="kodeInstrumen"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Kode Instrumen{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder="OBLIG-001"
                        maxLength={20}
                        className={cn(
                          "font-mono uppercase",
                          isEdit && "bg-muted cursor-not-allowed",
                        )}
                        disabled={isEdit}
                        aria-required="true"
                        title={
                          isEdit
                            ? "Kode instrumen tidak bisa diubah setelah dibuat."
                            : undefined
                        }
                        onChange={(e) =>
                          field.onChange(e.target.value.toUpperCase())
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      Alphanumeric, 2–20 karakter
                      {isEdit && " (tidak bisa diubah)"}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Tipe Instrumen */}
              <FormField
                control={form.control}
                name="tipeInstrumen"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Tipe Instrumen{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <Select
                      value={field.value ?? ""}
                      onValueChange={field.onChange}
                      disabled={isEdit}
                    >
                      <FormControl>
                        <SelectTrigger
                          aria-required="true"
                          className={isEdit ? "bg-muted cursor-not-allowed" : ""}
                        >
                          <SelectValue placeholder="Pilih tipe instrumen" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value="DEPOSITO">Deposito</SelectItem>
                        <SelectItem value="OBLIGASI">Obligasi</SelectItem>
                        <SelectItem value="SAHAM">Saham</SelectItem>
                        <SelectItem value="REKSADANA">Reksa Dana</SelectItem>
                        <SelectItem value="SBN">
                          SBN (Surat Berharga Negara)
                        </SelectItem>
                        <SelectItem value="SPN">
                          SPN (Surat Perbendaharaan Negara)
                        </SelectItem>
                        <SelectItem value="SUKUK">Sukuk</SelectItem>
                      </SelectContent>
                    </Select>
                    {isEdit && (
                      <FormDescription>Tidak bisa diubah</FormDescription>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Nama */}
              <FormField
                control={form.control}
                name="nama"
                render={({ field }) => (
                  <FormItem className="sm:col-span-2">
                    <FormLabel>
                      Nama{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder="Obligasi Pemerintah FR0080"
                        aria-required="true"
                      />
                    </FormControl>
                    <FormDescription>Min 2 karakter</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Sub Tipe */}
              <FormField
                control={form.control}
                name="subTipe"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Sub Tipe</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder="Obligasi Fixed Rate"
                        maxLength={50}
                      />
                    </FormControl>
                    <FormDescription>Opsional, maks 50 karakter</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* ISIN */}
              <FormField
                control={form.control}
                name="isin"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>ISIN</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ""}
                        placeholder="ID1000131902"
                        maxLength={20}
                        className="font-mono uppercase"
                        onChange={(e) =>
                          field.onChange(e.target.value.toUpperCase())
                        }
                      />
                    </FormControl>
                    <FormDescription>
                      Opsional, kode identifikasi internasional
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>

          {/* ================================================================ */}
          {/* Section 2: Counterparty & Kustodian                             */}
          {/* ================================================================ */}
          <div className="rounded-lg border p-6 space-y-4">
            <SectionHeading>Counterparty &amp; Kustodian</SectionHeading>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {/* Counterparty */}
              <FormField
                control={form.control}
                name="counterpartyId"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Counterparty{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <Select
                      value={field.value ?? ""}
                      onValueChange={field.onChange}
                      disabled={isEdit}
                    >
                      <FormControl>
                        <SelectTrigger
                          aria-required="true"
                          className={isEdit ? "bg-muted cursor-not-allowed" : ""}
                        >
                          <SelectValue placeholder="Pilih counterparty" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {counterparties.length === 0 ? (
                          <div className="px-3 py-2 text-sm text-muted-foreground">
                            Tidak ada counterparty yang disetujui
                          </div>
                        ) : (
                          counterparties.map((cp) => (
                            <SelectItem key={cp.id} value={cp.id}>
                              {cp.kode} — {cp.nama}
                            </SelectItem>
                          ))
                        )}
                      </SelectContent>
                    </Select>
                    <FormDescription>
                      Hanya counterparty berstatus APPROVED yang ditampilkan
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Portofolio */}
              <FormField
                control={form.control}
                name="portofolioId"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Portofolio{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <Select
                      value={field.value ?? ""}
                      onValueChange={(val) => {
                        field.onChange(val);
                        // Auto-fill bmCategory from portfolio if not set
                        const porto = portofolios.find((p) => p.id === val);
                        if (porto?.bmCategory && !form.getValues("bmCategory")) {
                          form.setValue(
                            "bmCategory",
                            porto.bmCategory as InstrumenCreateInput["bmCategory"],
                            { shouldDirty: false },
                          );
                        }
                      }}
                    >
                      <FormControl>
                        <SelectTrigger aria-required="true">
                          <SelectValue placeholder="Pilih portofolio" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {portofolios.length === 0 ? (
                          <div className="px-3 py-2 text-sm text-muted-foreground">
                            Tidak ada portofolio yang disetujui
                          </div>
                        ) : (
                          portofolios.map((p) => (
                            <SelectItem key={p.id} value={p.id}>
                              {p.kode} — {p.nama}
                              {p.bmCategory && (
                                <span className="ml-1 text-xs text-muted-foreground">
                                  ({p.bmCategory})
                                </span>
                              )}
                            </SelectItem>
                          ))
                        )}
                      </SelectContent>
                    </Select>
                    <FormDescription>
                      Hanya portofolio berstatus APPROVED. BM Category diisi
                      otomatis dari portofolio.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Mata Uang */}
              <FormField
                control={form.control}
                name="mataUang"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Mata Uang{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <Select
                      value={field.value ?? "IDR"}
                      onValueChange={field.onChange}
                    >
                      <FormControl>
                        <SelectTrigger aria-required="true">
                          <SelectValue placeholder="Pilih mata uang" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {mataUangList.length === 0 ? (
                          <SelectItem value="IDR">IDR — Rupiah Indonesia</SelectItem>
                        ) : (
                          mataUangList.map((m) => (
                            <SelectItem key={m.kodeMataUang} value={m.kodeMataUang}>
                              {m.kodeMataUang} — {m.namaMataUang}
                            </SelectItem>
                          ))
                        )}
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Manajer Investasi — conditional: REKSADANA */}
              {showManajerInvestasi && (
                <FormField
                  control={form.control}
                  name="manajerInvestasiId"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Manajer Investasi{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <Select
                        value={field.value ?? ""}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih manajer investasi" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {counterparties
                            .filter(() => true) // TODO: filter by type MI when backend supports it
                            .map((cp) => (
                              <SelectItem key={cp.id} value={cp.id}>
                                {cp.kode} — {cp.nama}
                              </SelectItem>
                            ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        Wajib untuk instrumen REKSADANA
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              {/* Bank Kustodian — conditional: SAHAM, REKSADANA */}
              {showBankKustodian && (
                <FormField
                  control={form.control}
                  name="bankKustodianId"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Bank Kustodian{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <Select
                        value={field.value ?? ""}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih bank kustodian" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {counterparties.map((cp) => (
                            <SelectItem key={cp.id} value={cp.id}>
                              {cp.kode} — {cp.nama}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        Wajib untuk instrumen {tipeInstrumen}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </div>
          </div>

          {/* ================================================================ */}
          {/* Section 3: Periode, Nominal & Kupon                             */}
          {/* ================================================================ */}
          <div className="rounded-lg border p-6 space-y-4">
            <SectionHeading>Periode, Nominal &amp; Kupon</SectionHeading>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {/* Tanggal Penempatan */}
              <FormField
                control={form.control}
                name="tanggalPenempatan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Tanggal Penempatan{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        type="date"
                        {...field}
                        disabled={isEdit}
                        className={isEdit ? "bg-muted cursor-not-allowed" : ""}
                        aria-required="true"
                      />
                    </FormControl>
                    {isEdit && (
                      <FormDescription>Tidak bisa diubah</FormDescription>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Tanggal Jatuh Tempo */}
              <FormField
                control={form.control}
                name="tanggalJatuhTempo"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Tanggal Jatuh Tempo</FormLabel>
                    <FormControl>
                      <Input
                        type="date"
                        {...field}
                        value={field.value ?? ""}
                      />
                    </FormControl>
                    <FormDescription>
                      Opsional. Harus setelah tanggal penempatan.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Nominal */}
              <FormField
                control={form.control}
                name="nominal"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Nominal (IDR){" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder="1.000.000.000"
                        disabled={isEdit}
                        className={cn(
                          "text-right font-mono",
                          isEdit && "bg-muted cursor-not-allowed",
                        )}
                        aria-required="true"
                        onBlur={(e) => {
                          field.onBlur();
                          const stripped = e.target.value.replace(/\./g, "").replace(/,/g, ".");
                          if (!isNaN(parseFloat(stripped))) {
                            field.onChange(stripped);
                          }
                        }}
                        onFocus={(e) => {
                          // Show raw value on focus
                          field.onChange(
                            e.target.value.replace(/\./g, "").replace(/,/g, "."),
                          );
                        }}
                      />
                    </FormControl>
                    <FormDescription>
                      Nilai penempatan awal (full precision)
                      {isEdit && " — tidak bisa diubah"}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Jumlah Lot */}
              <FormField
                control={form.control}
                name="jumlahLot"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Jumlah Lot</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ""}
                        type="number"
                        min={1}
                        placeholder="1000"
                        disabled={isEdit}
                        className={isEdit ? "bg-muted cursor-not-allowed" : ""}
                      />
                    </FormControl>
                    <FormDescription>
                      Opsional, khusus saham/obligasi
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Kupon — conditional: OBLIGASI, SBN, SPN, SUKUK */}
              {showKupon && (
                <FormField
                  control={form.control}
                  name="kupon"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Kupon (%)</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ""}
                          type="number"
                          min={0}
                          max={100}
                          step={0.0001}
                          placeholder="6.50"
                          className="text-right font-mono"
                        />
                      </FormControl>
                      <FormDescription>
                        0–100 persen per tahun (cth: 6.50 untuk 6.5%)
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              {/* Frekuensi Bunga — conditional: OBLIGASI, SBN, SPN, SUKUK */}
              {showKupon && (
                <FormField
                  control={form.control}
                  name="frekuensiBunga"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Frekuensi Bunga</FormLabel>
                      <Select
                        value={field.value ?? ""}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue placeholder="Pilih frekuensi" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="BULANAN">Bulanan</SelectItem>
                          <SelectItem value="TRIWULANAN">Triwulanan</SelectItem>
                          <SelectItem value="SEMESTERAN">Semesteran</SelectItem>
                          <SelectItem value="TAHUNAN">Tahunan</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              {/* Auto Renewal — conditional: DEPOSITO */}
              {showAutoRenewal && (
                <FormField
                  control={form.control}
                  name="autoRenewalFlag"
                  render={({ field }) => (
                    <FormItem className="flex flex-row items-center justify-between rounded-md border p-4">
                      <div>
                        <FormLabel>Auto Renewal</FormLabel>
                        <FormDescription className="mt-0.5">
                          Deposito diperbarui otomatis saat jatuh tempo
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
              )}
            </div>
          </div>

          {/* ================================================================ */}
          {/* Section 4: PSAK 71 Klasifikasi (read-only di Phase 3)           */}
          {/* ================================================================ */}
          <div className="rounded-lg border p-6 space-y-4">
            <div className="flex items-center justify-between">
              <SectionHeading>Klasifikasi PSAK 71</SectionHeading>
              {isKlasifikasiLocked && (
                <span className="flex items-center gap-1 rounded-full border border-amber-200 bg-amber-50 px-2.5 py-0.5 text-xs font-medium text-amber-700">
                  <Lock className="h-3 w-3" aria-hidden />
                  Terkunci
                </span>
              )}
            </div>

            {/* Read-only display if locked */}
            {isKlasifikasiLocked && (
              <div className="rounded-md bg-muted/30 border p-4 space-y-2">
                <div className="flex items-start gap-2">
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" aria-hidden />
                  <p className="text-sm text-muted-foreground">
                    Field klasifikasi dikunci setelah SPPI/BM workflow APPROVED.
                    Perubahan hanya bisa dilakukan via workflow Phase 4.
                  </p>
                </div>
                <dl className="grid grid-cols-2 gap-3 text-sm">
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground font-medium">
                      Klasifikasi PSAK 71
                    </dt>
                    <dd className="font-mono font-medium">
                      {defaultValues?.klasifikasiPsak71 ?? "Belum ditetapkan"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground font-medium">
                      BM Category
                    </dt>
                    <dd className="font-mono font-medium">
                      {defaultValues?.bmCategory ?? "Belum ditetapkan"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground font-medium">
                      SPPI Result
                    </dt>
                    <dd className="font-medium">
                      {defaultValues?.sppiResult ?? "Belum diuji"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground font-medium">
                      FVOCI Election
                    </dt>
                    <dd className="font-medium">
                      {defaultValues?.fvociElection ? "Ya (irrevocable)" : "Tidak"}
                    </dd>
                  </div>
                </dl>
              </div>
            )}

            {/* Editable PSAK 71 fields — only when not locked */}
            {!isKlasifikasiLocked && (
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* BM Category */}
                <FormField
                  control={form.control}
                  name="bmCategory"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Business Model Category</FormLabel>
                      <Select
                        value={field.value ?? ""}
                        onValueChange={(v) =>
                          field.onChange(v === "_none" ? undefined : v)
                        }
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue placeholder="Diisi otomatis dari portofolio" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="_none">— Dari portofolio —</SelectItem>
                          <SelectItem value="HTC">HTC (Hold-to-Collect)</SelectItem>
                          <SelectItem value="HTC_S">
                            HTC&S (Hold-to-Collect and Sell)
                          </SelectItem>
                          <SelectItem value="OTHER">Other (Trading)</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        Akan diisi otomatis dari portofolio
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* FVOCI Election — only for SAHAM */}
                {tipeInstrumen === "SAHAM" && (
                  <FormField
                    control={form.control}
                    name="fvociElection"
                    render={({ field }) => (
                      <FormItem className="flex flex-row items-center justify-between rounded-md border p-4">
                        <div>
                          <FormLabel>FVOCI Election</FormLabel>
                          <FormDescription className="mt-0.5">
                            Irrevocable — tidak bisa dibatalkan setelah disetujui
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
                )}

                {/* Read-only classification display */}
                <div className="sm:col-span-2 rounded-md bg-muted/30 border p-3">
                  <p className="text-xs text-muted-foreground">
                    <span className="font-medium">Klasifikasi PSAK 71</span> —
                    Akan ditetapkan secara otomatis setelah SPPI Test dan
                    Business Model Assessment selesai (Phase 4).
                  </p>
                </div>
              </div>
            )}
          </div>

          {/* ================================================================ */}
          {/* Section 5: EIR & Amortisasi (Phase 4 deferred — optional)      */}
          {/* ================================================================ */}
          <div className="rounded-lg border p-6 space-y-4">
            <div className="flex items-center gap-2">
              <SectionHeading>EIR &amp; Amortisasi</SectionHeading>
              <span className="rounded-md border bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                Phase 4 — opsional
              </span>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {/* EIR Awal */}
              <FormField
                control={form.control}
                name="eirAwal"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>EIR Awal</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ""}
                        type="number"
                        min={0}
                        max={1}
                        step={0.00000001}
                        placeholder="0.06250000"
                        className="text-right font-mono"
                      />
                    </FormControl>
                    <FormDescription>
                      Antara 0 dan 1 (cth: 0.0625 untuk 6.25%). Dihitung ulang
                      Phase 4.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Premium/Diskonto */}
              <FormField
                control={form.control}
                name="premiumDiskonto"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Premium / Diskonto Awal (IDR)</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? "0"}
                        placeholder="0"
                        className="text-right font-mono"
                      />
                    </FormControl>
                    <FormDescription>
                      Positif = premium, negatif = diskonto
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Biaya Transaksi */}
              <FormField
                control={form.control}
                name="biayaTransaksi"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Biaya Transaksi Capitalized (IDR)</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? "0"}
                        placeholder="0"
                        className="text-right font-mono"
                      />
                    </FormControl>
                    <FormDescription>
                      Biaya yang dikapitalisasi ke carrying amount
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
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <Select
                      value={field.value ?? "AKTIF"}
                      onValueChange={field.onChange}
                    >
                      <FormControl>
                        <SelectTrigger aria-required="true">
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value="AKTIF">Aktif</SelectItem>
                        <SelectItem value="TIDAK_AKTIF">Tidak Aktif</SelectItem>
                        <SelectItem value="MATURED">Matured</SelectItem>
                        <SelectItem value="SOLD">Sold</SelectItem>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>

          {/* Footer */}
          <Separator />
          <div className="flex justify-end gap-3">
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

// Re-export helper for display
export { formatNominal };

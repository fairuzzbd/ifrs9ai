"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { v4 as uuidv4 } from "uuid";
import { Lock } from "lucide-react";

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
import { kursApi } from "@/lib/api/kurs.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  kursCreateSchema,
  kursUpdateSchema,
  formatKursTengah,
  parseKursDisplay,
  SUMBER_KURS_LABELS,
  type KursCreateInput,
  type KursUpdateInput,
  type KursItem,
} from "@/lib/schemas/kurs.schema";

// ---------------------------------------------------------------------------
// Locked banner
// ---------------------------------------------------------------------------

function LockedBanner() {
  return (
    <div
      role="alert"
      className="flex items-start gap-3 rounded-md border border-slate-300 bg-slate-50 p-4 mb-6"
    >
      <Lock className="mt-0.5 h-4 w-4 shrink-0 text-slate-600" aria-hidden />
      <div>
        <p className="text-sm font-medium text-slate-800">
          Kurs ini terkunci (periode CLOSED)
        </p>
        <p className="text-xs text-slate-600 mt-0.5">
          Data tidak dapat diubah karena periode buku terkait sudah di-hard-close.
          Hubungi Finance Controller jika diperlukan koreksi.
        </p>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// IDR input with formatter
// Displays IDR-formatted value on blur, raw decimal on focus.
// ---------------------------------------------------------------------------

interface KursRateInputProps {
  id: string;
  value: string;
  onChange: (v: string) => void;
  onBlur?: () => void;
  placeholder?: string;
  disabled?: boolean;
  "aria-required"?: "true";
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
}

function KursRateInput({
  id,
  value,
  onChange,
  onBlur,
  placeholder = "0,0000",
  disabled,
  ...ariaProps
}: KursRateInputProps) {
  const [focused, setFocused] = React.useState(false);
  const [displayValue, setDisplayValue] = React.useState(value);

  React.useEffect(() => {
    if (!focused) {
      // On blur/init: show IDR-formatted or raw
      if (value.trim() !== "") {
        const n = parseFloat(value);
        if (!isNaN(n)) {
          setDisplayValue(
            new Intl.NumberFormat("id-ID", {
              minimumFractionDigits: 4,
              maximumFractionDigits: 4,
            }).format(n),
          );
        } else {
          setDisplayValue(value);
        }
      } else {
        setDisplayValue("");
      }
    } else {
      // On focus: show raw decimal
      setDisplayValue(value);
    }
  }, [value, focused]);

  const handleFocus = () => {
    setFocused(true);
    setDisplayValue(value); // show raw for editing
  };

  const handleBlur = () => {
    setFocused(false);
    onBlur?.();
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    // Allow only digits, dot, comma
    const raw = e.target.value.replace(",", ".");
    setDisplayValue(e.target.value);
    onChange(raw);
  };

  return (
    <Input
      id={id}
      type="text"
      inputMode="decimal"
      value={displayValue}
      onChange={handleChange}
      onFocus={handleFocus}
      onBlur={handleBlur}
      placeholder={placeholder}
      disabled={disabled}
      className={cn("font-mono text-right", disabled && "bg-muted cursor-not-allowed")}
      {...ariaProps}
    />
  );
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface KursFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<KursItem>;
  approvedCurrencies?: string[]; // list of APPROVED non-IDR kode_mata_uang
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function KursForm({ mode, defaultValues, approvedCurrencies = [] }: KursFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<string | null>(null);

  const isEdit = mode === "edit";
  const isLocked = defaultValues?.lockedFlag === true;

  // Determine today + 1 for max date
  const maxDate = React.useMemo(() => {
    const d = new Date();
    d.setDate(d.getDate() + 1);
    return d.toISOString().split("T")[0] as string;
  }, []);

  const form = useForm<KursCreateInput>({
    resolver: zodResolver(kursCreateSchema),
    defaultValues: {
      kodeMataUang: defaultValues?.kodeMataUang ?? "",
      tanggalBerlaku:
        defaultValues?.tanggalBerlaku ??
        new Date().toISOString().split("T")[0],
      kursBeli: defaultValues?.kursBeli ?? "",
      kursJual: defaultValues?.kursJual ?? "",
      kursTengah: defaultValues?.kursTengah ?? "",
      sumberKurs: (defaultValues?.sumberKurs as KursCreateInput["sumberKurs"]) ?? "MANUAL",
    },
  });

  const { isDirty } = form.formState;

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/kurs");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/kurs");
    }
  };

  const handleConfirmLeave = () => {
    if (pendingNavigation) router.push(pendingNavigation);
    setUnsavedDialogOpen(false);
  };

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const onSubmit = async (values: KursCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (isEdit && defaultValues?.id && defaultValues.rowVersion !== undefined) {
        const updateData: KursUpdateInput = {
          kursBeli: values.kursBeli || undefined,
          kursJual: values.kursJual || undefined,
          kursTengah: values.kursTengah,
          sumberKurs: values.sumberKurs,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await kursApi.update(defaultValues.id, updateData, idempotencyKey);
        notify.success(
          `Kurs ${res.data.fxRateIdKode} berhasil diperbarui. Menunggu review.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/kurs/${res.data.id}`),
            },
          },
        );
        router.push(`/master/kurs/${res.data.id}`);
      } else {
        const res = await kursApi.create(values, idempotencyKey);
        notify.success(
          `Kurs ${res.data.fxRateIdKode} berhasil dibuat. Menunggu review Finance Controller.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/kurs/${res.data.id}`),
            },
          },
        );
        router.push(`/master/kurs/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace("body.", "") as keyof KursCreateInput;
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

        if (err.code === "KURS_DUPLICATE_DATE") {
          form.setError("tanggalBerlaku", {
            message: `Kurs ${values.kodeMataUang} untuk tanggal ${values.tanggalBerlaku} sudah terdaftar.`,
          });
        }

        if (err.code === "KURS_INVALID_RATES") {
          form.setError("kursBeli", { message: "Relasi kurs tidak valid: beli ≤ tengah ≤ jual" });
        }

        const fieldErrorCount = err.details.length;
        if (fieldErrorCount > 0) {
          notify.error({ ...err, message: `${fieldErrorCount} field bermasalah — lihat form di bawah.` });
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

  // ---------------------------------------------------------------------------
  // Returned banner
  // ---------------------------------------------------------------------------

  const showReturnedBanner = isEdit && defaultValues?.workflowStatus === "RETURNED";

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <>
      {isLocked && <LockedBanner />}
      {showReturnedBanner && !isLocked && (
        <ReturnedBanner
          rejectedBy="Finance Controller"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <fieldset disabled={isLocked} aria-disabled={isLocked}>
            <div className="space-y-6">
              {/* ── Identifikasi ── */}
              <div className="rounded-lg border p-6 space-y-4">
                <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                  Identifikasi
                </h2>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">

                  {/* Kode Mata Uang */}
                  <FormField
                    control={form.control}
                    name="kodeMataUang"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Mata Uang{" "}
                          <span className="text-destructive" aria-hidden>*</span>
                        </FormLabel>
                        {isEdit ? (
                          <>
                            <FormControl>
                              <Input
                                {...field}
                                disabled
                                className="bg-muted cursor-not-allowed font-mono uppercase"
                                aria-required="true"
                              />
                            </FormControl>
                            <FormDescription>Tidak bisa diubah setelah dibuat</FormDescription>
                          </>
                        ) : approvedCurrencies.length > 0 ? (
                          <Select value={field.value} onValueChange={field.onChange}>
                            <FormControl>
                              <SelectTrigger aria-required="true">
                                <SelectValue placeholder="Pilih mata uang" />
                              </SelectTrigger>
                            </FormControl>
                            <SelectContent>
                              {approvedCurrencies.map((kode) => (
                                <SelectItem key={kode} value={kode}>
                                  {kode}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        ) : (
                          <>
                            <FormControl>
                              <Input
                                {...field}
                                placeholder="USD"
                                maxLength={3}
                                className="font-mono uppercase"
                                aria-required="true"
                                onChange={(e) => field.onChange(e.target.value.toUpperCase())}
                              />
                            </FormControl>
                            <FormDescription>3 huruf kapital ISO 4217 (bukan IDR)</FormDescription>
                          </>
                        )}
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  {/* Tanggal Berlaku */}
                  <FormField
                    control={form.control}
                    name="tanggalBerlaku"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Tanggal Berlaku{" "}
                          <span className="text-destructive" aria-hidden>*</span>
                        </FormLabel>
                        <FormControl>
                          <Input
                            type="date"
                            max={maxDate}
                            {...field}
                            disabled={isEdit}
                            className={cn(isEdit && "bg-muted cursor-not-allowed")}
                            aria-required="true"
                          />
                        </FormControl>
                        <FormDescription>Maks. hari ini + 1 hari</FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>

              {/* ── Nilai Kurs ── */}
              <div className="rounded-lg border p-6 space-y-4">
                <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                  Nilai Kurs (IDR per 1 unit mata uang asing)
                </h2>

                {/* Live preview kurs tengah */}
                <div className="rounded-md bg-muted/40 px-4 py-3">
                  <span className="text-xs text-muted-foreground">Preview kurs tengah: </span>
                  <span className="font-semibold text-sm">
                    {formatKursTengah(form.watch("kursTengah")) || "—"}
                  </span>
                </div>

                <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">

                  {/* Kurs Beli */}
                  <FormField
                    control={form.control}
                    name="kursBeli"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Kurs Beli</FormLabel>
                        <FormControl>
                          <KursRateInput
                            id="kursBeli"
                            value={field.value ?? ""}
                            onChange={field.onChange}
                            onBlur={field.onBlur}
                            placeholder="15.200,0000"
                            disabled={isLocked}
                            aria-describedby="kursBeli-desc"
                          />
                        </FormControl>
                        <FormDescription id="kursBeli-desc">
                          Opsional. Harus ≤ kurs tengah.
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  {/* Kurs Tengah */}
                  <FormField
                    control={form.control}
                    name="kursTengah"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Kurs Tengah{" "}
                          <span className="text-destructive" aria-hidden>*</span>
                        </FormLabel>
                        <FormControl>
                          <KursRateInput
                            id="kursTengah"
                            value={field.value ?? ""}
                            onChange={field.onChange}
                            onBlur={field.onBlur}
                            placeholder="15.432,1234"
                            disabled={isLocked}
                            aria-required="true"
                            aria-describedby="kursTengah-desc"
                          />
                        </FormControl>
                        <FormDescription id="kursTengah-desc">
                          Wajib. Harus &gt; 0.
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  {/* Kurs Jual */}
                  <FormField
                    control={form.control}
                    name="kursJual"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Kurs Jual</FormLabel>
                        <FormControl>
                          <KursRateInput
                            id="kursJual"
                            value={field.value ?? ""}
                            onChange={field.onChange}
                            onBlur={field.onBlur}
                            placeholder="15.650,0000"
                            disabled={isLocked}
                            aria-describedby="kursJual-desc"
                          />
                        </FormControl>
                        <FormDescription id="kursJual-desc">
                          Opsional. Harus ≥ kurs tengah.
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                {/* Sumber Kurs */}
                <FormField
                  control={form.control}
                  name="sumberKurs"
                  render={({ field }) => (
                    <FormItem className="max-w-xs">
                      <FormLabel>
                        Sumber Kurs{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select value={field.value} onValueChange={field.onChange} disabled={isLocked}>
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih sumber kurs" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="BI_JISDOR">BI JISDOR</SelectItem>
                          <SelectItem value="BI_KURS_TENGAH">BI Kurs Tengah</SelectItem>
                          <SelectItem value="INTERNAL">Internal</SelectItem>
                          <SelectItem value="MANUAL">Manual</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* Footer */}
            {!isLocked && (
              <div className="mt-6 flex justify-end gap-3 border-t pt-4">
                <Button type="button" variant="outline" onClick={handleCancelClick} disabled={submitting}>
                  Batal
                </Button>
                <Button type="submit" disabled={submitting}>
                  {submitting ? "Menyimpan..." : "Simpan"}
                </Button>
              </div>
            )}
            {isLocked && (
              <div className="mt-6 flex justify-end border-t pt-4">
                <Button type="button" variant="outline" asChild>
                  <Link href="/master/kurs">Kembali ke Daftar</Link>
                </Button>
              </div>
            )}
          </fieldset>
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

// Re-export for convenience
export { parseKursDisplay };

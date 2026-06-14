"use client";

import * as React from "react";
import { useForm, useFieldArray, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { v4 as uuidv4 } from "uuid";
import {
  Plus,
  Trash2,
  ArrowUp,
  ArrowDown,
  CheckCircle2,
  AlertCircle,
  Search,
} from "lucide-react";

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
import { Checkbox } from "@/components/ui/checkbox";
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
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { ReturnedBanner } from "@/components/blips/ReturnedBanner";
import { cn } from "@/lib/utils";
import { mappingJurnalApi } from "@/lib/api/mapping-jurnal.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  mappingJurnalFormSchema,
  type MappingJurnalFormInput,
  type MappingJurnalUpdateInput,
  type MappingJurnalDetail,
  type TipeInstrumen,
  type Klasifikasi,
} from "@/lib/schemas/mapping-jurnal.schema";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const KATEGORI_EVENT_OPTIONS = [
  { value: "PENEMPATAN", label: "Penempatan" },
  { value: "MTM", label: "Mark-to-Market (MTM)" },
  { value: "BUNGA_AKRUAL", label: "Bunga Akrual" },
  { value: "BUNGA_TERIMA", label: "Bunga Diterima" },
  { value: "JATUH_TEMPO", label: "Jatuh Tempo" },
  { value: "PENJUALAN", label: "Penjualan" },
  { value: "REKLASIFIKASI", label: "Reklasifikasi" },
  { value: "ECL_IMPAIRMENT", label: "ECL / Impairment" },
  { value: "FX_REVALUATION", label: "Revaluasi FX" },
  { value: "AMORTISASI_EIR", label: "Amortisasi EIR" },
  { value: "OTHER", label: "Lainnya" },
] as const;

const TRIGGER_SOURCE_OPTIONS = [
  { value: "MANUAL", label: "Manual" },
  { value: "AUTO_EOD", label: "Otomatis EOD" },
  { value: "AUTO_EOM", label: "Otomatis EOM" },
  { value: "FEED_IBPA", label: "Feed IBPA" },
  { value: "FEED_PEFINDO", label: "Feed Pefindo" },
  { value: "SYSTEM", label: "Sistem" },
] as const;

const TIPE_INSTRUMEN_OPTIONS: { value: TipeInstrumen; label: string }[] = [
  { value: "DEPOSITO", label: "Deposito" },
  { value: "OBLIGASI", label: "Obligasi" },
  { value: "SAHAM", label: "Saham" },
  { value: "REKSADANA", label: "Reksa Dana" },
  { value: "SBN", label: "SBN" },
  { value: "REPO", label: "Repo" },
  { value: "ALL", label: "Semua Tipe" },
];

const KLASIFIKASI_OPTIONS: { value: Klasifikasi; label: string }[] = [
  { value: "AC", label: "AC (Amortised Cost)" },
  { value: "FVOCI_DEBT", label: "FVOCI Debt" },
  { value: "FVOCI_EQUITY", label: "FVOCI Equity" },
  { value: "FVTPL", label: "FVTPL" },
  { value: "POCI", label: "POCI" },
  { value: "ALL", label: "Semua Klasifikasi" },
];

const SUMBER_AMOUNT_OPTIONS = [
  { value: "PRINCIPAL", label: "Pokok (Principal)" },
  { value: "ACCRUED_INTEREST", label: "Bunga Akrual" },
  { value: "FAIR_VALUE_CHANGE", label: "Perubahan Fair Value" },
  { value: "ECL_AMOUNT", label: "Cadangan ECL" },
  { value: "EIR_AMORTIZATION", label: "Amortisasi EIR" },
  { value: "FX_GAIN_LOSS", label: "Gain/Loss FX" },
  { value: "PREMIUM_DISCOUNT", label: "Premium/Diskon" },
  { value: "OTHER", label: "Lainnya" },
] as const;

// ---------------------------------------------------------------------------
// Balance Summary Component
// ---------------------------------------------------------------------------

interface BalanceSummaryProps {
  details: MappingJurnalFormInput["details"];
}

function parseDecimal(s: string): number {
  const n = parseFloat(s);
  return isNaN(n) ? 0 : n;
}

function BalanceSummary({ details }: BalanceSummaryProps) {
  const debitSum = details
    .filter((d) => d.dkIndicator === "DEBIT")
    .reduce((acc, d) => acc + parseDecimal(d.multiplier), 0);

  const kreditSum = details
    .filter((d) => d.dkIndicator === "KREDIT")
    .reduce((acc, d) => acc + parseDecimal(d.multiplier), 0);

  const isBalanced = Math.abs(debitSum - kreditSum) < 1e-8;
  const debitCount = details.filter((d) => d.dkIndicator === "DEBIT").length;
  const kreditCount = details.filter((d) => d.dkIndicator === "KREDIT").length;
  const hasEnough = debitCount >= 1 && kreditCount >= 1;

  return (
    <div
      className={cn(
        "flex items-center gap-4 rounded-lg border px-4 py-3 text-sm",
        isBalanced && hasEnough
          ? "border-green-200 bg-green-50"
          : "border-amber-200 bg-amber-50",
      )}
      role="status"
      aria-live="polite"
      aria-label="Status keseimbangan debit kredit"
    >
      {isBalanced && hasEnough ? (
        <CheckCircle2
          className="h-4 w-4 shrink-0 text-green-600"
          aria-hidden
        />
      ) : (
        <AlertCircle
          className="h-4 w-4 shrink-0 text-amber-600"
          aria-hidden
        />
      )}
      <div className="flex gap-6">
        <span>
          <span className="font-medium text-muted-foreground">Debit:</span>{" "}
          <span className="font-mono font-semibold">{debitSum.toFixed(8)}</span>
        </span>
        <span>
          <span className="font-medium text-muted-foreground">Kredit:</span>{" "}
          <span className="font-mono font-semibold">
            {kreditSum.toFixed(8)}
          </span>
        </span>
        <span
          className={cn(
            "font-medium",
            isBalanced && hasEnough ? "text-green-700" : "text-amber-700",
          )}
        >
          {!hasEnough
            ? "Minimal 1 Debit + 1 Kredit diperlukan"
            : isBalanced
              ? "Seimbang"
              : `Selisih: ${Math.abs(debitSum - kreditSum).toFixed(8)}`}
        </span>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// CoA Autocomplete
// ---------------------------------------------------------------------------

interface CoaAutocompleteProps {
  value: string;
  displayValue?: string;
  onChange: (id: string, display: string) => void;
  disabled?: boolean;
}

function CoaAutocomplete({
  value,
  displayValue,
  onChange,
  disabled = false,
}: CoaAutocompleteProps) {
  const [open, setOpen] = React.useState(false);
  const [q, setQ] = React.useState("");
  const [options, setOptions] = React.useState<
    { id: string; kodeAkun: string; namaAkun: string }[]
  >([]);
  const [searching, setSearching] = React.useState(false);
  const inputRef = React.useRef<HTMLInputElement>(null);

  const handleSearch = React.useCallback(async (query: string) => {
    if (query.length < 2) {
      setOptions([]);
      return;
    }
    setSearching(true);
    try {
      const res = await mappingJurnalApi.searchCoa(query);
      setOptions(res.data ?? []);
    } catch {
      setOptions([]);
    } finally {
      setSearching(false);
    }
  }, []);

  React.useEffect(() => {
    const t = setTimeout(() => {
      if (open) void handleSearch(q);
    }, 300);
    return () => clearTimeout(t);
  }, [q, open, handleSearch]);

  const label = displayValue || (value ? value : "Pilih akun CoA...");

  return (
    <div className="relative">
      <Button
        type="button"
        variant="outline"
        role="combobox"
        aria-expanded={open}
        aria-label="Pilih akun CoA"
        className={cn(
          "w-full justify-start text-left font-normal",
          !value && "text-muted-foreground",
          disabled && "cursor-not-allowed opacity-50",
        )}
        disabled={disabled}
        onClick={() => {
          setOpen((p) => !p);
          setTimeout(() => inputRef.current?.focus(), 50);
        }}
      >
        <Search className="mr-2 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{label}</span>
      </Button>

      {open && (
        <div className="absolute top-full z-50 mt-1 w-full min-w-[280px] rounded-md border bg-popover shadow-lg">
          <div className="p-2">
            <Input
              ref={inputRef}
              placeholder="Cari kode atau nama akun..."
              value={q}
              onChange={(e) => setQ(e.target.value)}
              className="h-8 text-sm"
              aria-label="Cari akun CoA"
            />
          </div>
          <div className="max-h-48 overflow-y-auto">
            {searching ? (
              <p className="px-3 py-4 text-center text-xs text-muted-foreground">
                Mencari...
              </p>
            ) : options.length === 0 ? (
              <p className="px-3 py-4 text-center text-xs text-muted-foreground">
                {q.length < 2
                  ? "Ketik minimal 2 karakter untuk mencari"
                  : "Tidak ada hasil"}
              </p>
            ) : (
              options.map((opt) => (
                <button
                  key={opt.id}
                  type="button"
                  className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-accent"
                  onClick={() => {
                    onChange(opt.id, `${opt.kodeAkun} — ${opt.namaAkun}`);
                    setOpen(false);
                    setQ("");
                  }}
                >
                  <span className="font-mono text-xs text-muted-foreground">
                    {opt.kodeAkun}
                  </span>
                  <span className="truncate">{opt.namaAkun}</span>
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Multi-select chip component
// ---------------------------------------------------------------------------

interface MultiSelectChipsProps<T extends string> {
  options: { value: T; label: string }[];
  value: T[];
  onChange: (val: T[]) => void;
  disabled?: boolean;
  "aria-label"?: string;
}

function MultiSelectChips<T extends string>({
  options,
  value,
  onChange,
  disabled = false,
  "aria-label": ariaLabel,
}: MultiSelectChipsProps<T>) {
  const toggle = (v: T) => {
    if (disabled) return;
    if (value.includes(v)) {
      onChange(value.filter((x) => x !== v));
    } else {
      onChange([...value, v]);
    }
  };

  return (
    <div
      className="flex flex-wrap gap-2"
      role="group"
      aria-label={ariaLabel}
    >
      {options.map((opt) => {
        const selected = value.includes(opt.value);
        return (
          <button
            key={opt.value}
            type="button"
            role="checkbox"
            aria-checked={selected}
            disabled={disabled}
            onClick={() => toggle(opt.value)}
            className={cn(
              "inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium transition-colors",
              selected
                ? "border-primary bg-primary text-primary-foreground"
                : "border-border bg-background text-muted-foreground hover:border-primary/50 hover:text-foreground",
              disabled && "cursor-not-allowed opacity-50",
            )}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Detail Row component
// ---------------------------------------------------------------------------

interface DetailRowProps {
  index: number;
  control: ReturnType<typeof useForm<MappingJurnalFormInput>>["control"];
  setValue: ReturnType<typeof useForm<MappingJurnalFormInput>>["setValue"];
  onRemove: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
  isFirst: boolean;
  isLast: boolean;
  disabled?: boolean;
}

function DetailRow({
  index,
  control,
  setValue,
  onRemove,
  onMoveUp,
  onMoveDown,
  isFirst,
  isLast,
  disabled = false,
}: DetailRowProps) {
  return (
    <div className="relative rounded-lg border bg-card p-4 pr-12">
      {/* Row controls */}
      <div className="absolute right-2 top-2 flex flex-col gap-0.5">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          onClick={onMoveUp}
          disabled={isFirst || disabled}
          aria-label={`Pindah baris ${index + 1} ke atas`}
        >
          <ArrowUp className="h-3 w-3" aria-hidden />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6"
          onClick={onMoveDown}
          disabled={isLast || disabled}
          aria-label={`Pindah baris ${index + 1} ke bawah`}
        >
          <ArrowDown className="h-3 w-3" aria-hidden />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6 text-destructive hover:text-destructive"
          onClick={onRemove}
          disabled={disabled}
          aria-label={`Hapus baris ${index + 1}`}
        >
          <Trash2 className="h-3 w-3" aria-hidden />
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {/* Urutan */}
        <FormField
          control={control}
          name={`details.${index}.urutan`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-xs">Urutan</FormLabel>
              <FormControl>
                <Input
                  type="number"
                  min={1}
                  {...field}
                  onChange={(e) => field.onChange(parseInt(e.target.value, 10))}
                  className="h-8 text-sm"
                  disabled={disabled}
                  aria-required="true"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* DK Indicator */}
        <FormField
          control={control}
          name={`details.${index}.dkIndicator`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-xs">D/K</FormLabel>
              <Select
                value={field.value}
                onValueChange={field.onChange}
                disabled={disabled}
              >
                <FormControl>
                  <SelectTrigger
                    className={cn(
                      "h-8 text-sm",
                      field.value === "DEBIT" &&
                        "border-blue-300 bg-blue-50 text-blue-700",
                      field.value === "KREDIT" &&
                        "border-orange-300 bg-orange-50 text-orange-700",
                    )}
                    aria-required="true"
                  >
                    <SelectValue placeholder="Pilih D/K" />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="DEBIT">Debit (D)</SelectItem>
                  <SelectItem value="KREDIT">Kredit (K)</SelectItem>
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Multiplier */}
        <FormField
          control={control}
          name={`details.${index}.multiplier`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-xs">Multiplier</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  placeholder="1.00000000"
                  className="h-8 font-mono text-sm"
                  disabled={disabled}
                  aria-required="true"
                />
              </FormControl>
              <FormDescription className="text-xs">
                Desimal, mis: 1.00000000
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Kode Akun (CoA) */}
        <FormField
          control={control}
          name={`details.${index}.kodeAkunId`}
          render={({ field }) => (
            <FormItem className="sm:col-span-2">
              <FormLabel className="text-xs">Akun CoA (APPROVED)</FormLabel>
              <FormControl>
                <CoaAutocomplete
                  value={field.value}
                  displayValue={
                    control._formValues.details?.[index]?.kodeAkunDisplay ?? ""
                  }
                  onChange={(id, display) => {
                    field.onChange(id);
                    setValue(
                      `details.${index}.kodeAkunDisplay`,
                      display,
                      { shouldDirty: true },
                    );
                  }}
                  disabled={disabled}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Sumber Amount */}
        <FormField
          control={control}
          name={`details.${index}.sumberAmount`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-xs">Sumber Amount</FormLabel>
              <Select
                value={field.value}
                onValueChange={field.onChange}
                disabled={disabled}
              >
                <FormControl>
                  <SelectTrigger className="h-8 text-sm" aria-required="true">
                    <SelectValue placeholder="Pilih sumber" />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  {SUMBER_AMOUNT_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Mata Uang Posting */}
        <FormField
          control={control}
          name={`details.${index}.matauangPosting`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-xs">Mata Uang Posting</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  placeholder="IDR"
                  maxLength={10}
                  className="h-8 font-mono text-sm uppercase"
                  onChange={(e) =>
                    field.onChange(e.target.value.toUpperCase())
                  }
                  disabled={disabled}
                  aria-required="true"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Klasifikasi Filter */}
        <FormField
          control={control}
          name={`details.${index}.klasifikasiFilter`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-xs">
                Filter Klasifikasi (opsional)
              </FormLabel>
              <Select
                value={field.value ?? "__none__"}
                onValueChange={(v) =>
                  field.onChange(v === "__none__" ? null : v)
                }
                disabled={disabled}
              >
                <FormControl>
                  <SelectTrigger className="h-8 text-sm">
                    <SelectValue placeholder="Semua" />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="__none__">Semua Klasifikasi</SelectItem>
                  {KLASIFIKASI_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Underlying Type Filter */}
        <FormField
          control={control}
          name={`details.${index}.underlyingTypeFilter`}
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-xs">
                Filter Underlying (opsional)
              </FormLabel>
              <FormControl>
                <Input
                  {...field}
                  value={field.value ?? ""}
                  onChange={(e) =>
                    field.onChange(e.target.value || null)
                  }
                  placeholder="Mis: OBLIGASI_KORPORAT"
                  className="h-8 text-sm"
                  disabled={disabled}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface MappingJurnalFormProps {
  mode: "create" | "edit";
  defaultValues?: MappingJurnalDetail;
}

// ---------------------------------------------------------------------------
// Main Form Component
// ---------------------------------------------------------------------------

export function MappingJurnalForm({
  mode,
  defaultValues,
}: MappingJurnalFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<
    string | null
  >(null);

  const isEdit = mode === "edit";

  const form = useForm<MappingJurnalFormInput>({
    resolver: zodResolver(mappingJurnalFormSchema),
    defaultValues: {
      header: {
        eventIdKode: defaultValues?.eventIdKode ?? "",
        eventCode: defaultValues?.eventCode ?? "",
        namaEvent: defaultValues?.namaEvent ?? "",
        kategoriEvent: defaultValues?.kategoriEvent ?? "PENEMPATAN",
        triggerSource: defaultValues?.triggerSource ?? "MANUAL",
        tipeInstrumenBerlaku: defaultValues?.tipeInstrumenBerlaku ?? [],
        klasifikasiBerlaku: defaultValues?.klasifikasiBerlaku ?? [],
        aktifFlag:
          defaultValues?.aktifFlag !== undefined
            ? defaultValues.aktifFlag
            : true,
        catatan: defaultValues?.catatan ?? "",
      },
      details:
        defaultValues?.details?.map((d) => ({
          _clientKey: uuidv4(),
          id: d.id,
          urutan: d.urutan,
          kodeAkunId: d.kodeAkunId,
          kodeAkunDisplay:
            d.kodeAkunDisplay
              ? `${d.kodeAkunDisplay} — ${d.namaAkun ?? ""}`
              : "",
          dkIndicator: d.dkIndicator,
          sumberAmount: d.sumberAmount,
          klasifikasiFilter: d.klasifikasiFilter ?? null,
          tipeInstrumenFilter: d.tipeInstrumenFilter ?? [],
          underlyingTypeFilter: d.underlyingTypeFilter ?? null,
          multiplier: d.multiplier,
          matauangPosting: d.matauangPosting,
        })) ?? [],
    },
  });

  const { fields, append, remove, move } = useFieldArray({
    control: form.control,
    name: "details",
  });

  const watchedDetails = useWatch({
    control: form.control,
    name: "details",
  });

  const { isDirty } = form.formState;

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/mapping-jurnal");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/mapping-jurnal");
    }
  };

  const handleConfirmLeave = () => {
    if (pendingNavigation) {
      router.push(pendingNavigation);
    }
    setUnsavedDialogOpen(false);
  };

  // ---------------------------------------------------------------------------
  // Add detail row
  // ---------------------------------------------------------------------------

  const handleAddDetail = () => {
    const nextUrutan = fields.length + 1;
    append({
      _clientKey: uuidv4(),
      urutan: nextUrutan,
      kodeAkunId: "",
      kodeAkunDisplay: "",
      dkIndicator: "DEBIT",
      sumberAmount: "PRINCIPAL",
      klasifikasiFilter: null,
      tipeInstrumenFilter: [],
      underlyingTypeFilter: null,
      multiplier: "1.00000000",
      matauangPosting: "IDR",
    });
  };

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const onSubmit = async (values: MappingJurnalFormInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    try {
      if (
        isEdit &&
        defaultValues?.id &&
        defaultValues.rowVersion !== undefined
      ) {
        const updateData: MappingJurnalUpdateInput = {
          ...values,
          rowVersion: defaultValues.rowVersion,
        };
        const res = await mappingJurnalApi.update(
          defaultValues.id,
          updateData,
          idempotencyKey,
        );
        notify.success(
          `Mapping Jurnal "${res.data.namaEvent}" berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/mapping-jurnal/${res.data.id}`),
            },
          },
        );
        router.push(`/master/mapping-jurnal/${res.data.id}`);
      } else {
        const res = await mappingJurnalApi.create(values, idempotencyKey);
        notify.success(
          `Mapping Jurnal "${res.data.namaEvent}" berhasil dibuat. Menunggu review.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () =>
                router.push(`/master/mapping-jurnal/${res.data.id}`),
            },
          },
        );
        router.push(`/master/mapping-jurnal/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const field = d.field.replace("body.", "");
          // Try to set nested field errors (header.* or details.*)
          try {
            form.setError(field as Parameters<typeof form.setError>[0], {
              message: d.message,
            });
          } catch {
            // Ignore unrecognized field paths
          }
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

        const fieldErrorCount = err.details.length;
        if (fieldErrorCount > 0) {
          notify.error({
            ...err,
            message: `${fieldErrorCount} field bermasalah — lihat form di bawah.`,
          });
          setTimeout(() => {
            const firstErrorEl = document.querySelector(
              "[aria-invalid='true']",
            );
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
          rejectedBy="Reviewer"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* ----------------------------------------------------------------
                Section 1: Header
            ---------------------------------------------------------------- */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Informasi Header Mapping Jurnal
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Event ID Kode */}
                <FormField
                  control={form.control}
                  name="header.eventIdKode"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Event ID Kode{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="PENEMPATAN_DEPOSITO_AC"
                          className={cn(
                            "font-mono uppercase",
                            isEdit && "bg-muted cursor-not-allowed",
                          )}
                          disabled={isEdit}
                          onChange={(e) =>
                            field.onChange(e.target.value.toUpperCase())
                          }
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>
                        Huruf kapital, angka, underscore. Tidak bisa diubah
                        setelah dibuat.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Event Code */}
                <FormField
                  control={form.control}
                  name="header.eventCode"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Event Code{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="EVT-001"
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>Unik di sistem</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Nama Event */}
                <FormField
                  control={form.control}
                  name="header.namaEvent"
                  render={({ field }) => (
                    <FormItem className="sm:col-span-2">
                      <FormLabel>
                        Nama Event{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="Penempatan Deposito - Amortised Cost"
                          aria-required="true"
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Kategori Event */}
                <FormField
                  control={form.control}
                  name="header.kategoriEvent"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Kategori Event{" "}
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
                            <SelectValue placeholder="Pilih kategori" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {KATEGORI_EVENT_OPTIONS.map((opt) => (
                            <SelectItem key={opt.value} value={opt.value}>
                              {opt.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Trigger Source */}
                <FormField
                  control={form.control}
                  name="header.triggerSource"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Trigger Source{" "}
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
                            <SelectValue placeholder="Pilih trigger" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {TRIGGER_SOURCE_OPTIONS.map((opt) => (
                            <SelectItem key={opt.value} value={opt.value}>
                              {opt.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tipe Instrumen Berlaku */}
                <FormField
                  control={form.control}
                  name="header.tipeInstrumenBerlaku"
                  render={({ field }) => (
                    <FormItem className="sm:col-span-2">
                      <FormLabel>
                        Tipe Instrumen Berlaku{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <MultiSelectChips
                          options={TIPE_INSTRUMEN_OPTIONS}
                          value={field.value}
                          onChange={field.onChange}
                          aria-label="Pilih tipe instrumen berlaku"
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Klasifikasi Berlaku */}
                <FormField
                  control={form.control}
                  name="header.klasifikasiBerlaku"
                  render={({ field }) => (
                    <FormItem className="sm:col-span-2">
                      <FormLabel>
                        Klasifikasi PSAK 71 Berlaku{" "}
                        <span className="text-destructive" aria-hidden>
                          *
                        </span>
                      </FormLabel>
                      <FormControl>
                        <MultiSelectChips
                          options={KLASIFIKASI_OPTIONS}
                          value={field.value}
                          onChange={field.onChange}
                          aria-label="Pilih klasifikasi berlaku"
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Aktif Flag */}
                <FormField
                  control={form.control}
                  name="header.aktifFlag"
                  render={({ field }) => (
                    <FormItem className="flex flex-row items-center justify-between rounded-md border p-4">
                      <div>
                        <FormLabel>Status Aktif</FormLabel>
                        <FormDescription className="mt-0.5">
                          Mapping jurnal tidak aktif tidak akan digunakan oleh
                          mesin jurnal otomatis
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

                {/* Catatan */}
                <FormField
                  control={form.control}
                  name="header.catatan"
                  render={({ field }) => (
                    <FormItem className="sm:col-span-2">
                      <FormLabel>Catatan (opsional)</FormLabel>
                      <FormControl>
                        <Textarea
                          {...field}
                          value={field.value ?? ""}
                          placeholder="Keterangan tambahan untuk mapping jurnal ini..."
                          rows={3}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>

            {/* ----------------------------------------------------------------
                Section 2: Details
            ---------------------------------------------------------------- */}
            <div className="rounded-lg border p-6 space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                    Detail Jurnal
                  </h2>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    Minimal 2 baris (1 Debit + 1 Kredit). Total Debit harus
                    sama dengan total Kredit.
                  </p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleAddDetail}
                >
                  <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                  Tambah Detail
                </Button>
              </div>

              {/* Balance summary */}
              <BalanceSummary details={watchedDetails ?? []} />

              {/* Details error message */}
              {form.formState.errors.details &&
                "message" in form.formState.errors.details && (
                  <p className="text-sm text-destructive" role="alert">
                    {form.formState.errors.details.message as string}
                  </p>
                )}

              {/* Detail rows */}
              <div className="space-y-3">
                {fields.length === 0 ? (
                  <div className="flex items-center justify-center rounded-lg border border-dashed py-8 text-sm text-muted-foreground">
                    Belum ada detail. Klik &quot;+ Tambah Detail&quot; untuk
                    menambahkan baris jurnal.
                  </div>
                ) : (
                  fields.map((field, index) => (
                    <DetailRow
                      key={field.id}
                      index={index}
                      control={form.control}
                      setValue={form.setValue}
                      onRemove={() => remove(index)}
                      onMoveUp={() => move(index, index - 1)}
                      onMoveDown={() => move(index, index + 1)}
                      isFirst={index === 0}
                      isLast={index === fields.length - 1}
                    />
                  ))
                )}
              </div>

              {/* Summary chips */}
              {fields.length > 0 && (
                <div className="flex flex-wrap gap-2 pt-2">
                  <Badge variant="outline">
                    {fields.length} baris total
                  </Badge>
                  <Badge variant="outline" className="text-blue-700 border-blue-200">
                    {
                      (watchedDetails ?? []).filter(
                        (d) => d.dkIndicator === "DEBIT",
                      ).length
                    }{" "}
                    Debit
                  </Badge>
                  <Badge
                    variant="outline"
                    className="text-orange-700 border-orange-200"
                  >
                    {
                      (watchedDetails ?? []).filter(
                        (d) => d.dkIndicator === "KREDIT",
                      ).length
                    }{" "}
                    Kredit
                  </Badge>
                </div>
              )}
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

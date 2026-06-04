"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import type { Resolver } from "react-hook-form";
import { useRouter, useSearchParams } from "next/navigation";
import { v4 as uuidv4 } from "uuid";
import { useQuery } from "@tanstack/react-query";
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
import { ratingHistoryApi } from "@/lib/api/rating-history.api";
import { counterpartyApi } from "@/lib/api/counterparty.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  ratingHistoryCreateSchema,
  ratingHistoryUpdateSchema,
  ACTION_TYPE_LABELS,
  RATING_OUTLOOK_LABELS,
  type RatingHistoryCreateInput,
  type RatingHistoryUpdateInput,
  type RatingHistoryItem,
} from "@/lib/schemas/rating-history.schema";

// Rating options in order (highest to lowest)
const RATING_OPTIONS = [
  "idAAA", "idAA+", "idAA", "idAA-",
  "idA+", "idA", "idA-",
  "idBBB+", "idBBB", "idBBB-",
  "idBB+", "idBB", "idBB-",
  "idB+", "idB", "idB-",
  "idCCC", "idD", "SD", "NR",
] as const;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface RatingHistoryFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<RatingHistoryItem>;
  /** Pre-fill counterparty when coming from nested route */
  presetCounterpartyId?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RatingHistoryForm({ mode, defaultValues, presetCounterpartyId }: RatingHistoryFormProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const cpIdFromQuery = searchParams.get("counterpartyId") ?? presetCounterpartyId ?? "";

  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const isEdit = mode === "edit";

  const form = useForm<RatingHistoryCreateInput>({
    // Cast needed: Zod v4 + @hookform/resolvers v5 type mismatch (TS2719)
    resolver: zodResolver(ratingHistoryCreateSchema) as Resolver<RatingHistoryCreateInput>,
    defaultValues: {
      counterpartyId: defaultValues?.counterpartyId ?? cpIdFromQuery,
      ratingPefindo: defaultValues?.ratingPefindo ?? "idBBB",
      ratingOutlook: defaultValues?.ratingOutlook ?? "STABLE",
      actionType: defaultValues?.actionType ?? "INITIAL",
      notchChange: defaultValues?.notchChange ?? 0,
      tanggalBerlaku: defaultValues?.tanggalBerlaku ?? new Date().toISOString().split("T")[0],
      tanggalBerakhir: defaultValues?.tanggalBerakhir ?? "",
      sumber: defaultValues?.sumber ?? "Pefindo",
      catatan: defaultValues?.catatan ?? "",
    },
  });

  const { isDirty } = form.formState;
  const cpId = form.watch("counterpartyId");

  // Load counterparty list for selector (only on create)
  const { data: cpList } = useQuery({
    queryKey: ["counterparty-selector"],
    queryFn: () => counterpartyApi.list({ limit: 200, sort: "nama:asc", "filter[status]": "AKTIF" }),
    enabled: !isEdit,
    staleTime: 60_000,
  });

  // Load counterparty name for display
  const { data: cpDetail } = useQuery({
    queryKey: ["counterparty", cpId],
    queryFn: () => counterpartyApi.get(cpId),
    enabled: !!cpId,
    staleTime: 60_000,
  });

  const handleCancelClick = () => {
    if (isDirty) {
      setUnsavedDialogOpen(true);
    } else {
      goBack();
    }
  };

  const goBack = () => {
    if (cpIdFromQuery) {
      router.push(`/master/counterparty/${cpIdFromQuery}/rating-history`);
    } else {
      router.push("/master/rating-history");
    }
  };

  const onSubmit = async (values: RatingHistoryCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    // Strip empty optional strings
    const cleaned: RatingHistoryCreateInput = {
      ...values,
      tanggalBerakhir: values.tanggalBerakhir || undefined,
      sumber: values.sumber || undefined,
      catatan: values.catatan || undefined,
    };

    try {
      if (isEdit && defaultValues?.id) {
        const updateData: RatingHistoryUpdateInput = {
          ...cleaned,
          rowVersion: defaultValues.rowVersion!,
        };
        const res = await ratingHistoryApi.update(defaultValues.id, updateData, idempotencyKey);
        const cp = res.data.counterpartyNama ?? res.data.counterpartyKode;
        notify.success(
          `Rating history ${res.data.ratingPefindo} untuk ${cp} berhasil diperbarui.`,
          { action: { label: "Lihat detail", onClick: () => router.push(`/master/rating-history/${res.data.id}`) } },
        );
        router.push(`/master/rating-history/${res.data.id}`);
      } else {
        const res = await ratingHistoryApi.create(cleaned, idempotencyKey);
        const cp = res.data.counterpartyNama ?? res.data.counterpartyKode;
        notify.success(
          `Rating history ${res.data.ratingPefindo} untuk ${cp} berhasil dibuat.`,
          { action: { label: "Lihat detail", onClick: () => router.push(`/master/rating-history/${res.data.id}`) } },
        );
        if (cpIdFromQuery) {
          router.push(`/master/counterparty/${cpIdFromQuery}/rating-history`);
        } else {
          router.push(`/master/rating-history/${res.data.id}`);
        }
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace("body.", "") as keyof RatingHistoryCreateInput;
          form.setError(fieldName, { message: d.message });
        });
        if (err.details.length > 0) {
          notify.error({ ...err, message: `${err.details.length} field bermasalah — lihat form di bawah.` });
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

  return (
    <>
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Counterparty */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Counterparty
              </h2>
              {isEdit ? (
                <div className="rounded-md bg-muted px-3 py-2 text-sm">
                  <span className="font-medium">{defaultValues?.counterpartyNama ?? defaultValues?.counterpartyKode}</span>
                  <span className="ml-2 font-mono text-muted-foreground">({defaultValues?.counterpartyKode})</span>
                  <p className="text-xs text-muted-foreground mt-0.5">Counterparty tidak bisa diubah setelah rating dibuat.</p>
                </div>
              ) : (
                <FormField
                  control={form.control}
                  name="counterpartyId"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Counterparty{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih counterparty" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {(cpList?.data ?? []).map((cp) => (
                            <SelectItem key={cp.id} value={cp.id}>
                              {cp.nama} <span className="ml-1 text-xs text-muted-foreground font-mono">({cp.kodeCounterparty})</span>
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      {cpDetail?.data && (
                        <FormDescription>
                          Rating saat ini: <strong>{cpDetail.data.ratingPefindoCurrent ?? "—"}</strong>
                        </FormDescription>
                      )}
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </div>

            {/* Rating Info */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Informasi Rating
              </h2>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Rating Pefindo */}
                <FormField
                  control={form.control}
                  name="ratingPefindo"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Rating Pefindo{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih rating" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {RATING_OPTIONS.map((r) => (
                            <SelectItem key={r} value={r}><span className="font-mono">{r}</span></SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Rating Outlook */}
                <FormField
                  control={form.control}
                  name="ratingOutlook"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Outlook{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih outlook" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {Object.entries(RATING_OUTLOOK_LABELS).map(([val, label]) => (
                            <SelectItem key={val} value={val}>{label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Action Type */}
                <FormField
                  control={form.control}
                  name="actionType"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Action Type{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih action" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {Object.entries(ACTION_TYPE_LABELS).map(([val, label]) => (
                            <SelectItem key={val} value={val}>{label}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Notch Change */}
                <FormField
                  control={form.control}
                  name="notchChange"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Notch Change{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min={-20}
                          max={20}
                          {...field}
                          onChange={(e) => field.onChange(parseInt(e.target.value, 10))}
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>
                        Positif = upgrade, negatif = downgrade, 0 = afirmasi/inisial.
                        SICR trigger otomatis jika &le; -2 atau dari IG ke non-IG.
                      </FormDescription>
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
                        <Input type="date" {...field} aria-required="true" />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Tanggal Berakhir */}
                <FormField
                  control={form.control}
                  name="tanggalBerakhir"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Tanggal Berakhir (opsional)</FormLabel>
                      <FormControl>
                        <Input type="date" {...field} value={field.value ?? ""} />
                      </FormControl>
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
                      <FormLabel>Sumber</FormLabel>
                      <FormControl>
                        <Input {...field} value={field.value ?? ""} placeholder="Pefindo" />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Catatan */}
                <FormField
                  control={form.control}
                  name="catatan"
                  render={({ field }) => (
                    <FormItem className="sm:col-span-2">
                      <FormLabel>Catatan</FormLabel>
                      <FormControl>
                        <Textarea {...field} value={field.value ?? ""} rows={3} placeholder="Catatan tambahan (opsional)" />
                      </FormControl>
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
            <Button variant="outline" onClick={() => setUnsavedDialogOpen(false)}>Tetap di Sini</Button>
            <Button variant="destructive" onClick={goBack}>Keluar Tanpa Menyimpan</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

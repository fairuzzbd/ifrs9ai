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
import { ReturnedBanner } from "@/components/blips/ReturnedBanner";
import { bobotSkenarioApi } from "@/lib/api/bobot-skenario.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  bobotSkenarioCreateSchema,
  bobotSkenarioUpdateSchema,
  bobotPercentToDecimal,
  bobotDecimalToPercent,
  SKENARIO_ECL_LABELS,
  type BobotSkenarioCreateInput,
  type BobotSkenarioItem,
} from "@/lib/schemas/bobot-skenario.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface BobotSkenarioFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<BobotSkenarioItem>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function BobotSkenarioForm({ mode, defaultValues }: BobotSkenarioFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<string | null>(null);

  const isEdit = mode === "edit";

  const form = useForm<BobotSkenarioCreateInput>({
    resolver: zodResolver(bobotSkenarioCreateSchema),
    defaultValues: {
      skenario: defaultValues?.skenario ?? ("" as BobotSkenarioCreateInput["skenario"]),
      // Convert decimal API value (e.g. "0.25000000") to percentage display string (e.g. "25.00")
      bobotPersen: bobotDecimalToPercent(defaultValues?.bobot),
      periodeBerlakuDari: defaultValues?.periodeBerlakuDari ?? "",
      periodeBerlakuSampai: defaultValues?.periodeBerlakuSampai ?? "",
      catatan: defaultValues?.catatan ?? "",
    },
  });

  const { isDirty } = form.formState;

  // Watch bobotPersen to show live preview
  const bobotPersenValue = form.watch("bobotPersen");
  const bobotDecimalPreview = bobotPercentToDecimal(bobotPersenValue ?? "");

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/bobot-skenario");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/bobot-skenario");
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

  const onSubmit = async (values: BobotSkenarioCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    // Convert bobot from percentage string to decimal string (DEC-016)
    const bobotDecimal = bobotPercentToDecimal(values.bobotPersen);
    if (!bobotDecimal) {
      form.setError("bobotPersen", { message: "Nilai bobot tidak valid" });
      setSubmitting(false);
      return;
    }

    const payload = {
      skenario: values.skenario,
      bobot: bobotDecimal,
      periodeBerlakuDari: values.periodeBerlakuDari,
      periodeBerlakuSampai: values.periodeBerlakuSampai || null,
      catatan: values.catatan || null,
    };

    try {
      if (isEdit && defaultValues?.id && defaultValues.rowVersion !== undefined) {
        const updatePayload = {
          ...payload,
          rowVersion: defaultValues.rowVersion,
        };
        bobotSkenarioUpdateSchema.parse({ ...values, rowVersion: defaultValues.rowVersion });

        const res = await bobotSkenarioApi.update(
          defaultValues.id,
          updatePayload,
          idempotencyKey,
        );
        notify.success(
          `Bobot skenario ${SKENARIO_ECL_LABELS[res.data.skenario]} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/bobot-skenario/${res.data.id}`),
            },
          },
        );
        router.push(`/master/bobot-skenario/${res.data.id}`);
      } else {
        const res = await bobotSkenarioApi.create(payload, idempotencyKey);
        notify.success(
          `Bobot skenario ${SKENARIO_ECL_LABELS[res.data.skenario]} berhasil dibuat. Menunggu submit ke workflow.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/bobot-skenario/${res.data.id}`),
            },
          },
        );
        router.push(`/master/bobot-skenario/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace(
            "body.",
            "",
          ) as keyof BobotSkenarioCreateInput;
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
          rejectedBy="Risk Officer / ALCO"
          rejectedAt={defaultValues?.updatedAt ?? new Date().toISOString()}
          comment="Data dikembalikan untuk diperbaiki. Periksa komentar di halaman detail."
          className="mb-6"
        />
      )}

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-6">
            {/* Section: Parameter Bobot Skenario */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Parameter Bobot Skenario ECL
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Skenario */}
                <FormField
                  control={form.control}
                  name="skenario"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Skenario{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih skenario" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value="GOOD">{SKENARIO_ECL_LABELS.GOOD}</SelectItem>
                          <SelectItem value="NORMAL">{SKENARIO_ECL_LABELS.NORMAL}</SelectItem>
                          <SelectItem value="BAD">{SKENARIO_ECL_LABELS.BAD}</SelectItem>
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        Skenario makroekonomi untuk kalkulasi ECL (DEC-010)
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Bobot */}
                <FormField
                  control={form.control}
                  name="bobotPersen"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Bobot (%){" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <div className="relative">
                          <input
                            type="text"
                            inputMode="decimal"
                            placeholder="25.00"
                            aria-required="true"
                            aria-describedby="bobot-desc"
                            aria-invalid={!!form.formState.errors.bobotPersen}
                            className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 pr-8 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                            {...field}
                            value={field.value}
                            onChange={(e) => field.onChange(e.target.value)}
                          />
                          <span
                            className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground"
                            aria-hidden
                          >
                            %
                          </span>
                        </div>
                      </FormControl>
                      <FormDescription id="bobot-desc">
                        Masukkan dalam persen (0–100). Default DEC-010: Good=25%, Normal=50%, Bad=25%.
                        {bobotDecimalPreview && (
                          <span className="ml-1 font-mono text-xs text-muted-foreground">
                            Desimal: {bobotDecimalPreview}
                          </span>
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Periode Berlaku Dari */}
                <FormField
                  control={form.control}
                  name="periodeBerlakuDari"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Periode Berlaku Dari{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          aria-required="true"
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        Tanggal mulai berlakunya bobot skenario ini
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
                      <FormLabel>Periode Berlaku Sampai</FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          aria-describedby="periode-sampai-desc"
                          {...field}
                          value={field.value ?? ""}
                          onChange={(e) =>
                            field.onChange(e.target.value || "")
                          }
                        />
                      </FormControl>
                      <FormDescription id="periode-sampai-desc">
                        Kosongkan jika masih berlaku (tidak ada tanggal berakhir)
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              {/* Catatan (full-width) */}
              <FormField
                control={form.control}
                name="catatan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Catatan</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        placeholder="Dasar penetapan bobot skenario, referensi ALCO, catatan metodologi..."
                        rows={4}
                        aria-describedby="catatan-desc"
                      />
                    </FormControl>
                    <FormDescription id="catatan-desc">
                      Opsional. Maks 2000 karakter. Dokumentasikan dasar ALCO dalam menetapkan bobot.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            {/* Informational box: sum constraint */}
            <div className="rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
              <strong>Validasi trio DEC-010:</strong> Total bobot Good + Normal + Bad
              harus sama dengan 100% (1.0). Sistem menampilkan indikator balance
              pada halaman daftar dan detail. Pastikan tiga skenario untuk periode
              yang sama telah lengkap sebelum workflow approval.
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

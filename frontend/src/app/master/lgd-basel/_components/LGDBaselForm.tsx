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
import { lgdBaselApi } from "@/lib/api/lgd-basel.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  lgdBaselCreateSchema,
  lgdBaselUpdateSchema,
  lgdPercentToDecimal,
  lgdDecimalToPercent,
  TIPE_EKSPOSUR_LABELS,
  type LGDBaselCreateInput,
  type LGDBaselUpdateInput,
  type LGDBaselItem,
} from "@/lib/schemas/lgd-basel.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface LGDBaselFormProps {
  mode: "create" | "edit";
  defaultValues?: Partial<LGDBaselItem>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function LGDBaselForm({ mode, defaultValues }: LGDBaselFormProps) {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [unsavedDialogOpen, setUnsavedDialogOpen] = React.useState(false);
  const [pendingNavigation, setPendingNavigation] = React.useState<string | null>(null);

  const isEdit = mode === "edit";

  const form = useForm<LGDBaselCreateInput>({
    resolver: zodResolver(lgdBaselCreateSchema),
    defaultValues: {
      tipeEksposur: defaultValues?.tipeEksposur ?? ("" as LGDBaselCreateInput["tipeEksposur"]),
      // Convert decimal API value to percentage display string
      lgdPersen: lgdDecimalToPercent(defaultValues?.lgd),
      karakteristik: defaultValues?.karakteristik ?? "",
      periodeBerlakuDari: defaultValues?.periodeBerlakuDari ?? "",
      periodeBerlakuSampai: defaultValues?.periodeBerlakuSampai ?? "",
      sumber: defaultValues?.sumber ?? "BASEL_III_IRB",
      dokumenPendukungId: defaultValues?.dokumenPendukungId ?? "",
    },
  });

  const { isDirty } = form.formState;

  // ---------------------------------------------------------------------------
  // Navigation guard
  // ---------------------------------------------------------------------------

  const handleCancelClick = () => {
    if (isDirty) {
      setPendingNavigation("/master/lgd-basel");
      setUnsavedDialogOpen(true);
    } else {
      router.push("/master/lgd-basel");
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

  const onSubmit = async (values: LGDBaselCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();

    // Convert LGD from percentage string to decimal string
    const lgdDecimal = lgdPercentToDecimal(values.lgdPersen);
    if (!lgdDecimal) {
      form.setError("lgdPersen", { message: "Nilai LGD tidak valid" });
      setSubmitting(false);
      return;
    }

    const payload = {
      tipeEksposur: values.tipeEksposur,
      lgd: lgdDecimal,
      karakteristik: values.karakteristik || undefined,
      periodeBerlakuDari: values.periodeBerlakuDari,
      periodeBerlakuSampai: values.periodeBerlakuSampai || null,
      sumber: values.sumber,
      dokumenPendukungId: values.dokumenPendukungId || null,
    };

    try {
      if (isEdit && defaultValues?.id && defaultValues.rowVersion !== undefined) {
        const updatePayload: Parameters<typeof lgdBaselApi.update>[1] = {
          ...payload,
          rowVersion: defaultValues.rowVersion,
        };
        // Validate update schema
        lgdBaselUpdateSchema.parse({ ...values, rowVersion: defaultValues.rowVersion });

        const res = await lgdBaselApi.update(
          defaultValues.id,
          updatePayload,
          idempotencyKey,
        );
        notify.success(
          `LGD pool ${TIPE_EKSPOSUR_LABELS[res.data.tipeEksposur]} berhasil diperbarui.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/lgd-basel/${res.data.id}`),
            },
          },
        );
        router.push(`/master/lgd-basel/${res.data.id}`);
      } else {
        const res = await lgdBaselApi.create(payload, idempotencyKey);
        notify.success(
          `LGD pool untuk ${TIPE_EKSPOSUR_LABELS[res.data.tipeEksposur]} berhasil dibuat. Menunggu submit ke workflow ALCO.`,
          {
            action: {
              label: "Lihat detail",
              onClick: () => router.push(`/master/lgd-basel/${res.data.id}`),
            },
          },
        );
        router.push(`/master/lgd-basel/${res.data.id}`);
      }
    } catch (err) {
      if (isApiError(err)) {
        err.details.forEach((d) => {
          const fieldName = d.field.replace(
            "body.",
            "",
          ) as keyof LGDBaselCreateInput;
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
            {/* Section: Parameter LGD */}
            <div className="rounded-lg border p-6 space-y-4">
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Parameter LGD Basel
              </h2>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {/* Tipe Eksposur */}
                <FormField
                  control={form.control}
                  name="tipeEksposur"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tipe Eksposur{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger aria-required="true">
                            <SelectValue placeholder="Pilih tipe eksposur" />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {(Object.keys(TIPE_EKSPOSUR_LABELS) as Array<keyof typeof TIPE_EKSPOSUR_LABELS>).map((key) => (
                            <SelectItem key={key} value={key}>
                              {TIPE_EKSPOSUR_LABELS[key]}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        Sesuai kategori eksposur Basel III IRB
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* LGD */}
                <FormField
                  control={form.control}
                  name="lgdPersen"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        LGD (%){" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <div className="relative">
                          <Input
                            type="number"
                            step="0.01"
                            min="0"
                            max="100"
                            placeholder="45.50"
                            aria-required="true"
                            aria-describedby="lgd-desc"
                            {...field}
                            // Keep value as string to avoid float precision issues (DEC-016)
                            value={field.value}
                            onChange={(e) => field.onChange(e.target.value)}
                            className="pr-8"
                          />
                          <span
                            className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground"
                            aria-hidden
                          >
                            %
                          </span>
                        </div>
                      </FormControl>
                      <FormDescription id="lgd-desc">
                        Masukkan dalam persen (0–100). Contoh: 45.50 untuk LGD 45.50%.
                        Nilai disimpan sebagai desimal (0.4550) di sistem.
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
                        Tanggal mulai berlakunya parameter LGD ini
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
                        Kosongkan jika parameter masih berlaku (tidak ada tanggal berakhir)
                      </FormDescription>
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
                      <FormLabel>
                        Sumber{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="BASEL_III_IRB"
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>
                        Referensi regulasi / metodologi (mis. BASEL_III_IRB)
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              {/* Karakteristik (full-width) */}
              <FormField
                control={form.control}
                name="karakteristik"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Karakteristik</FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        placeholder="Deskripsi karakteristik pool eksposur, basis kalibrasi, catatan metodologi..."
                        rows={4}
                        aria-describedby="karakteristik-desc"
                      />
                    </FormControl>
                    <FormDescription id="karakteristik-desc">
                      Opsional. Maks 2000 karakter. Jelaskan karakteristik pool eksposur dan dasar penetapan LGD.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
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

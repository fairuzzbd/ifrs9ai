"use client";

import * as React from "react";
import { Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm, useFieldArray } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { Plus, Trash2 } from "lucide-react";

import {
  amendmentProposeFormSchema,
  type AmendmentProposeForm,
} from "@/lib/schemas/eir.schema";
import { eirApi } from "@/lib/api/eir.api";
import { notify } from "@/lib/notify";
import { Button } from "@/components/ui/button";
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
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

function EIRAmendmentNewContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const instrumenIdFromQuery = searchParams.get("instrumenId") ?? "";

  const form = useForm<AmendmentProposeForm>({
    resolver: zodResolver(amendmentProposeFormSchema),
    defaultValues: {
      instrumenId: instrumenIdFromQuery,
      amendmentDate: "",
      alasan: "",
      dokumenPendukungId: "",
      revisedCashflows: [
        { date: "", amountIdr: 0 },
        { date: "", amountIdr: 0 },
      ],
    },
  });

  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: "revisedCashflows",
  });

  const mutation = useMutation({
    mutationFn: (data: AmendmentProposeForm) =>
      eirApi.proposeAmendment(
        {
          instrumenId: data.instrumenId,
          amendmentDate: data.amendmentDate,
          alasan: data.alasan,
          dokumenPendukungId: data.dokumenPendukungId,
          revisedCashflows: data.revisedCashflows,
        },
        uuidv4(),
      ),
    onSuccess: (res) => {
      notify.success(
        `Amandemen EIR berhasil diajukan. Solver akan menghitung ulang EIR. Menunggu review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () =>
              router.push(`/ecl/eir/amendments/${res.data.id}`),
          },
        },
      );
      router.push(`/ecl/eir/amendments/${res.data.id}`);
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const onSubmit = (data: AmendmentProposeForm) => mutation.mutate(data);

  return (
    <div className="max-w-2xl mx-auto p-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex gap-1">
          <li>
            <button
              className="hover:underline"
              onClick={() => router.push("/ecl/eir/amendments/queue")}
            >
              Amandemen EIR
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">Ajukan Baru</li>
        </ol>
      </nav>

      <div>
        <h1 className="text-xl font-semibold">Ajukan Amandemen EIR</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Re-estimasi EIR diperlukan saat terdapat perubahan kontraktual
          yang memengaruhi cashflow (PSAK 71 §5.4.3). Amortisasi schedule lama
          tetap disimpan (immutable). Versi baru dibuat setelah approved.
        </p>
      </div>

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          {/* Instrumen ID */}
          <FormField
            control={form.control}
            name="instrumenId"
            render={({ field }) => (
              <FormItem>
                <FormLabel>ID Instrumen</FormLabel>
                <FormControl>
                  <Input placeholder="UUID instrumen" {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Tanggal amandemen */}
          <FormField
            control={form.control}
            name="amendmentDate"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Tanggal Amandemen Kontrak</FormLabel>
                <FormControl>
                  <Input type="date" {...field} />
                </FormControl>
                <FormDescription>
                  Tanggal kontrak diamandemen — EIR berlaku dari tanggal ini.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Dokumen pendukung */}
          <FormField
            control={form.control}
            name="dokumenPendukungId"
            render={({ field }) => (
              <FormItem>
                <FormLabel>ID Dokumen Pendukung</FormLabel>
                <FormControl>
                  <Input placeholder="UUID dokumen yang sudah di-upload" {...field} />
                </FormControl>
                <FormDescription>
                  Dokumen amandemen kontrak wajib dilampirkan.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Revised cashflows */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">Revised Cashflows</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="text-sm text-muted-foreground">
                CF_0 (baris pertama) harus negatif (penempatan awal / nilai revisi).
                Minimal 2 baris.
              </p>
              {fields.map((field, index) => (
                <div key={field.id} className="flex gap-2 items-start">
                  <FormField
                    control={form.control}
                    name={`revisedCashflows.${index}.date`}
                    render={({ field: f }) => (
                      <FormItem className="flex-1">
                        {index === 0 && <FormLabel>Tanggal</FormLabel>}
                        <FormControl>
                          <Input type="date" {...f} aria-label={`Tanggal cashflow ${index + 1}`} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name={`revisedCashflows.${index}.amountIdr`}
                    render={({ field: f }) => (
                      <FormItem className="flex-1">
                        {index === 0 && <FormLabel>Jumlah (IDR)</FormLabel>}
                        <FormControl>
                          <Input
                            type="number"
                            step="1"
                            placeholder={index === 0 ? "Negatif untuk CF_0" : ""}
                            {...f}
                            onChange={(e) => f.onChange(parseFloat(e.target.value) || 0)}
                            aria-label={`Jumlah cashflow ${index + 1} IDR`}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  {fields.length > 2 && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="mt-1"
                      onClick={() => remove(index)}
                      aria-label={`Hapus baris cashflow ${index + 1}`}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" aria-hidden="true" />
                    </Button>
                  )}
                </div>
              ))}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => append({ date: "", amountIdr: 0 })}
              >
                <Plus className="h-4 w-4 mr-1" aria-hidden="true" />
                Tambah Cashflow
              </Button>
              {form.formState.errors.revisedCashflows?.root && (
                <p className="text-sm text-destructive">
                  {form.formState.errors.revisedCashflows.root.message}
                </p>
              )}
            </CardContent>
          </Card>

          {/* Alasan */}
          <FormField
            control={form.control}
            name="alasan"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Alasan Re-estimasi EIR</FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    placeholder="Deskripsikan perubahan kontraktual yang memerlukan re-estimasi EIR (minimal 20 karakter)..."
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className="flex gap-2 pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => router.back()}
              disabled={mutation.isPending}
            >
              Batal
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? (
                <>
                  <span
                    className="h-4 w-4 mr-2 border-2 border-current border-t-transparent rounded-full animate-spin"
                    aria-hidden="true"
                  />
                  Mengajukan...
                </>
              ) : (
                "Ajukan Amandemen"
              )}
            </Button>
          </div>
        </form>
      </Form>
    </div>
  );
}

export default function EIRAmendmentNewPage() {
  return (
    <Suspense fallback={<div className="p-6 animate-pulse h-40 bg-muted rounded-lg" />}>
      <EIRAmendmentNewContent />
    </Suspense>
  );
}

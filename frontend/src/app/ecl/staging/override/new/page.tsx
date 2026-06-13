"use client";

import * as React from "react";
import { Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";

import {
  overrideSubmitFormSchema,
  type OverrideSubmitForm,
} from "@/lib/schemas/staging.schema";
import { stagingApi } from "@/lib/api/staging.api";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

function StagingOverrideNewContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const instrumenIdFromQuery = searchParams.get("instrumenId") ?? "";

  const form = useForm<OverrideSubmitForm>({
    resolver: zodResolver(overrideSubmitFormSchema),
    defaultValues: {
      instrumenId: instrumenIdFromQuery,
      stageTarget: undefined,
      alasan: "",
      periodeId: "",
      dokumenPendukungId: undefined,
    },
  });

  const mutation = useMutation({
    mutationFn: (data: OverrideSubmitForm) =>
      stagingApi.submitOverride(
        {
          instrumenId: data.instrumenId,
          stageTarget: data.stageTarget,
          alasan: data.alasan,
          periodeId: data.periodeId,
          ...(data.dokumenPendukungId && {
            dokumenPendukungId: data.dokumenPendukungId,
          }),
        },
        uuidv4(),
      ),
    onSuccess: (res) => {
      notify.success(
        `Override staging berhasil diajukan. Menunggu review Risk Officer.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/ecl/staging/override/${res.data.id}`),
          },
        },
      );
      router.push(`/ecl/staging/override/${res.data.id}`);
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const onSubmit = (data: OverrideSubmitForm) => mutation.mutate(data);

  return (
    <div className="max-w-lg mx-auto p-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex gap-1">
          <li>
            <button
              className="hover:underline focus-visible:ring-2 focus-visible:ring-ring rounded"
              onClick={() => router.push("/ecl/staging/override")}
            >
              Override Staging
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">Ajukan Override Baru</li>
        </ol>
      </nav>

      <h1 className="text-xl font-semibold">Ajukan Override Staging</h1>
      <p className="text-sm text-muted-foreground">
        Override manual hanya untuk skenario luar biasa yang tidak ter-cover
        oleh trigger otomatis. Wajib mendapat sign-off RISK + ALCO + KOMITE
        (6-eyes) untuk target Stage 3.
      </p>

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
                  <Input
                    placeholder="UUID instrumen"
                    {...field}
                    aria-describedby="instrumenId-desc"
                  />
                </FormControl>
                <FormDescription id="instrumenId-desc">
                  UUID instrumen yang ingin di-override stage-nya.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Stage Target */}
          <FormField
            control={form.control}
            name="stageTarget"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Stage Target</FormLabel>
                <Select
                  onValueChange={field.onChange}
                  value={field.value ?? ""}
                >
                  <FormControl>
                    <SelectTrigger aria-label="Pilih stage target">
                      <SelectValue placeholder="Pilih stage..." />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value="STAGE_1">Stage 1 (Performing)</SelectItem>
                    <SelectItem value="STAGE_2">Stage 2 (SICR)</SelectItem>
                    <SelectItem value="STAGE_3">Stage 3 (Default)</SelectItem>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Periode ID */}
          <FormField
            control={form.control}
            name="periodeId"
            render={({ field }) => (
              <FormItem>
                <FormLabel>ID Periode Buku</FormLabel>
                <FormControl>
                  <Input placeholder="UUID periode buku" {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Alasan */}
          <FormField
            control={form.control}
            name="alasan"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Alasan Override</FormLabel>
                <FormControl>
                  <Textarea
                    placeholder="Jelaskan alasan override secara detail (minimal 20 karakter)..."
                    rows={4}
                    {...field}
                    aria-describedby="alasan-desc"
                  />
                </FormControl>
                <FormDescription id="alasan-desc">
                  Minimal 20 karakter. Alasan harus jelas dan dapat
                  dipertanggungjawabkan.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Dokumen Pendukung ID (opsional) */}
          <FormField
            control={form.control}
            name="dokumenPendukungId"
            render={({ field }) => (
              <FormItem>
                <FormLabel>ID Dokumen Pendukung (opsional)</FormLabel>
                <FormControl>
                  <Input
                    placeholder="UUID dokumen yang sudah di-upload"
                    {...field}
                    value={field.value ?? ""}
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
                "Ajukan Override"
              )}
            </Button>
          </div>
        </form>
      </Form>
    </div>
  );
}

export default function StagingOverrideNewPage() {
  return (
    <Suspense fallback={<div className="p-6 animate-pulse h-40 bg-muted rounded-lg" />}>
      <StagingOverrideNewContent />
    </Suspense>
  );
}

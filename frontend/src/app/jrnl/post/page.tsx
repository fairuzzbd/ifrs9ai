"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { manualPostApi } from "@/lib/api/jurnal.api";
import {
  manualPostSchema,
  type ManualPostInput,
} from "@/lib/schemas/jurnal.schema";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";

const MANUAL_EVENT_CODES = [
  { value: "PERIODE_ADJUSTMENT", label: "PERIODE_ADJUSTMENT — Penyesuaian Periode" },
  { value: "CORRECTION_PERIODE_CLOSED", label: "CORRECTION_PERIODE_CLOSED — Koreksi Periode Ditutup" },
];

export default function ManualPostPage() {
  const router = useRouter();
  const idempotencyKey = React.useRef(uuidv4());

  const form = useForm<ManualPostInput>({
    resolver: zodResolver(manualPostSchema) as unknown as import("react-hook-form").Resolver<ManualPostInput>,
    defaultValues: {
      eventCode: "PERIODE_ADJUSTMENT",
      periodeId: "",
      instrumenId: null,
      amountIdr: "",
      narasi: "",
      dokumenDocId: null,
    },
  });

  const { control, handleSubmit, formState: { errors } } = form;

  const postMutation = useMutation({
    mutationFn: (data: ManualPostInput) =>
      manualPostApi.create(data, idempotencyKey.current),
    onSuccess: (res) => {
      notify.success(
        `Jurnal ${res.data.noJurnal} berhasil diposting. Status: ${res.data.statusInternal}.`,
        {
          action: {
            label: "Lihat jurnal",
            onClick: () => router.push(`/jrnl/journal-entries/${res.data.id}`),
          },
        },
      );
      router.push(`/jrnl/journal-entries/${res.data.id}`);
    },
    onError: (err) => {
      if (isApiError(err)) notify.error(err);
      else notify.error({ code: "INTERNAL", message: String(err), traceId: "" });
    },
  });

  return (
    <div className="mx-auto max-w-3xl px-6 py-6 space-y-6">
      <div>
        <h1 className="text-xl font-semibold">Posting Jurnal Manual</h1>
        <p className="text-sm text-muted-foreground">
          Posting jurnal koreksi atau penyesuaian secara manual. Hanya untuk event PERIODE_ADJUSTMENT dan CORRECTION_PERIODE_CLOSED.
        </p>
      </div>

      {/* Warning */}
      <div className="flex items-start gap-3 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
        <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" aria-hidden="true" />
        <div>
          <p className="font-medium">Perhatian</p>
          <p className="text-xs mt-0.5">
            Posting manual hanya untuk koreksi dan penyesuaian yang tidak dapat dilakukan melalui alur otomatis.
            Setiap posting manual memerlukan persetujuan Finance Controller (ROLE-AKUN-CTL).
          </p>
        </div>
      </div>

      <form
        onSubmit={handleSubmit((data: unknown) => postMutation.mutate(data as ManualPostInput))}
        className="space-y-6"
      >
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Detail Posting</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Event Code */}
            <div className="space-y-1.5">
              <Label>Kode Event <span className="text-destructive">*</span></Label>
              <Controller
                control={control}
                name="eventCode"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger aria-label="Pilih kode event manual">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {MANUAL_EVENT_CODES.map((e) => (
                        <SelectItem key={e.value} value={e.value} className="text-sm">
                          {e.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              {errors.eventCode && (
                <p role="alert" className="text-xs text-destructive">{errors.eventCode.message}</p>
              )}
            </div>

            {/* Periode */}
            <div className="space-y-1.5">
              <Label htmlFor="periodeId">Periode (YYYY-MM-01) <span className="text-destructive">*</span></Label>
              <Controller
                control={control}
                name="periodeId"
                render={({ field }) => (
                  <Input
                    {...field}
                    id="periodeId"
                    placeholder="2026-06-01"
                    aria-describedby="periode-hint"
                  />
                )}
              />
              <p id="periode-hint" className="text-xs text-muted-foreground">
                Format: YYYY-MM-01 (hari selalu 01, mewakili bulan buku)
              </p>
              {errors.periodeId && (
                <p role="alert" className="text-xs text-destructive">{errors.periodeId.message}</p>
              )}
            </div>

            {/* Instrumen ID */}
            <div className="space-y-1.5">
              <Label htmlFor="instrumenId">ID Instrumen (opsional)</Label>
              <Controller
                control={control}
                name="instrumenId"
                render={({ field }) => (
                  <Input
                    {...field}
                    value={field.value ?? ""}
                    onChange={(e) => field.onChange(e.target.value || null)}
                    id="instrumenId"
                    placeholder="UUID instrumen terkait..."
                  />
                )}
              />
            </div>

            {/* Amount */}
            <div className="space-y-1.5">
              <Label htmlFor="amountIdr">Nominal IDR <span className="text-destructive">*</span></Label>
              <Controller
                control={control}
                name="amountIdr"
                render={({ field }) => (
                  <Input
                    {...field}
                    id="amountIdr"
                    inputMode="decimal"
                    placeholder="1000000.0000"
                    aria-describedby="amount-hint-post"
                  />
                )}
              />
              <p id="amount-hint-post" className="text-xs text-muted-foreground">
                Format: angka dengan maks 4 desimal. Harus lebih dari 0.
              </p>
              {errors.amountIdr && (
                <p role="alert" className="text-xs text-destructive">{errors.amountIdr.message}</p>
              )}
            </div>

            {/* Narasi */}
            <div className="space-y-1.5">
              <Label htmlFor="narasi">Narasi Jurnal <span className="text-destructive">*</span></Label>
              <Controller
                control={control}
                name="narasi"
                render={({ field }) => (
                  <Textarea
                    {...field}
                    id="narasi"
                    rows={3}
                    placeholder="Jelaskan alasan dan konteks posting manual ini..."
                    aria-describedby="narasi-hint"
                  />
                )}
              />
              <p id="narasi-hint" className="text-xs text-muted-foreground">
                Maksimum 500 karakter. Narasi akan dicantumkan dalam setiap baris jurnal.
              </p>
              {errors.narasi && (
                <p role="alert" className="text-xs text-destructive">{errors.narasi.message}</p>
              )}
            </div>
          </CardContent>
        </Card>

        <div className="flex items-center justify-end gap-3">
          <Button
            type="button"
            variant="ghost"
            onClick={() => router.back()}
            disabled={postMutation.isPending}
          >
            Batal
          </Button>
          <Button
            type="submit"
            disabled={postMutation.isPending}
          >
            {postMutation.isPending ? "Memposting..." : "Post Jurnal Manual"}
          </Button>
        </div>
      </form>
    </div>
  );
}

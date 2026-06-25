"use client";

import * as React from "react";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { EventCodePicker } from "@/components/blips/jurnal/EventCodePicker";
import { BalancePreviewCard } from "@/components/blips/jurnal/BalancePreviewCard";
import { JurnalLinesTable } from "@/components/blips/jurnal/JurnalLinesTable";
import { resolverApi } from "@/lib/api/jurnal.api";
import { isApiError } from "@/lib/api";
import {
  resolverRequestSchema,
  type ResolverRequest,
  type ResolverResponse,
  type KlasifikasiPsak71,
} from "@/lib/schemas/jurnal.schema";
import { notify } from "@/lib/notify";

const KLASIFIKASI_OPTIONS: { value: KlasifikasiPsak71; label: string }[] = [
  { value: "AC", label: "AC — Amortised Cost" },
  { value: "FVOCI", label: "FVOCI — Debt" },
  { value: "FVTPL", label: "FVTPL" },
  { value: "FVOCI_ELECTION", label: "FVOCI Election — Equity" },
  { value: "POCI", label: "POCI" },
];

export default function ResolverPage() {
  const [result, setResult] = React.useState<ResolverResponse | null>(null);

  const form = useForm<ResolverRequest>({
    resolver: zodResolver(resolverRequestSchema) as unknown as import("react-hook-form").Resolver<ResolverRequest>,
    defaultValues: {
      eventCode: "",
      klasifikasiPsak71: "AC",
      periodeId: "",
      amountIdr: "",
      currency: "IDR",
      fxRate: "1.00000000",
    },
  });

  const { control, handleSubmit, watch, formState: { errors } } = form;
  const eventCode = watch("eventCode");

  const resolveMutation = useMutation({
    mutationFn: (data: ResolverRequest) => resolverApi.resolve(data),
    onSuccess: (res) => {
      setResult(res.data);
      if (res.data.isBalanced) {
        notify.success(
          `Resolver berhasil — ${res.data.lines.length} baris jurnal. DEBIT = KREDIT = ${
            new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR" }).format(
              parseFloat(res.data.totalDebitIdr),
            )
          }`,
        );
      } else {
        notify.warning("Resolver mengembalikan jurnal yang TIDAK SEIMBANG. Periksa template mapping.");
      }
    },
    onError: (err) => {
      if (isApiError(err)) notify.error(err);
    },
  });

  const lines = result
    ? result.lines.map((l) => ({
        urutan: l.urutan,
        posisi: l.posisi,
        akunId: l.akunId,
        akunKode: l.akunKode,
        akunNama: l.akunNama,
        amountIdr: l.amountIdr,
        narasi: l.narasi,
      }))
    : [];

  return (
    <div className="mx-auto max-w-4xl px-6 py-6 space-y-6">
      <div>
        <h1 className="text-xl font-semibold">Resolver Playground</h1>
        <p className="text-sm text-muted-foreground">
          Uji mapping jurnal secara interaktif tanpa menyimpan data.
          Resolver berjalan &le;100ms dan tidak menulis ke database.
        </p>
      </div>

      <div className="grid grid-cols-[1fr_1fr] gap-6 items-start">
        {/* Input form */}
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Parameter Resolver</CardTitle>
          </CardHeader>
          <CardContent>
            <form
              onSubmit={handleSubmit((data: unknown) => resolveMutation.mutate(data as ResolverRequest))}
              className="space-y-4"
            >
              {/* Event Code */}
              <div className="space-y-1.5">
                <Label>Kode Event <span className="text-destructive">*</span></Label>
                <Controller
                  control={control}
                  name="eventCode"
                  render={({ field }) => (
                    <EventCodePicker
                      value={field.value}
                      onChange={field.onChange}
                      allowCustom={false}
                    />
                  )}
                />
                {errors.eventCode && (
                  <p role="alert" className="text-xs text-destructive">{errors.eventCode.message}</p>
                )}
              </div>

              {/* Klasifikasi */}
              <div className="space-y-1.5">
                <Label>Klasifikasi PSAK 71 <span className="text-destructive">*</span></Label>
                <Controller
                  control={control}
                  name="klasifikasiPsak71"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger aria-label="Pilih klasifikasi">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {KLASIFIKASI_OPTIONS.map((o) => (
                          <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
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
                      pattern="^\d{4}-\d{2}-01$"
                    />
                  )}
                />
                {errors.periodeId && (
                  <p role="alert" className="text-xs text-destructive">{errors.periodeId.message}</p>
                )}
              </div>

              {/* Amount */}
              <div className="space-y-1.5">
                <Label htmlFor="amountIdr">Nominal (IDR) <span className="text-destructive">*</span></Label>
                <Controller
                  control={control}
                  name="amountIdr"
                  render={({ field }) => (
                    <Input
                      {...field}
                      id="amountIdr"
                      placeholder="1000000.0000"
                      inputMode="decimal"
                      aria-describedby="amount-hint"
                    />
                  )}
                />
                <p id="amount-hint" className="text-xs text-muted-foreground">
                  Format: angka dengan maks 4 desimal. Contoh: 1000000.0000
                </p>
                {errors.amountIdr && (
                  <p role="alert" className="text-xs text-destructive">{errors.amountIdr.message}</p>
                )}
              </div>

              {/* Instrumen ID (optional) */}
              <div className="space-y-1.5">
                <Label htmlFor="instrumenId">ID Instrumen (opsional)</Label>
                <Controller
                  control={control}
                  name="instrumenId"
                  render={({ field }) => (
                    <Input
                      {...field}
                      value={field.value ?? ""}
                      id="instrumenId"
                      placeholder="UUID instrumen..."
                    />
                  )}
                />
              </div>

              <Button
                type="submit"
                className="w-full"
                disabled={resolveMutation.isPending || !eventCode}
              >
                {resolveMutation.isPending ? "Menjalankan resolver..." : "Jalankan Resolver"}
              </Button>
            </form>
          </CardContent>
        </Card>

        {/* Result */}
        <div className="space-y-4">
          {result ? (
            <>
              <Card>
                <CardHeader>
                  <CardTitle className="text-sm">
                    Mapping yang Digunakan
                  </CardTitle>
                </CardHeader>
                <CardContent className="text-sm space-y-1">
                  <p>
                    <span className="text-muted-foreground">Kode Event:</span>{" "}
                    <span className="font-mono font-medium">{result.headerUsed.eventCode}</span>
                  </p>
                  <p>
                    <span className="text-muted-foreground">Kategori:</span>{" "}
                    {result.headerUsed.kategoriEvent}
                  </p>
                  {result.headerUsed.namaEvent && (
                    <p>
                      <span className="text-muted-foreground">Nama:</span>{" "}
                      {result.headerUsed.namaEvent}
                    </p>
                  )}
                </CardContent>
              </Card>

              <BalancePreviewCard
                rows={[]}
                resolverLines={lines}
              />

              <JurnalLinesTable lines={lines} showSubtotal showBalanceBadge />
            </>
          ) : (
            <div className="flex flex-col items-center justify-center h-64 rounded-md border border-dashed text-center text-sm text-muted-foreground">
              <p>Hasil resolver akan ditampilkan di sini</p>
              <p className="text-xs mt-1">Isi parameter dan klik &ldquo;Jalankan Resolver&rdquo;</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

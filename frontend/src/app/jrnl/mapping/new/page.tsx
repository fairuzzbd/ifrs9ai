"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
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
import { EventCodePicker } from "@/components/blips/jurnal/EventCodePicker";
import { MappingDetailRowsBuilder } from "@/components/blips/jurnal/MappingDetailRowsBuilder";
import { KlasifikasiCompatibilityChips } from "@/components/blips/jurnal/KlasifikasiCompatibilityChips";
import { BalancePreviewCard } from "@/components/blips/jurnal/BalancePreviewCard";
import { WorkflowPathBadge } from "@/components/blips/jurnal/WorkflowPathBadge";
import { mappingApi } from "@/lib/api/jurnal.api";
import {
  mappingHeaderCreateSchema,
  type MappingHeaderCreateInput,
  type KlasifikasiPsak71,
  isRegulatedCode,
} from "@/lib/schemas/jurnal.schema";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";

async function searchCoa(q: string) {
  const res = await fetch(`/api/v1/master/coa?q=${encodeURIComponent(q)}&limit=10`);
  if (!res.ok) return [];
  const json = await res.json();
  return (json.data ?? []).map((c: { id: string; kode_akun: string; nama_akun: string }) => ({
    id: c.id,
    kodeAkun: c.kode_akun,
    namaAkun: c.nama_akun,
  }));
}

export default function MappingJurnalNewPage() {
  const router = useRouter();
  const idempotencyKey = React.useRef(uuidv4());

  const form = useForm<MappingHeaderCreateInput>({
    resolver: zodResolver(mappingHeaderCreateSchema) as unknown as import("react-hook-form").Resolver<MappingHeaderCreateInput>,
    defaultValues: {
      eventCode: "",
      namaEvent: "",
      kategoriEvent: "PENEMPATAN",
      triggerSource: "SYSTEM_JOB",
      klasifikasiBerlaku: null,
      deskripsi: "",
      detailRows: [],
    },
  });

  const { watch, control, setValue, handleSubmit, formState: { errors } } = form;
  const eventCode = watch("eventCode");
  const detailRows = watch("detailRows");
  const klasifikasiBerlaku = watch("klasifikasiBerlaku");
  const isRegulated = isRegulatedCode(eventCode);

  const createMutation = useMutation({
    mutationFn: (data: MappingHeaderCreateInput) =>
      mappingApi.create(data, idempotencyKey.current),
    onSuccess: (res) => {
      notify.success(
        `Mapping ${res.data.eventCode} berhasil dibuat. Menunggu review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/jrnl/mapping/${res.data.id}`),
          },
        },
      );
      router.push(`/jrnl/mapping/${res.data.id}`);
    },
    onError: (err) => {
      if (isApiError(err)) notify.error(err);
      else notify.error({ code: "INTERNAL", message: String(err), traceId: "" });
    },
  });

  const onSubmit = (data: unknown) => {
    createMutation.mutate(data as MappingHeaderCreateInput);
  };

  const formErrors = Object.fromEntries(
    Object.entries(errors).flatMap(([key, val]) => {
      if (key === "detailRows" && Array.isArray(val)) {
        return val.flatMap((rowErr, i) =>
          Object.entries(rowErr ?? {}).map(([field, e]) => [
            `detailRows.${i}.${field}`,
            (e as { message?: string }).message ?? "",
          ]),
        );
      }
      return [[key, (val as { message?: string })?.message ?? ""]];
    }),
  );

  return (
    <div className="mx-auto max-w-5xl px-6 py-6 space-y-6">
      <div>
        <h1 className="text-xl font-semibold">Buat Mapping Jurnal</h1>
        <p className="text-sm text-muted-foreground">
          Mapping event code ke template baris jurnal DEBIT/KREDIT
        </p>
      </div>

      <form onSubmit={handleSubmit(onSubmit)} className="space-y-6">
        {/* Identity */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Identitas Mapping</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              {/* Event Code */}
              <div className="space-y-1.5">
                <Label htmlFor="eventCode">
                  Kode Event <span className="text-destructive">*</span>
                </Label>
                <Controller
                  control={control}
                  name="eventCode"
                  render={({ field }) => (
                    <EventCodePicker
                      value={field.value}
                      onChange={(code) => {
                        field.onChange(code);
                        // auto-set namaEvent from metadata
                        import("@/lib/schemas/jurnal.schema").then(({ EVENT_CODE_METADATA }) => {
                          const meta = EVENT_CODE_METADATA.find((m) => m.eventCode === code);
                          if (meta) setValue("namaEvent", meta.namaEvent);
                        });
                      }}
                      allowCustom={false}
                    />
                  )}
                />
                {errors.eventCode && (
                  <p role="alert" className="text-xs text-destructive">{errors.eventCode.message}</p>
                )}
                {isRegulated && (
                  <div className="flex items-center gap-2 mt-1">
                    <WorkflowPathBadge path="6-eyes" size="sm" showTooltip />
                    <span className="text-xs text-purple-700">Event teregulasi — workflow 6-eyes</span>
                  </div>
                )}
              </div>

              {/* Nama Event */}
              <div className="space-y-1.5">
                <Label htmlFor="namaEvent">
                  Nama Event <span className="text-destructive">*</span>
                </Label>
                <Controller
                  control={control}
                  name="namaEvent"
                  render={({ field }) => (
                    <Input
                      {...field}
                      id="namaEvent"
                      placeholder="Nama event jurnal..."
                      aria-describedby="namaEvent-err"
                    />
                  )}
                />
                {errors.namaEvent && (
                  <p id="namaEvent-err" role="alert" className="text-xs text-destructive">
                    {errors.namaEvent.message}
                  </p>
                )}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              {/* Kategori */}
              <div className="space-y-1.5">
                <Label>Kategori Event <span className="text-destructive">*</span></Label>
                <Controller
                  control={control}
                  name="kategoriEvent"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger aria-label="Pilih kategori event">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {["PENEMPATAN","AKRUAL","ECL","MUTASI_MTM","STAGE_MIGRATION","CLOSURE","REKLASIFIKASI","FX","KOREKSI"].map((k) => (
                          <SelectItem key={k} value={k}>{k}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>

              {/* Trigger Source */}
              <div className="space-y-1.5">
                <Label>Trigger Source <span className="text-destructive">*</span></Label>
                <Controller
                  control={control}
                  name="triggerSource"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger aria-label="Pilih trigger source">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="SYSTEM_JOB">System Job (Otomatis)</SelectItem>
                        <SelectItem value="USER_INPUT">User Input (Manual)</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>
            </div>

            {/* Deskripsi */}
            <div className="space-y-1.5">
              <Label htmlFor="deskripsi">Deskripsi</Label>
              <Controller
                control={control}
                name="deskripsi"
                render={({ field }) => (
                  <Textarea
                    {...field}
                    value={field.value ?? ""}
                    id="deskripsi"
                    rows={2}
                    placeholder="Deskripsi singkat tentang mapping ini..."
                  />
                )}
              />
            </div>

            {/* Klasifikasi */}
            <div className="space-y-1.5">
              <Label>Klasifikasi PSAK 71 Berlaku</Label>
              <Controller
                control={control}
                name="klasifikasiBerlaku"
                render={({ field }) => (
                  <KlasifikasiCompatibilityChips
                    selectedEventCode={eventCode || null}
                    value={field.value ?? []}
                    onChange={(v) => field.onChange(v.length === 0 ? null : v)}
                    allowNull
                  />
                )}
              />
            </div>
          </CardContent>
        </Card>

        {/* Detail rows */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Template Baris Jurnal</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <Controller
              control={control}
              name="detailRows"
              render={({ field }) => (
                <MappingDetailRowsBuilder
                  value={field.value}
                  onChange={field.onChange}
                  klasifikasiBerlaku={
                    (klasifikasiBerlaku as KlasifikasiPsak71[] | null) ?? []
                  }
                  onSearchCoa={searchCoa}
                  errors={formErrors}
                />
              )}
            />
            {errors.detailRows?.message && (
              <p role="alert" className="text-sm text-destructive">
                {errors.detailRows.message}
              </p>
            )}

            <BalancePreviewCard rows={detailRows} />
          </CardContent>
        </Card>

        {/* Actions */}
        <div className="flex items-center justify-end gap-3">
          <Button
            type="button"
            variant="ghost"
            onClick={() => router.back()}
            disabled={createMutation.isPending}
          >
            Batal
          </Button>
          <Button type="submit" disabled={createMutation.isPending}>
            {createMutation.isPending ? "Menyimpan..." : "Simpan & Submit ke Review"}
          </Button>
        </div>
      </form>
    </div>
  );
}

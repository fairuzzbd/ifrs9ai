"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
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
import { MappingDetailRowsBuilder } from "@/components/blips/jurnal/MappingDetailRowsBuilder";
import { KlasifikasiCompatibilityChips } from "@/components/blips/jurnal/KlasifikasiCompatibilityChips";
import { BalancePreviewCard } from "@/components/blips/jurnal/BalancePreviewCard";
import { mappingApi } from "@/lib/api/jurnal.api";
import {
  mappingHeaderEditSchema,
  type MappingHeaderEditInput,
  type MappingDetailRow,
  type KlasifikasiPsak71,
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

export default function MappingJurnalEditPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const idempotencyKey = React.useRef(uuidv4());

  const { data, isLoading } = useQuery({
    queryKey: ["mapping-jurnal", id],
    queryFn: () => mappingApi.get(id),
  });

  const mapping = data?.data;
  const isDraft = mapping?.workflowStatus === "DRAFT";

  const form = useForm<MappingHeaderEditInput>({
    resolver: zodResolver(mappingHeaderEditSchema) as unknown as import("react-hook-form").Resolver<MappingHeaderEditInput>,
    values: mapping
      ? {
          eventCode: mapping.eventCode,
          namaEvent: mapping.namaEvent,
          kategoriEvent: mapping.kategoriEvent,
          triggerSource: mapping.triggerSource,
          klasifikasiBerlaku: mapping.klasifikasiBerlaku,
          deskripsi: mapping.deskripsi ?? "",
          detailRows: mapping.detailRows.map((r) => ({
            id: r.id,
            _clientKey: r.id,
            urutan: r.urutan,
            dkIndicator: r.dkIndicator,
            kodeAkunId: r.kodeAkunId,
            kodeAkunDisplay: r.kodeAkunDisplay,
            namaAkun: r.namaAkun,
            sumberAmount: r.sumberAmount,
            multiplier: r.multiplier,
            klasifikasiFilter: r.klasifikasiFilter as KlasifikasiPsak71 | null,
            catatan: r.catatan ?? "",
          })),
        }
      : undefined,
  });

  const { control, watch, handleSubmit, formState: { errors } } = form;
  const detailRows = watch("detailRows") ?? [];
  const eventCode = watch("eventCode");
  const klasifikasiBerlaku = watch("klasifikasiBerlaku");

  const editMutation = useMutation({
    mutationFn: (data: MappingHeaderEditInput) =>
      mappingApi.edit(id, data, idempotencyKey.current),
    onSuccess: () => {
      notify.success(`Mapping ${mapping?.eventCode ?? ""} berhasil diperbarui.`, {
        action: {
          label: "Lihat detail",
          onClick: () => router.push(`/jrnl/mapping/${id}`),
        },
      });
      router.push(`/jrnl/mapping/${id}`);
    },
    onError: (err) => {
      if (isApiError(err)) notify.error(err);
    },
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-24 text-sm text-muted-foreground">
        Memuat data mapping...
      </div>
    );
  }

  if (!mapping) {
    return (
      <div className="flex flex-col items-center py-24 gap-4">
        <p className="text-sm text-muted-foreground">Mapping tidak ditemukan.</p>
        <Button variant="outline" onClick={() => router.back()}>Kembali</Button>
      </div>
    );
  }

  if (!isDraft) {
    return (
      <div className="mx-auto max-w-3xl px-6 py-12">
        <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          Mapping dengan status &ldquo;{mapping.workflowStatus}&rdquo; tidak bisa diedit. Tarik ke Draft terlebih dahulu.
        </div>
        <Button
          variant="outline"
          className="mt-4"
          onClick={() => router.push(`/jrnl/mapping/${id}`)}
        >
          Kembali ke Detail
        </Button>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl px-6 py-6 space-y-6">
      <div>
        <h1 className="text-xl font-semibold">Edit Mapping — <span className="font-mono">{mapping.eventCode}</span></h1>
        <p className="text-sm text-muted-foreground">Hanya mapping berstatus DRAFT yang bisa diedit.</p>
      </div>

      <form onSubmit={handleSubmit((data: unknown) => editMutation.mutate(data as MappingHeaderEditInput))} className="space-y-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Identitas Mapping</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label>Kode Event</Label>
                <Input value={mapping.eventCode} disabled className="bg-muted" />
                <p className="text-xs text-muted-foreground">Kode event tidak bisa diubah setelah dibuat.</p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="namaEvent">Nama Event <span className="text-destructive">*</span></Label>
                <Controller
                  control={control}
                  name="namaEvent"
                  render={({ field }) => (
                    <Input {...field} id="namaEvent" placeholder="Nama event..." />
                  )}
                />
                {errors.namaEvent && (
                  <p role="alert" className="text-xs text-destructive">{errors.namaEvent.message}</p>
                )}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label>Kategori Event</Label>
                <Controller
                  control={control}
                  name="kategoriEvent"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger>
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
              <div className="space-y-1.5">
                <Label>Trigger Source</Label>
                <Controller
                  control={control}
                  name="triggerSource"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="SYSTEM_JOB">System Job</SelectItem>
                        <SelectItem value="USER_INPUT">User Input</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <Label>Deskripsi</Label>
              <Controller
                control={control}
                name="deskripsi"
                render={({ field }) => (
                  <Textarea {...field} value={field.value ?? ""} rows={2} placeholder="Deskripsi..." />
                )}
              />
            </div>

            <div className="space-y-1.5">
              <Label>Klasifikasi PSAK 71 Berlaku</Label>
              <Controller
                control={control}
                name="klasifikasiBerlaku"
                render={({ field }) => (
                  <KlasifikasiCompatibilityChips
                    selectedEventCode={eventCode ?? null}
                    value={field.value ?? []}
                    onChange={(v) => field.onChange(v.length === 0 ? null : v)}
                    allowNull
                  />
                )}
              />
            </div>
          </CardContent>
        </Card>

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
                  value={field.value ?? []}
                  onChange={field.onChange}
                  klasifikasiBerlaku={(klasifikasiBerlaku as KlasifikasiPsak71[] | null) ?? []}
                  onSearchCoa={searchCoa}
                />
              )}
            />
            <BalancePreviewCard rows={detailRows as MappingDetailRow[]} />
          </CardContent>
        </Card>

        <div className="flex justify-end gap-3">
          <Button type="button" variant="ghost" onClick={() => router.push(`/jrnl/mapping/${id}`)}>
            Batal
          </Button>
          <Button type="submit" disabled={editMutation.isPending}>
            {editMutation.isPending ? "Menyimpan..." : "Simpan Perubahan"}
          </Button>
        </div>
      </form>
    </div>
  );
}

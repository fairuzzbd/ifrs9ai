"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { JurnalLinesTable } from "@/components/blips/jurnal/JurnalLinesTable";
import { GlDeliveryStatusPanel } from "@/components/blips/gl-delivery/GlDeliveryStatusPanel";
import { jurnalQueryApi } from "@/lib/api/jurnal.api";
import type { JurnalLine } from "@/lib/schemas/jurnal.schema";

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

const STATUS_VARIANT: Record<string, string> = {
  POSTED: "default",
  REVERSED: "destructive",
  PENDING_APPROVAL: "secondary",
};

const STATUS_LABELS: Record<string, string> = {
  POSTED: "Terposting",
  REVERSED: "Direverse",
  PENDING_APPROVAL: "Menunggu Persetujuan",
};

export default function JurnalEntryDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, error } = useQuery({
    queryKey: ["jurnal-entry", id],
    queryFn: () => jurnalQueryApi.get(id),
  });

  const jurnal = data?.data;

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-24 text-sm text-muted-foreground">
        Memuat data jurnal...
      </div>
    );
  }

  if (error || !jurnal) {
    return (
      <div className="flex flex-col items-center py-24 gap-4">
        <p className="text-sm text-muted-foreground">Entri jurnal tidak ditemukan.</p>
        <Button variant="outline" onClick={() => router.back()}>Kembali</Button>
      </div>
    );
  }

  const lines: JurnalLine[] = jurnal.detailLines.map((l) => ({
    urutan: l.urutan,
    posisi: parseFloat(l.debitAmount) > 0 ? "DEBIT" : "KREDIT",
    akunId: l.kodeAkunId,
    akunKode: l.kodeAkun,
    akunNama: l.namaAkun,
    amountIdr: parseFloat(l.debitAmount) > 0 ? l.debitAmount : l.kreditAmount,
    narasi: l.narrativeLine,
  }));

  return (
    <div className="mx-auto max-w-4xl px-6 py-6 space-y-6">
      {/* Top nav */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.push("/jrnl/journal-entries")}
          aria-label="Kembali ke daftar jurnal"
        >
          <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
          Daftar Jurnal
        </Button>
      </div>

      {/* Header info */}
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-semibold font-mono">{jurnal.noJurnal}</h1>
            <Badge
              variant={
                (STATUS_VARIANT[jurnal.statusInternal] as "default" | "secondary" | "destructive" | "outline") ?? "outline"
              }
            >
              {STATUS_LABELS[jurnal.statusInternal] ?? jurnal.statusInternal}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground mt-0.5">
            {jurnal.eventCode} · Periode {jurnal.periodeLabel}
          </p>
        </div>
        <div className="text-right text-xs text-muted-foreground">
          <p>Tgl Posting: {new Date(jurnal.tanggalPosting).toLocaleDateString("id-ID")}</p>
          <p>Dibuat: {new Date(jurnal.createdAt).toLocaleDateString("id-ID")}</p>
        </div>
      </div>

      {/* Info card */}
      <div className="grid grid-cols-2 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs text-muted-foreground">Instrumen</CardTitle>
          </CardHeader>
          <CardContent className="text-sm">
            {jurnal.instrumenNama ?? "-"}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs text-muted-foreground">Mata Uang</CardTitle>
          </CardHeader>
          <CardContent className="text-sm font-mono">
            {jurnal.currency}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs text-muted-foreground">Total Debit</CardTitle>
          </CardHeader>
          <CardContent className="text-sm font-mono font-semibold text-blue-700">
            {IDR.format(parseFloat(jurnal.totalDebit || "0"))}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs text-muted-foreground">Total Kredit</CardTitle>
          </CardHeader>
          <CardContent className="text-sm font-mono font-semibold text-green-700">
            {IDR.format(parseFloat(jurnal.totalKredit || "0"))}
          </CardContent>
        </Card>
      </div>

      {/* Narrative */}
      {jurnal.narrative && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs text-muted-foreground">Narasi</CardTitle>
          </CardHeader>
          <CardContent className="text-sm">{jurnal.narrative}</CardContent>
        </Card>
      )}

      {/* Detail lines */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Baris Jurnal ({jurnal.detailLines.length} baris)</CardTitle>
        </CardHeader>
        <CardContent>
          <JurnalLinesTable lines={lines} showSubtotal showBalanceBadge />
        </CardContent>
      </Card>

      {/* GL Delivery Status — P5-M3 (S2-AC1/2/3) */}
      <GlDeliveryStatusPanel
        jurnalHeaderId={id}
        jurnalNumber={jurnal.noJurnal}
      />

      {/* Audit trail */}
      <div className="text-xs text-muted-foreground flex flex-wrap gap-4">
        {jurnal.referenceEventType && (
          <span>Dari: {jurnal.referenceEventType}</span>
        )}
        {jurnal.referenceEventId && (
          <span>Ref ID: <span className="font-mono">{jurnal.referenceEventId}</span></span>
        )}
        {jurnal.idempotencyKey && (
          <span>Idempotency: <span className="font-mono">{jurnal.idempotencyKey}</span></span>
        )}
      </div>
    </div>
  );
}

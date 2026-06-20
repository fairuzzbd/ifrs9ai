"use client";

import * as React from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, AlertTriangle, ShieldAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PenjualanBMRiskBadge } from "@/components/blips/penjualan/PenjualanBMRiskBadge";
import { penjualanBMApi, penjualanQueryKeys } from "@/lib/api/penjualan.api";
import type { BMAlertItem } from "@/lib/schemas/penjualan.schema";

// ---------------------------------------------------------------------------
// Page — BM Frequency Alerts (S4-AC1..AC4)
// ---------------------------------------------------------------------------

export default function PenjualanBMAlertPage() {
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: penjualanQueryKeys.bmAlerts(),
    queryFn: () => penjualanBMApi.bmAlerts(),
    staleTime: 60_000,
  });

  const alerts: BMAlertItem[] = Array.isArray(data?.data) ? (data?.data as BMAlertItem[]) : [];

  return (
    <div className="container mx-auto max-w-4xl py-6 space-y-6">
      <div className="flex items-center gap-3">
        <Link href="/transaksi/penjualan">
          <Button variant="ghost" size="sm" aria-label="Kembali ke daftar penjualan">
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            Kembali
          </Button>
        </Link>
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <AlertTriangle className="h-6 w-6 text-orange-500" aria-hidden="true" />
            BM Frequency Alerts
          </h1>
          <p className="text-sm text-muted-foreground">
            Instrumen HTC dengan disposal kumulatif 12-bulan melebihi threshold peringatan (PSAK 71 §4.1.2b)
          </p>
        </div>
      </div>

      {isLoading && (
        <div className="text-center text-muted-foreground py-10" aria-busy="true">
          Memuat data BM alerts...
        </div>
      )}

      {isError && (
        <div className="text-center py-10">
          <p className="text-red-600">Gagal memuat BM alerts.</p>
          <Button onClick={() => void refetch()} className="mt-3">Coba Lagi</Button>
        </div>
      )}

      {!isLoading && !isError && alerts.length === 0 && (
        <div className="text-center py-10 text-muted-foreground">
          <ShieldAlert className="h-10 w-10 mx-auto mb-2 text-green-400" aria-hidden="true" />
          <p>Tidak ada instrumen HTC yang melebihi threshold peringatan BM saat ini.</p>
        </div>
      )}

      {alerts.length > 0 && (
        <div className="space-y-3">
          {alerts.map((alert) => (
            <div
              key={alert.instrumenId}
              className={`rounded-md border p-4 space-y-2 ${
                alert.flagStatus === "BM_VIOLATION_BLOCK"
                  ? "border-red-300 bg-red-50"
                  : "border-orange-300 bg-orange-50"
              }`}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="font-mono font-semibold">{alert.instrumenKode}</span>
                  <PenjualanBMRiskBadge
                    flag={alert.flagStatus}
                    pct={alert.cumulativeSold12mPct}
                    warnThreshold={alert.warnThresholdPct}
                    blockThreshold={alert.blockThresholdPct}
                    size="sm"
                  />
                </div>
                <span className="text-xs text-muted-foreground">
                  {new Date(alert.lastUpdated).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}
                </span>
              </div>
              <dl className="grid grid-cols-3 gap-x-4 text-xs">
                <div>
                  <dt className="text-muted-foreground">Portofolio</dt>
                  <dd className="font-medium">{alert.portofolioNama}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Disposal Kumulatif 12-bln</dt>
                  <dd className="font-mono font-bold">{alert.cumulativeSold12mPct}%</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Threshold</dt>
                  <dd className="font-mono">
                    Warn: {alert.warnThresholdPct}% / Block: {alert.blockThresholdPct}%
                  </dd>
                </div>
              </dl>
              {alert.flagStatus === "BM_VIOLATION_BLOCK" && (
                <p className="text-xs text-red-700 font-medium">
                  Penjualan dari instrumen ini memerlukan approval ROLE-RISK sebelum bisa diposting.
                </p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

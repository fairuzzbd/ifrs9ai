"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsInteger, parseAsString } from "nuqs";
import { ArrowLeft, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { PociDashboardCards } from "@/components/blips/poci/PociDashboardCards";
import { pociDashboardApi, pociQueryKeys } from "@/lib/api/poci.api";

const BULAN_NAMES = [
  "Januari", "Februari", "Maret", "April", "Mei", "Juni",
  "Juli", "Agustus", "September", "Oktober", "November", "Desember",
];

function PociDashboardContent() {
  const now = new Date();
  const [year, setYear] = useQueryState("year", parseAsInteger.withDefault(now.getFullYear()));
  const [month, setMonth] = useQueryState("month", parseAsInteger.withDefault(now.getMonth() + 1));
  const [portofolioId] = useQueryState("portofolio_id", parseAsString.withDefault(""));

  const params = {
    year,
    month,
    portofolio_id: portofolioId || undefined,
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: pociQueryKeys.dashboard(params),
    queryFn: () => pociDashboardApi.summary(params),
    staleTime: 60_000,
  });

  return (
    <div className="container mx-auto py-6 space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/poci/delta-log" aria-label="Kembali ke delta log">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
              Kembali
            </Button>
          </Link>
          <div>
            <h1 className="text-2xl font-bold">Dashboard POCI Delta ECL</h1>
            <p className="text-sm text-muted-foreground">
              MTD/YTD aggregate delta ECL POCI per portofolio — APP-C P5-M10
            </p>
          </div>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void refetch()}
          aria-label="Refresh data dashboard POCI"
        >
          <RefreshCw className="h-4 w-4 mr-1" aria-hidden="true" />
          Refresh
        </Button>
      </div>

      {/* Period selector */}
      <div className="flex items-center gap-3 p-3 rounded-md border bg-muted/30">
        <span className="text-sm font-medium">Periode:</span>
        <select
          className="text-sm border rounded px-2 py-1 bg-background"
          value={month}
          onChange={(e) => void setMonth(Number(e.target.value))}
          aria-label="Pilih bulan"
        >
          {BULAN_NAMES.map((name, i) => (
            <option key={i + 1} value={i + 1}>{name}</option>
          ))}
        </select>
        <select
          className="text-sm border rounded px-2 py-1 bg-background"
          value={year}
          onChange={(e) => void setYear(Number(e.target.value))}
          aria-label="Pilih tahun"
        >
          {[2024, 2025, 2026, 2027].map((y) => (
            <option key={y} value={y}>{y}</option>
          ))}
        </select>
      </div>

      {isLoading && (
        <div className="rounded-lg border p-8 text-center text-muted-foreground">
          Memuat data dashboard...
        </div>
      )}
      {isError && (
        <div
          className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700"
          role="alert"
        >
          Gagal memuat dashboard. Periksa koneksi atau coba refresh.
        </div>
      )}
      {data?.data && <PociDashboardCards data={data.data} />}
    </div>
  );
}

export default function PociDashboardPage() {
  return (
    <Suspense>
      <PociDashboardContent />
    </Suspense>
  );
}

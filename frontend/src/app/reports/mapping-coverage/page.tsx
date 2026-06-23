/**
 * Route: /reports/mapping-coverage
 * Story: P5-M12-S4 — RPT-19 Mapping Coverage Dashboard
 * Actors: ROLE-AKUN, ROLE-AKUN-CTL, ROLE-RISK, ROLE-AUDIT
 */

"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Download, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { MappingCoverageCard } from "@/components/blips/mapping-jurnal/MappingCoverageCard";
import { mappingReportsApi, mappingP12QueryKeys } from "@/lib/api/mapping-jurnal-p12.api";
import { notify } from "@/lib/notify";

function Rpt19Content() {
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: mappingP12QueryKeys.rpt19(),
    queryFn: () => mappingReportsApi.getRpt19(),
    staleTime: 60_000,
  });

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4 max-w-4xl">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-48" />
      </div>
    );
  }

  if (isError || !data?.data) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">Gagal memuat RPT-19.</p>
        <Button variant="outline" onClick={() => void refetch()}>Coba Lagi</Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto py-6 space-y-6 max-w-4xl">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/reports" className="hover:underline">Laporan</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">RPT-19 Mapping Coverage</span>
      </nav>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">RPT-19 — Mapping Coverage Dashboard</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Status coverage mapping jurnal per event code. GAP = event tanpa APPROVED_ACTIVE mapping.
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={() => { void refetch(); }}
            aria-label="Refresh RPT-19"
          >
            <RefreshCw className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Refresh
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              window.open(mappingReportsApi.exportRpt19Url("xlsx"), "_blank");
              notify.info("Export RPT-19 XLSX dimulai.");
            }}
            aria-label="Export RPT-19 XLSX"
          >
            <Download className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Export XLSX
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              window.open(mappingReportsApi.exportRpt19Url("csv"), "_blank");
              notify.info("Export RPT-19 CSV dimulai.");
            }}
            aria-label="Export RPT-19 CSV"
          >
            Export CSV
          </Button>
        </div>
      </div>

      <MappingCoverageCard coverage={data.data} />
    </div>
  );
}

export default function Rpt19Page() {
  return (
    <Suspense>
      <Rpt19Content />
    </Suspense>
  );
}

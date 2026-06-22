/**
 * Route: /reports/mapping-validation
 * Story: P5-M12-S5-AC1..2 — RPT-20 Mapping Validation Report
 * Actors: ROLE-AKUN, ROLE-RISK (pre-checklist sebelum approve-2), ROLE-AUDIT
 */

"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, XCircle, Download, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { mappingReportsApi, mappingP12QueryKeys } from "@/lib/api/mapping-jurnal-p12.api";
import { notify } from "@/lib/notify";
import type { Rpt20Issue } from "@/lib/schemas/mapping-jurnal-p12.schema";

function IssueRow({ issue }: { issue: Rpt20Issue }) {
  return (
    <div className="flex items-start justify-between gap-3 py-3 border-b last:border-0">
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex items-center gap-2 flex-wrap">
          <code className="font-mono text-sm font-bold">{issue.eventCode}</code>
          <div className="flex gap-1 flex-wrap">
            {issue.errorCodes.map((code) => (
              <Badge key={code} variant="outline" className="text-xs text-destructive border-destructive/30">
                {code}
              </Badge>
            ))}
          </div>
        </div>
        <p className="text-xs text-muted-foreground">{issue.details}</p>
      </div>
      <Link
        href={`/mapping-jurnal/${issue.eventCode}`}
        className="text-xs text-primary hover:underline shrink-0"
        aria-label={`Lihat dan perbaiki mapping ${issue.eventCode}`}
      >
        Perbaiki &rarr;
      </Link>
    </div>
  );
}

function Rpt20Content() {
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: mappingP12QueryKeys.rpt20(),
    queryFn: () => mappingReportsApi.getRpt20(),
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
        <p className="text-sm text-destructive">Gagal memuat RPT-20.</p>
        <Button variant="outline" onClick={() => void refetch()}>Coba Lagi</Button>
      </div>
    );
  }

  const rpt = data.data;
  const allValid = rpt.invalidMappings === 0;

  return (
    <div className="container mx-auto py-6 space-y-6 max-w-4xl">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/reports" className="hover:underline">Laporan</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">RPT-20 Mapping Validation</span>
      </nav>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">RPT-20 — Mapping Validation</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Validasi setiap ACTIVE mapping: akun non-null, balanced D/K, akun valid di COA.
            ROLE-RISK menggunakan laporan ini sebagai pre-checklist sebelum approve-2.
          </p>
        </div>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={() => void refetch()} aria-label="Refresh RPT-20">
            <RefreshCw className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Refresh
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              window.open(mappingReportsApi.exportRpt20Url("xlsx"), "_blank");
              notify.info("Export RPT-20 XLSX dimulai.");
            }}
            aria-label="Export RPT-20 XLSX"
          >
            <Download className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Export XLSX
          </Button>
        </div>
      </div>

      {/* KPI summary */}
      <div className="grid grid-cols-3 gap-3">
        <div className="rounded-lg border p-3 text-center">
          <p className="text-2xl font-bold">{rpt.totalActiveMappings}</p>
          <p className="text-xs text-muted-foreground mt-0.5">Total Aktif</p>
        </div>
        <div className="rounded-lg border border-green-200 bg-green-50 p-3 text-center">
          <p className="text-2xl font-bold text-green-700">{rpt.validMappings}</p>
          <p className="text-xs text-green-600 mt-0.5">Valid</p>
        </div>
        <div className="rounded-lg border border-red-200 bg-red-50 p-3 text-center">
          <p className="text-2xl font-bold text-red-700">{rpt.invalidMappings}</p>
          <p className="text-xs text-red-600 mt-0.5">Issues</p>
        </div>
      </div>

      {allValid ? (
        <div className="flex items-center gap-2 rounded-lg border border-green-200 bg-green-50 p-4">
          <CheckCircle2 className="h-5 w-5 text-green-600 shrink-0" aria-hidden="true" />
          <span className="text-sm text-green-700 font-medium">
            Semua mapping aktif valid. Aman untuk approve-2.
          </span>
        </div>
      ) : (
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center gap-2">
              <XCircle className="h-4 w-4 text-destructive" aria-hidden="true" />
              <CardTitle className="text-sm text-destructive">
                Issues ({rpt.issues.length} mapping bermasalah)
              </CardTitle>
            </div>
          </CardHeader>
          <CardContent>
            {rpt.issues.map((issue) => (
              <IssueRow key={issue.headerId} issue={issue} />
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

export default function Rpt20Page() {
  return (
    <Suspense>
      <Rpt20Content />
    </Suspense>
  );
}

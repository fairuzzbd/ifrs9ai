"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";

import { eirApi } from "@/lib/api/eir.api";
import type { DriftReportEntry } from "@/lib/schemas/eir.schema";
import { DataTable } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function fmtEir(s: string): string {
  const n = parseFloat(s);
  if (isNaN(n)) return s;
  return `${(n * 100).toFixed(4)}%`;
}

const SEVERITY_VARIANT: Record<
  string,
  "default" | "secondary" | "destructive" | "outline"
> = {
  HIGH: "destructive",
  MEDIUM: "default",
  LOW: "secondary",
  MISSING: "outline",
};

function buildColumns(router: ReturnType<typeof useRouter>): ColumnDef<DriftReportEntry>[] {
  return [
    {
      id: "instrumen",
      header: "Instrumen",
      cell: ({ row }) => (
        <button
          className="text-sm font-medium text-primary hover:underline"
          onClick={() =>
            router.push(`/ecl/eir/instrumen/${row.original.instrumenId}`)
          }
        >
          {row.original.kodeInstrumen ?? row.original.instrumenId.slice(0, 8)}
        </button>
      ),
    },
    {
      id: "namaInstrumen",
      header: "Nama",
      cell: ({ row }) => (
        <span className="text-sm text-muted-foreground">
          {row.original.namaInstrumen ?? "—"}
        </span>
      ),
    },
    {
      id: "eirStored",
      header: "EIR Tersimpan",
      cell: ({ row }) => (
        <span className="text-sm font-mono">{fmtEir(row.original.eirStored)}</span>
      ),
    },
    {
      id: "eirComputed",
      header: "EIR Terhitung",
      cell: ({ row }) => (
        <span className="text-sm font-mono">{fmtEir(row.original.eirComputed)}</span>
      ),
    },
    {
      id: "deltaBp",
      header: "Delta (bp)",
      enableSorting: true,
      cell: ({ row }) => {
        const bp = row.original.deltaBp ?? Math.round(parseFloat(row.original.delta) * 10000);
        return (
          <span className={`text-sm font-mono ${bp > 0 ? "text-amber-600" : bp < 0 ? "text-blue-600" : "text-muted-foreground"}`}>
            {bp} bp
          </span>
        );
      },
    },
    {
      id: "severity",
      header: "Severity",
      cell: ({ row }) => (
        <Badge variant={SEVERITY_VARIANT[row.original.severity] ?? "secondary"}>
          {row.original.severity}
        </Badge>
      ),
    },
    {
      id: "proposalStatus",
      header: "Proposal",
      cell: ({ row }) => {
        if (!row.original.proposalId) return <span className="text-xs text-muted-foreground">Belum ada</span>;
        return (
          <button
            className="text-xs text-primary hover:underline"
            onClick={() =>
              router.push(`/ecl/eir/amendments/${row.original.proposalId}`)
            }
          >
            {row.original.proposalStatus ?? "Ada"}
          </button>
        );
      },
    },
  ];
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function DriftReportDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const router = useRouter();

  const [cursor, setCursor] = React.useState<string | null>(null);
  const [pageNumber, setPageNumber] = React.useState(1);
  const [prevCursors, setPrevCursors] = React.useState<string[]>([]);

  const { data, isLoading } = useQuery({
    queryKey: ["eir-drift-report", id],
    queryFn: () => eirApi.getDriftReport(id),
    enabled: !!id,
  });

  const report = data?.data;
  const entries = report?.entries ?? [];

  const columns = buildColumns(router);

  if (isLoading) {
    return (
      <div className="p-6">
        <div className="h-40 rounded-lg bg-muted animate-pulse" />
      </div>
    );
  }

  if (!report) {
    return (
      <div className="p-6">
        <p>Drift report tidak ditemukan.</p>
        <Button variant="link" onClick={() => router.push("/ecl/eir/drift-reports")}>
          Kembali
        </Button>
      </div>
    );
  }

  const TRIGGER_LABELS: Record<string, string> = {
    CRON_DAILY: "Cron Harian",
    AD_HOC: "Ad-Hoc",
    PRE_ECL_CALC: "Pre-ECL Calc",
  };

  return (
    <div className="p-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex gap-1">
          <li>
            <button className="hover:underline" onClick={() => router.push("/ecl/eir/drift-reports")}>
              Drift Reports
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">{id.slice(0, 12)}…</li>
        </ol>
      </nav>

      <h1 className="text-xl font-semibold">Detail EIR Drift Report</h1>

      {/* Summary card */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Ringkasan</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 text-sm">
            <div>
              <p className="text-xs text-muted-foreground">Sumber</p>
              <p>{TRIGGER_LABELS[report.triggerSource] ?? report.triggerSource}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Mulai Scan</p>
              <p>{new Date(report.scanStartedAt).toLocaleString("id-ID")}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Total Dipindai</p>
              <p className="font-mono">{report.totalScanned.toLocaleString("id-ID")}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Drift Ditemukan</p>
              <p className={`font-mono font-medium ${report.driftCount > 0 ? "text-amber-600" : "text-green-600"}`}>
                {report.driftCount}
              </p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Proposal Auto-Dibuat</p>
              <p className="font-mono">{report.proposalsAutoCreated}</p>
            </div>
            {report.scheduleMissingCount !== undefined && (
              <div>
                <p className="text-xs text-muted-foreground">Schedule Kosong</p>
                <p className={`font-mono ${report.scheduleMissingCount > 0 ? "text-destructive" : ""}`}>
                  {report.scheduleMissingCount}
                </p>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Entry table */}
      {entries.length > 0 ? (
        <DataTable
          columns={columns}
          data={entries}
          emptyMessage="Tidak ada drift entry dalam laporan ini."
        />
      ) : (
        <div className="rounded-lg border bg-muted/30 p-6 text-center text-sm text-muted-foreground">
          Tidak ada instrumen dengan drift signifikan dalam laporan ini.
        </div>
      )}
    </div>
  );
}

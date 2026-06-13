"use client";

import * as React from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef, SortingState } from "@tanstack/react-table";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { AlertTriangle, Info } from "lucide-react";

import { rollForwardApi } from "@/lib/api/roll-forward.api";
import { usePermissions } from "@/lib/stores/auth.store";
import { ReconcileBadge } from "@/components/blips/ReconcileBadge";
import { RollForwardDetectionMethodBadge } from "@/components/blips/RollForwardDetectionMethodBadge";
import { TransferBucketRow } from "@/components/blips/TransferBucketRow";
import { DataTable } from "@/components/blips/DataTable";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { RollForwardInstrumentLine } from "@/lib/schemas/roll-forward.schema";

// ---------------------------------------------------------------------------
// Formatter
// ---------------------------------------------------------------------------

function formatIDR(value: string | null | undefined): string {
  if (!value) return "—";
  const isNeg = value.startsWith("-");
  const abs = isNeg ? value.slice(1) : value;
  const num = parseFloat(abs);
  if (isNaN(num)) return value;
  const f = new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(num);
  return isNeg ? `−${f}` : f;
}

const STAGE_LABEL: Record<string, string> = {
  STAGE_1: "Stage 1",
  STAGE_2: "Stage 2",
  STAGE_3: "Stage 3",
};

const BUCKET_LABELS: Record<string, string> = {
  stage_1_to_2: "Penurunan/SICR (1→2)",
  stage_2_to_1: "Pemulihan/Cure (2→1)",
  stage_2_to_3: "Default (2→3)",
  stage_1_to_3: "Default Langsung (1→3)",
  stage_3_to_2: "Pemulihan Parsial (3→2)",
  stage_3_to_1: "Pemulihan Penuh (3→1)",
  new_origination: "Originasi Baru",
  derecognition: "Penghapusbukuan",
  stage_same: "Tahap Sama (Remeasurement)",
};

// ---------------------------------------------------------------------------
// Instrument DataTable columns
// ---------------------------------------------------------------------------

const instrumentColumns: ColumnDef<RollForwardInstrumentLine>[] = [
  {
    id: "instrumenKode",
    header: "Kode",
    enableSorting: true,
    cell: ({ row }) => (
      <span className="font-mono text-xs">
        {row.original.instrumenKode ?? row.original.instrumenId.slice(0, 8)}
      </span>
    ),
  },
  {
    id: "instrumenNama",
    header: "Nama Instrumen",
    enableSorting: true,
    cell: ({ row }) => (
      <span className="text-sm">{row.original.instrumenNama ?? "—"}</span>
    ),
  },
  {
    id: "stagePrior",
    header: "Stage Prior",
    cell: ({ row }) => (
      <span className="text-xs">
        {row.original.stagePrior ? STAGE_LABEL[row.original.stagePrior] : "—"}
      </span>
    ),
  },
  {
    id: "stageCurrent",
    header: "Stage Saat Ini",
    cell: ({ row }) => (
      <span className="text-xs">
        {row.original.stageCurrent ? STAGE_LABEL[row.original.stageCurrent] : "—"}
      </span>
    ),
  },
  {
    id: "eclPriorIdr",
    header: "ECL Prior (IDR)",
    enableSorting: true,
    cell: ({ row }) => (
      <span className="font-mono text-xs">{formatIDR(row.original.eclPriorIdr)}</span>
    ),
  },
  {
    id: "eclCurrentIdr",
    header: "ECL Saat Ini (IDR)",
    enableSorting: true,
    cell: ({ row }) => (
      <span className="font-mono text-xs">{formatIDR(row.original.eclCurrentIdr)}</span>
    ),
  },
  {
    id: "eclMovementIdr",
    header: "Pergerakan ECL (IDR)",
    enableSorting: true,
    cell: ({ row }) => {
      const v = row.original.eclMovementIdr;
      const isNeg = v?.startsWith("-");
      return (
        <span
          className={`font-mono text-xs ${isNeg ? "text-green-700" : v && v !== "0.0000" ? "text-red-700" : ""}`}
        >
          {formatIDR(v)}
        </span>
      );
    },
  },
  {
    id: "overrideFlag",
    header: "Override",
    cell: ({ row }) =>
      row.original.overrideFlag ? (
        <Badge
          variant="outline"
          className="text-xs px-1.5 py-0 border-amber-400 text-amber-700 bg-amber-50"
        >
          Override
        </Badge>
      ) : null,
  },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function PortfolioRollForwardPage() {
  const params = useParams<{ reportId: string; portofolioId: string }>();
  const searchParams = useSearchParams();
  const router = useRouter();
  const { can } = usePermissions();

  const { reportId, portofolioId } = params;
  const currentCalcRunId = searchParams.get("currentCalcRunId") ?? "";
  const priorCalcRunId = searchParams.get("priorCalcRunId") ?? null;

  const [selectedBucket, setSelectedBucket] = React.useState<string>("_all");
  const [sorting, setSorting] = React.useState<SortingState>([
    { id: "eclMovementIdr", desc: true },
  ]);
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [searchValue, setSearchValue] = React.useState("");
  const [activeFilters, setActiveFilters] = React.useState<ActiveFilter[]>([]);
  const hasPermission = can("ecl.roll_forward.read");

  // Fetch portfolio roll-forward aggregate
  const {
    data: pfData,
    isLoading: pfLoading,
    isError: pfError,
  } = useQuery({
    queryKey: ["portfolio-roll-forward", portofolioId, currentCalcRunId, priorCalcRunId],
    queryFn: () =>
      rollForwardApi.getPortfolio(portofolioId, {
        currentCalcRunId,
        priorCalcRunId: priorCalcRunId ?? undefined,
      }),
    enabled: !!currentCalcRunId,
  });

  const pf = pfData?.data;

  // Fetch instrument drill-down DataTable
  const sortStr = sorting
    .map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`)
    .join(",");

  const {
    data: instrData,
    isLoading: instrLoading,
    isError: instrError,
    refetch: instrRefetch,
  } = useQuery({
    queryKey: [
      "roll-forward-instruments",
      portofolioId,
      currentCalcRunId,
      priorCalcRunId,
      selectedBucket,
      sortStr,
      cursor,
      searchValue,
    ],
    queryFn: () =>
      rollForwardApi.listPortfolioInstruments(portofolioId, {
        currentCalcRunId,
        priorCalcRunId: priorCalcRunId ?? undefined,
        bucket: selectedBucket === "_all" ? undefined : selectedBucket,
        sort: sortStr || undefined,
        cursor: cursor ?? undefined,
        limit: 50,
        q: searchValue || undefined,
      }),
    enabled: !!currentCalcRunId,
  });

  const instruments = instrData?.data ?? [];
  const pagination = instrData?.pagination;

  const handleBucketChange = (value: string) => {
    setSelectedBucket(value);
    setCursor(null);
    if (value !== "_all") {
      const label = BUCKET_LABELS[value] ?? value;
      setActiveFilters([{ key: "bucket", label: "Bucket", value, displayValue: label }]);
    } else {
      setActiveFilters([]);
    }
  };

  const handleExport = (format: "csv" | "xlsx") => {
    rollForwardApi.exportPortfolioInstruments(portofolioId, {
      currentCalcRunId,
      priorCalcRunId: priorCalcRunId ?? undefined,
      bucket: selectedBucket === "_all" ? undefined : selectedBucket,
      format,
    });
  };

  // Permission guard (after all hooks)
  if (!hasPermission) {
    return (
      <div className="p-6">
        <Alert variant="destructive">
          <AlertTitle>Akses Ditolak</AlertTitle>
          <AlertDescription>
            Anda tidak memiliki izin <code>ecl.roll_forward.read</code>.
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  if (pfLoading) {
    return (
      <div className="p-6 space-y-4 max-w-5xl">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }

  if (pfError || !pf) {
    return (
      <div className="p-6">
        <Alert variant="destructive">
          <AlertTitle>Data Tidak Ditemukan</AlertTitle>
          <AlertDescription>
            Roll-forward untuk portofolio ini tidak tersedia.
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-5xl">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex items-center gap-1 flex-wrap">
          <li>
            <button
              className="hover:underline"
              onClick={() => router.push("/ecl/roll-forward")}
            >
              Roll-Forward CKPN
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li>
            <button
              className="hover:underline"
              onClick={() =>
                router.push(
                  `/ecl/roll-forward/${encodeURIComponent(reportId)}?currentCalcRunId=${currentCalcRunId}${priorCalcRunId ? `&priorCalcRunId=${priorCalcRunId}` : ""}`,
                )
              }
            >
              Laporan
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">
            {pf.portofolioNama ?? portofolioId}
          </li>
        </ol>
      </nav>

      {/* Header */}
      <div className="space-y-1">
        <h1 className="text-xl font-semibold">
          Roll-Forward Portofolio: {pf.portofolioNama ?? portofolioId}
        </h1>
        <div className="flex items-center gap-3 text-sm text-muted-foreground flex-wrap">
          <span>{pf.instrumentCount} instrumen</span>
          <span aria-hidden>·</span>
          <RollForwardDetectionMethodBadge method={pf.detectionMethod} />
        </div>
      </div>

      {/* Phase 5 notice */}
      <Alert className="border-amber-200 bg-amber-50 py-2">
        <Info className="h-4 w-4 text-amber-600" aria-hidden="true" />
        <AlertDescription className="text-amber-700 text-sm">
          Deteksi origination/derecognition menggunakan perubahan status instrumen
          (BASIC_STATUS_DIFF). Presisi penuh tersedia di Phase 5.
        </AlertDescription>
      </Alert>

      {/* Waterfall per portfolio */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">
            Waterfall CKPN — {pf.portofolioNama ?? "Portofolio"}
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead>
                <tr className="border-b">
                  <th className="py-2 px-4 text-left font-medium text-muted-foreground">
                    Komponen
                  </th>
                  <th className="py-2 px-4 text-right font-medium text-muted-foreground">
                    Jumlah (IDR)
                  </th>
                  <th className="py-2 px-4 text-left font-medium text-muted-foreground w-32">
                    Ket
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr className="border-b bg-muted/20 font-semibold">
                  <td className="py-2.5 px-4 text-sm">Saldo Awal CKPN</td>
                  <td className="py-2.5 px-4 text-right font-mono text-xs">
                    {formatIDR(pf.openingEclIdr)}
                  </td>
                  <td className="py-2.5 px-4 text-xs text-muted-foreground">
                    (Opening)
                  </td>
                </tr>

                <TransferBucketRow
                  label="Penurunan/SICR (Stage 1 → 2)"
                  bucket={pf.transfers.stage1To2}
                  sign="+"
                />
                <TransferBucketRow
                  label="Pemulihan/Cure (Stage 2 → 1)"
                  bucket={pf.transfers.stage2To1}
                  sign="-"
                />
                <TransferBucketRow
                  label="Default (Stage 2 → 3)"
                  bucket={pf.transfers.stage2To3}
                  sign="+"
                />
                <TransferBucketRow
                  label="Default Langsung (Stage 1 → 3)"
                  bucket={pf.transfers.stage1To3}
                  sign="+"
                />
                <TransferBucketRow
                  label="Pemulihan Parsial (Stage 3 → 2)"
                  bucket={pf.transfers.stage3To2}
                  sign="-"
                />
                <TransferBucketRow
                  label="Pemulihan Penuh (Stage 3 → 1)"
                  bucket={pf.transfers.stage3To1}
                  sign="-"
                />

                <tr className="border-b hover:bg-muted/20">
                  <td className="py-2.5 px-4 pl-8 text-sm text-red-700">
                    <span className="mr-1 font-mono text-xs">+</span>
                    Originasi Baru
                  </td>
                  <td className="py-2.5 px-4 text-right font-mono text-xs text-red-700">
                    +{formatIDR(pf.newOriginations.eclIdr)}
                  </td>
                  <td className="py-2.5 px-4">
                    <span className="text-xs text-muted-foreground">
                      {pf.newOriginations.count} instr.
                    </span>
                  </td>
                </tr>

                <tr className="border-b hover:bg-muted/20">
                  <td className="py-2.5 px-4 pl-8 text-sm text-green-700">
                    <span className="mr-1 font-mono text-xs">−</span>
                    Penghapusbukuan
                  </td>
                  <td className="py-2.5 px-4 text-right font-mono text-xs text-green-700">
                    −{formatIDR(pf.derecognitions.priorEclIdr)}
                  </td>
                  <td className="py-2.5 px-4">
                    <span className="text-xs text-muted-foreground">
                      {pf.derecognitions.count} instr.
                    </span>
                  </td>
                </tr>

                <tr className="border-b hover:bg-muted/20">
                  <td className="py-2.5 px-4 pl-8 text-sm text-muted-foreground">
                    <span className="mr-1 font-mono text-xs">±</span>
                    Pengukuran Ulang (Residual)
                  </td>
                  <td className="py-2.5 px-4 text-right font-mono text-xs">
                    {formatIDR(pf.remeasurementsIdr)}
                  </td>
                  <td className="py-2.5 px-4" />
                </tr>

                <tr className="border-t-2 bg-muted/40 font-bold">
                  <td className="py-3 px-4 text-sm">= Saldo Akhir CKPN</td>
                  <td className="py-3 px-4 text-right font-mono">
                    {formatIDR(pf.closingEclIdr)}
                  </td>
                  <td className="py-3 px-4" />
                </tr>
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {/* Data quality warnings (portfolio) */}
      {pf.dataQualityWarnings && pf.dataQualityWarnings.length > 0 && (
        <Alert className="border-amber-200 bg-amber-50">
          <AlertTriangle className="h-4 w-4 text-amber-600" aria-hidden="true" />
          <AlertTitle className="text-amber-800">
            Peringatan Kualitas Data ({pf.dataQualityWarnings.length})
          </AlertTitle>
          <AlertDescription className="text-amber-700">
            <ul className="mt-1 space-y-1">
              {pf.dataQualityWarnings.slice(0, 3).map((w, i) => (
                <li key={i} className="text-xs">
                  <span className="font-mono mr-1">{w.warningCode}</span>
                  {w.instrumenKode && <span className="font-medium">[{w.instrumenKode}]</span>}{" "}
                  {w.message}
                </li>
              ))}
              {pf.dataQualityWarnings.length > 3 && (
                <li className="text-xs text-muted-foreground">
                  ... dan {pf.dataQualityWarnings.length - 3} lainnya.
                </li>
              )}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      {/* Instrument drill-down DataTable */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Instrumen per Bucket</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {/* Bucket filter */}
          <div className="px-4 py-3 border-b flex items-center gap-3 flex-wrap">
            <label className="text-sm font-medium text-muted-foreground">
              Filter Bucket:
            </label>
            <Select value={selectedBucket} onValueChange={handleBucketChange}>
              <SelectTrigger className="w-64" aria-label="Pilih bucket transfer">
                <SelectValue placeholder="Semua bucket..." />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="_all">Semua Instrumen</SelectItem>
                {Object.entries(BUCKET_LABELS).map(([key, label]) => (
                  <SelectItem key={key} value={key}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <DataTable
            columns={instrumentColumns}
            data={instruments}
            pagination={pagination}
            isLoading={instrLoading}
            isError={instrError}
            sorting={sorting}
            onSortingChange={setSorting}
            searchValue={searchValue}
            onSearchChange={(v) => {
              setSearchValue(v);
              setCursor(null);
            }}
            searchPlaceholder="Cari kode / nama instrumen..."
            activeFilters={activeFilters}
            onRemoveFilter={() => {
              setActiveFilters([]);
              setSelectedBucket("_all");
            }}
            onClearFilters={() => {
              setActiveFilters([]);
              setSelectedBucket("_all");
            }}
            onNextPage={() => setCursor(pagination?.nextCursor ?? null)}
            onPrevPage={() => setCursor(null)}
            canPrevPage={!!cursor}
            onExport={handleExport}
            onRefresh={() => void instrRefetch()}
            emptyMessage={
              selectedBucket !== "_all"
                ? `Tidak ada instrumen di bucket "${BUCKET_LABELS[selectedBucket] ?? selectedBucket}"`
                : "Tidak ada instrumen dalam portofolio ini"
            }
          />
        </CardContent>
      </Card>

      {/* Back link */}
      <Button
        variant="outline"
        onClick={() =>
          router.push(
            `/ecl/roll-forward/${encodeURIComponent(reportId)}?currentCalcRunId=${currentCalcRunId}${priorCalcRunId ? `&priorCalcRunId=${priorCalcRunId}` : ""}`,
          )
        }
      >
        Kembali ke Laporan
      </Button>
    </div>
  );
}

"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ShieldAlert, Plus } from "lucide-react";
import { v4 as uuidv4 } from "uuid";

import { stagingApi } from "@/lib/api/staging.api";
import type { StageHistoryRow } from "@/lib/schemas/staging.schema";
import { StageBadge } from "@/components/blips/StageBadge";
import { SicrEvidenceCard } from "@/components/blips/SicrEvidenceCard";
import type { SicrTriggerType } from "@/components/blips/SicrEvidenceCard";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { DataTable } from "@/components/blips/DataTable";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import { useStagingStore } from "@/lib/stores/staging.store";

// ---------------------------------------------------------------------------
// Evidence expand cell
// ---------------------------------------------------------------------------

function EvidenceCell({ row }: { row: StageHistoryRow }) {
  const ev = row.sicrEvidence;
  if (!ev) return <span className="text-xs text-muted-foreground">—</span>;

  // Map sicrEvidence to SicrEvidenceCard props
  const triggerType = (row.triggerType as SicrTriggerType) ?? "ORIGINATION";

  return (
    <SicrEvidenceCard
      triggerType={triggerType}
      evidence={{
        notchChange: ev.notchDelta ?? null,
        ratingLama: ev.ratingBaseline ?? null,
        ratingBaru: ev.ratingCurrent ?? null,
        dpd: ev.dpdValue ?? null,
        curiePeriode: ev.cureConsecutivePeriodes ?? null,
      }}
      compact
    />
  );
}

// ---------------------------------------------------------------------------
// Columns
// ---------------------------------------------------------------------------

function buildColumns(): ColumnDef<StageHistoryRow>[] {
  return [
    {
      id: "tanggalMigrasi",
      accessorKey: "tanggalMigrasi",
      header: "Tanggal",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-xs">{row.original.tanggalMigrasi}</span>
      ),
    },
    {
      id: "stageSebelum",
      accessorKey: "stageSebelum",
      header: "Sebelum",
      cell: ({ row }) => {
        const s = row.original.stageSebelum;
        if (!s) return <span className="text-xs text-muted-foreground">—</span>;
        const num = parseInt(s.replace("STAGE_", ""), 10) as 1 | 2 | 3;
        return <StageBadge stage={num} size="sm" />;
      },
    },
    {
      id: "stageSesudah",
      accessorKey: "stageSesudah",
      header: "Sesudah",
      cell: ({ row }) => {
        const s = row.original.stageSesudah;
        const num = parseInt(s.replace("STAGE_", ""), 10) as 1 | 2 | 3;
        return <StageBadge stage={num} size="sm" />;
      },
    },
    {
      id: "triggerType",
      accessorKey: "triggerType",
      header: "Trigger",
      cell: ({ row }) => (
        <span className="text-xs font-mono">{row.original.triggerType}</span>
      ),
    },
    {
      id: "evidence",
      header: "Evidence",
      cell: ({ row }) => <EvidenceCell row={row.original} />,
    },
    {
      id: "userApproverNama",
      accessorKey: "userApproverNama",
      header: "Disetujui Oleh",
      cell: ({ row }) => (
        <span className="text-xs">
          {row.original.userApproverNama ?? (row.original.statusApproval === "AUTO" ? "Auto" : "—")}
        </span>
      ),
    },
  ];
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function StagingInstrumenPage() {
  const params = useParams();
  const instrumenId = params.id as string;
  const router = useRouter();
  const queryClient = useQueryClient();
  const { can, userId } = usePermissions();
  const { setActiveJobId, activeJobId } = useStagingStore();

  // Filters
  const [triggerFilter, setTriggerFilter] = React.useState("");
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [pageNumber, setPageNumber] = React.useState(1);
  const [prevCursors, setPrevCursors] = React.useState<string[]>([]);

  // Current staging
  const { data: currentData, isLoading: loadingCurrent } = useQuery({
    queryKey: ["staging-current", instrumenId],
    queryFn: () => stagingApi.getCurrent(instrumenId),
    enabled: !!instrumenId,
  });

  const current = currentData?.data;
  const currentStageNum = current?.currentStage
    ? (parseInt(current.currentStage.replace("STAGE_", ""), 10) as 1 | 2 | 3)
    : null;

  // History
  const historyParams = {
    limit: 50,
    sort: "tanggal_migrasi:desc",
    ...(triggerFilter && { "filter[trigger_type]": triggerFilter }),
    ...(cursor && { cursor }),
  };

  const { data: historyData, isLoading: loadingHistory, isError, refetch } = useQuery({
    queryKey: ["staging-history", instrumenId, historyParams],
    queryFn: () => stagingApi.listHistory(instrumenId, historyParams),
    enabled: !!instrumenId,
  });

  // Evaluate mutation
  const evaluateMutation = useMutation({
    mutationFn: () =>
      stagingApi.evaluate(
        { instrumenIds: [instrumenId], triggerType: "ALL" },
        uuidv4(),
      ),
    onSuccess: (res) => {
      setActiveJobId(res.data.jobId);
      notify.info("Evaluasi staging dimulai...");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  // Active filters for chips
  const activeFilters: ActiveFilter[] = [];
  if (triggerFilter) {
    activeFilters.push({
      key: "trigger_type",
      label: "Trigger",
      value: triggerFilter,
      displayValue: triggerFilter,
    });
  }

  const handleExport = (format: "csv" | "xlsx") => {
    const url = stagingApi.historyExportUrl(instrumenId, { ...historyParams, export: format });
    window.open(url, "_blank");
  };

  const handleNextPage = () => {
    const next = historyData?.pagination?.nextCursor ?? null;
    if (next) {
      setPrevCursors((p) => [...p, cursor ?? ""]);
      setCursor(next);
      setPageNumber((n) => n + 1);
    }
  };

  const handlePrevPage = () => {
    const prev = prevCursors[prevCursors.length - 1] ?? null;
    setPrevCursors((p) => p.slice(0, -1));
    setCursor(prev);
    setPageNumber((n) => Math.max(1, n - 1));
  };

  const columns = buildColumns();

  // Stage 3 SICR evidence card
  const lastEvidence =
    historyData?.data?.[0]?.sicrEvidence ?? null;
  const lastTriggerType = current?.lastTriggerType as SicrTriggerType | undefined;

  return (
    <div className="space-y-4 p-6">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex gap-1">
          <li>ECL</li>
          <li aria-hidden>&rsaquo;</li>
          <li>Staging</li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">{current?.kodeInstrumen ?? instrumenId}</li>
        </ol>
      </nav>

      {/* Instrumen header card */}
      {current && (
        <Card>
          <CardContent className="pt-4">
            <div className="flex flex-wrap gap-4 items-start">
              <div>
                <p className="font-semibold">{current.kodeInstrumen}</p>
                <p className="text-sm text-muted-foreground">{current.namaInstrumen}</p>
              </div>
              <div className="flex gap-2 ml-auto">
                <span className="text-xs text-muted-foreground">
                  {current.klasifikasiPsak71}
                </span>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Stage banner */}
      {loadingCurrent ? (
        <div className="h-16 rounded-lg bg-muted animate-pulse" />
      ) : current ? (
        <div className="rounded-lg p-4">
          <StageBadge stage={currentStageNum} size="lg" />
          {current.lastTransitionDate && (
            <p className="text-sm text-muted-foreground mt-1">
              Evaluasi terakhir: {current.lastTransitionDate}
            </p>
          )}
          {current.lastTriggerDetail && (
            <p className="text-sm mt-1">{current.lastTriggerDetail}</p>
          )}
        </div>
      ) : (
        <div className="rounded-lg p-4 bg-gray-100">
          <p className="text-sm font-medium">Stage belum dievaluasi</p>
        </div>
      )}

      {/* Stage 3 alert */}
      {currentStageNum === 3 && (
        <Alert variant="destructive">
          <ShieldAlert className="h-4 w-4" aria-hidden="true" />
          <AlertDescription>
            Instrumen ini dalam status default (Stage 3). PD = 1.0 digunakan
            untuk ECL. Bunga dihitung dari Net Carrying Amount (Gross − ECL).
          </AlertDescription>
        </Alert>
      )}

      {/* SICR evidence card */}
      {lastTriggerType && (
        <SicrEvidenceCard
          triggerType={(lastTriggerType as unknown as SicrTriggerType) ?? "ORIGINATION"}
          evidence={
            lastEvidence
              ? {
                  notchChange: lastEvidence.notchDelta ?? null,
                  ratingLama: lastEvidence.ratingBaseline ?? null,
                  ratingBaru: lastEvidence.ratingCurrent ?? null,
                  dpd: lastEvidence.dpdValue ?? null,
                }
              : null
          }
          date={current?.lastTransitionDate ?? undefined}
        />
      )}

      {/* Job progress panel */}
      {activeJobId && (
        <JobProgressPanel
          jobId={activeJobId}
          title="Mengevaluasi staging..."
          variant="inline"
          onComplete={() => {
            setActiveJobId(null);
            void queryClient.invalidateQueries({ queryKey: ["staging-current", instrumenId] });
            void queryClient.invalidateQueries({ queryKey: ["staging-history", instrumenId] });
            notify.success("Evaluasi staging selesai.");
          }}
          onFail={() => {
            setActiveJobId(null);
            notify.error({
              code: "STAGING_EVAL_FAILED",
              message: "Evaluasi staging gagal.",
              traceId: "",
            });
          }}
        />
      )}

      {/* Action bar */}
      <div className="flex gap-2">
        {can("ecl_staging.evaluate") && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => evaluateMutation.mutate()}
            disabled={evaluateMutation.isPending || !!activeJobId}
          >
            Evaluasi Staging
          </Button>
        )}
        {can("ecl_staging.override.propose") && (
          <Button
            size="sm"
            onClick={() =>
              router.push(
                `/ecl/staging/override/new?instrumenId=${instrumenId}`,
              )
            }
          >
            <Plus className="h-4 w-4 mr-1" aria-hidden="true" />
            Request Override
          </Button>
        )}
      </div>

      {/* History table */}
      <DataTable
        columns={columns}
        data={historyData?.data ?? []}
        pagination={historyData?.pagination}
        isLoading={loadingHistory}
        isError={isError}
        activeFilters={activeFilters}
        onRemoveFilter={(key) => {
          if (key === "trigger_type") setTriggerFilter("");
        }}
        onClearFilters={() => setTriggerFilter("")}
        onExport={handleExport}
        onRefresh={() => void refetch()}
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageNumber > 1}
        pageNumber={pageNumber}
        emptyMessage="Belum ada riwayat staging. Jalankan evaluasi pertama."
      />
    </div>
  );
}

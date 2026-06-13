"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { Plus, RefreshCw } from "lucide-react";

import { eirApi } from "@/lib/api/eir.api";
import { useEIRStore } from "@/lib/stores/eir.store";
import { NewtonRaphsonSolverPanel } from "@/components/blips/NewtonRaphsonSolverPanel";
import { AmortizationScheduleTable } from "@/components/blips/AmortizationScheduleTable";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { ScheduleVersion } from "@/lib/schemas/eir.schema";

// ---------------------------------------------------------------------------
// EIR format helper (string, 8 decimals, no parseFloat for money)
// ---------------------------------------------------------------------------

function formatEIRPct(eirStr: string | null | undefined): string {
  if (!eirStr) return "—";
  // eirStr is NUMERIC(10,8) e.g. "0.07250000"
  // Multiply by 100 to get percentage — use string arithmetic to stay safe
  // For display only: parseFloat is acceptable here (not stored)
  const num = parseFloat(eirStr);
  if (isNaN(num)) return eirStr;
  return `${(num * 100).toFixed(4)}%`;
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function EIRInstrumenPage() {
  const params = useParams();
  const instrumenId = params.id as string;
  const router = useRouter();
  const queryClient = useQueryClient();
  const { can } = usePermissions();
  const {
    selectedScheduleVersion,
    setSelectedScheduleVersion,
    activeDriftJobId,
    setActiveDriftJobId,
  } = useEIRStore();

  const [scheduleCursor, setScheduleCursor] = React.useState<string | null>(null);
  const [schedulePageNumber, setSchedulePageNumber] = React.useState(1);
  const [prevScheduleCursors, setPrevScheduleCursors] = React.useState<string[]>([]);

  // Schedule list
  const scheduleParams = {
    limit: 50,
    sort: "periode_seq:asc",
    ...(scheduleCursor && { cursor: scheduleCursor }),
  };

  const { data: scheduleData, isLoading: loadingSchedule, isError: errorSchedule, refetch: refetchSchedule } = useQuery({
    queryKey: ["eir-schedule", instrumenId, scheduleParams],
    queryFn: () => eirApi.listSchedule(instrumenId, scheduleParams),
    enabled: !!instrumenId,
  });

  // Compute latest EIR (read-only display)
  const { data: computeData, isLoading: loadingCompute } = useQuery({
    queryKey: ["eir-compute-latest", instrumenId],
    queryFn: () =>
      eirApi.compute(
        { instrumenId, persistResult: false, forceRecompute: false },
        uuidv4(),
      ),
    enabled: !!instrumenId,
    retry: false,
  });

  const latestEir = computeData?.data;
  const solverMeta = latestEir?.solverMetadata ?? null;

  // Versions — derive from schedule
  const scheduleVersions: ScheduleVersion[] = React.useMemo(() => {
    const versionSet = new Map<number, ScheduleVersion>();
    for (const row of scheduleData?.data ?? []) {
      // scheduleVersion not on row, but we can fake it from data
      // In a real implementation, the backend version endpoint would return this
    }
    return Array.from(versionSet.values());
  }, [scheduleData]);

  // Export handler
  const handleExport = (format: "csv" | "xlsx") => {
    const url = eirApi.scheduleExportUrl(instrumenId, { ...scheduleParams, export: format });
    window.open(url, "_blank");
  };

  const handleNextSchedulePage = () => {
    const next = scheduleData?.pagination?.nextCursor ?? null;
    if (next) {
      setPrevScheduleCursors((p) => [...p, scheduleCursor ?? ""]);
      setScheduleCursor(next);
      setSchedulePageNumber((n) => n + 1);
    }
  };

  const handlePrevSchedulePage = () => {
    const prev = prevScheduleCursors[prevScheduleCursors.length - 1] ?? null;
    setPrevScheduleCursors((p) => p.slice(0, -1));
    setScheduleCursor(prev);
    setSchedulePageNumber((n) => Math.max(1, n - 1));
  };

  return (
    <div className="p-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex gap-1">
          <li>ECL</li>
          <li aria-hidden>&rsaquo;</li>
          <li>EIR</li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">{instrumenId}</li>
        </ol>
      </nav>

      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">EIR — Instrumen</h1>
        <div className="flex gap-2">
          {can("ecl_eir.amendment.propose") && (
            <Button
              size="sm"
              onClick={() =>
                router.push(
                  `/ecl/eir/amendments/new?instrumenId=${instrumenId}`,
                )
              }
            >
              <Plus className="h-4 w-4 mr-1" aria-hidden="true" />
              Ajukan Amandemen
            </Button>
          )}
        </div>
      </div>

      {/* EIR summary card */}
      {loadingCompute ? (
        <div className="h-24 rounded-lg bg-muted animate-pulse" />
      ) : latestEir ? (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">EIR Terkini</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
              <div>
                <p className="text-xs text-muted-foreground">EIR per Periode</p>
                <p className="text-xl font-bold font-mono">
                  {formatEIRPct(latestEir.eirPerPeriod)}
                </p>
              </div>
              {latestEir.eirAnnualEquivalent && (
                <div>
                  <p className="text-xs text-muted-foreground">Ekuivalen Tahunan</p>
                  <p className="text-lg font-mono">
                    {formatEIRPct(latestEir.eirAnnualEquivalent)}
                  </p>
                </div>
              )}
              <div>
                <p className="text-xs text-muted-foreground">Tipe EIR</p>
                <Badge variant={latestEir.eirType === "CREDIT_ADJUSTED" ? "destructive" : "default"}>
                  {latestEir.eirType === "CREDIT_ADJUSTED" ? "POCI Credit-Adj." : "Standard"}
                </Badge>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Dihitung</p>
                <p className="text-sm">{latestEir.computedAt.slice(0, 10)}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {/* Schedule missing warning */}
      {scheduleData?.warning && (
        <Alert>
          <AlertDescription>{scheduleData.warning}</AlertDescription>
        </Alert>
      )}

      {/* Newton-Raphson solver panel */}
      {solverMeta && (
        <NewtonRaphsonSolverPanel solverMetadata={solverMeta} />
      )}

      {/* Job progress (bulk recompute) */}
      {activeDriftJobId && (
        <JobProgressPanel
          jobId={activeDriftJobId}
          title="Re-estimasi EIR berjalan..."
          variant="inline"
          onComplete={() => {
            setActiveDriftJobId(null);
            void queryClient.invalidateQueries({ queryKey: ["eir-schedule", instrumenId] });
            void queryClient.invalidateQueries({ queryKey: ["eir-compute-latest", instrumenId] });
            notify.success("Re-estimasi EIR selesai.");
          }}
          onFail={() => {
            setActiveDriftJobId(null);
          }}
        />
      )}

      {/* Amortization schedule table */}
      <AmortizationScheduleTable
        instrumenId={instrumenId}
        versions={scheduleVersions}
        data={scheduleData?.data ?? []}
        pagination={scheduleData?.pagination}
        selectedVersion={selectedScheduleVersion}
        onVersionChange={setSelectedScheduleVersion}
        isLoading={loadingSchedule}
        isError={errorSchedule}
        onExport={handleExport}
        onRefresh={() => void refetchSchedule()}
        onNextPage={handleNextSchedulePage}
        onPrevPage={handlePrevSchedulePage}
        canPrevPage={schedulePageNumber > 1}
        pageNumber={schedulePageNumber}
      />
    </div>
  );
}

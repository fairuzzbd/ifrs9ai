"use client";

/**
 * P5-M15 — Risk Dashboard client component.
 * Widgets: W-RK-01..05.
 */

import * as React from "react";
import { Shield } from "lucide-react";
import { type ColumnDef } from "@tanstack/react-table";
import { useQuery } from "@tanstack/react-query";
import { useReportData } from "@/lib/hooks/useReportData";
import { parseDecimal, formatIDRAbbrev, formatIDR } from "@/lib/format";
import { DashboardShell, GridCol } from "@/components/blips/dashboard/DashboardShell";
import { WidgetCard, WidgetEmpty } from "@/components/blips/dashboard/WidgetCard";
import { KPICard } from "@/components/blips/dashboard/KPICard";
import { StageDistributionDonut } from "@/components/blips/dashboard/StageDistributionDonut";
import { StageMovementBar } from "@/components/blips/dashboard/StageMovementBar";
import { RecentTransactionsList } from "@/components/blips/dashboard/RecentTransactionsList";
import { JobStatusList } from "@/components/blips/dashboard/JobStatusList";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { Badge } from "@/components/ui/badge";
import { jobsListApi } from "@/lib/api/reports.api";
import { notify } from "@/lib/notify";
import { useRouter } from "next/navigation";
import type { Rpt13Row, Rpt14Row, Rpt15Row } from "@/lib/schemas/dashboard.schema";

interface RiskDashboardClientProps {
  username: string;
  permissions: string[];
  userId: string;
}

const CURRENT_PERIODE = new Date().toISOString().slice(0, 7).replace("-", "");

export function RiskDashboardClient({ username, userId }: RiskDashboardClientProps) {
  const router = useRouter();

  // Latest calc run job query (check for running ECL job)
  const activeJobQuery = useQuery({
    queryKey: ["dashboard", "ecl-active-job"],
    queryFn: () =>
      jobsListApi.list({
        "filter[type]": "ECL_CALC_RUN",
        "filter[status]": "running,queued",
        limit: 1,
        sort: "created_at:desc",
      }),
    refetchInterval: 300_000,
  });

  const lastJobQuery = useQuery({
    queryKey: ["dashboard", "ecl-last-job"],
    queryFn: () =>
      jobsListApi.list({
        "filter[type]": "ECL_CALC_RUN",
        limit: 1,
        sort: "created_at:desc",
      }),
    refetchInterval: 300_000,
  });

  const activeJob = activeJobQuery.data?.data[0];
  const lastJob = lastJobQuery.data?.data[0];
  const latestCalcRunId = lastJob?.result != null
    ? (lastJob.result as { calcRunId?: string }).calcRunId
    : undefined;

  // Report data
  const eclDetail = useReportData("rpt-13", {
    ...(latestCalcRunId ? { "filter[calc_run_id]": latestCalcRunId } : {}),
    sort: "stage:asc",
    limit: 200,
  });

  const eclTop10 = useReportData("rpt-13", {
    ...(latestCalcRunId ? { "filter[calc_run_id]": latestCalcRunId } : {}),
    sort: "ecl_weighted:desc",
    limit: 10,
  });

  const stageMovement = useReportData("rpt-14", {
    sort: "tanggal_transisi:asc",
    limit: 200,
  });

  const sicrTriggers = useReportData("rpt-15", {
    sort: "tanggal_trigger:desc",
    limit: 50,
  });

  // Compute stage distribution
  const eclRows: Rpt13Row[] = eclDetail.data?.data ?? [];
  const stageGroups = eclRows.reduce<Record<number, { count: number; eclTotal: number }>>(
    (acc, r) => {
      const s = r.stage;
      if (!acc[s]) acc[s] = { count: 0, eclTotal: 0 };
      acc[s].count += 1;
      acc[s].eclTotal += parseDecimal(r.ecl_weighted);
      return acc;
    },
    {},
  );
  const totalInstrumen = eclRows.length;
  const stageData = [1, 2, 3].map((s) => ({
    stage: s as 1 | 2 | 3,
    count: stageGroups[s]?.count ?? 0,
    eclTotal: stageGroups[s]?.eclTotal ?? 0,
  }));

  // SICR counters
  const sicrRows: Rpt15Row[] = sicrTriggers.data?.data ?? [];
  const sicrByType = sicrRows.reduce<Record<string, number>>((acc, r) => {
    acc[r.trigger_type] = (acc[r.trigger_type] ?? 0) + 1;
    return acc;
  }, {});
  const totalEcl = eclRows.reduce((s, r) => s + parseDecimal(r.ecl_weighted), 0);

  // Stage movement trend
  const movementRows: Rpt14Row[] = stageMovement.data?.data ?? [];
  // Aggregate by periode
  const movementByPeriode = movementRows.reduce<Record<string, { s1Count: number; s2Count: number; s3Count: number }>>(
    (acc, r) => {
      const key = r.periode_label ?? r.periode_id;
      if (!acc[key]) acc[key] = { s1Count: 0, s2Count: 0, s3Count: 0 };
      if (r.stage_to === 1) acc[key].s1Count += r.count_instrumen;
      else if (r.stage_to === 2) acc[key].s2Count += r.count_instrumen;
      else if (r.stage_to === 3) acc[key].s3Count += r.count_instrumen;
      return acc;
    },
    {},
  );
  const movementChartData = Object.entries(movementByPeriode)
    .slice(-6)
    .map(([periode, v]) => ({ periode, ...v }));

  // Top-10 ECL columns
  const top10Columns: ColumnDef<Rpt13Row>[] = [
    {
      accessorKey: "kode_instrumen",
      header: "Kode",
      cell: ({ getValue, row }) => (
        <a
          href={`/reports/rpt-13?filter[instrumen_id]=${row.original.instrumen_id}`}
          className="font-mono text-primary hover:underline"
        >
          {String(getValue())}
        </a>
      ),
    },
    { accessorKey: "nama", header: "Nama", cell: ({ getValue }) => <span className="truncate max-w-24 block">{String(getValue() ?? "—")}</span> },
    {
      accessorKey: "stage",
      header: "Stage",
      cell: ({ getValue }) => {
        const s = Number(getValue());
        const colors: Record<number, string> = { 1: "bg-green-100 text-green-800", 2: "bg-amber-100 text-amber-800", 3: "bg-red-100 text-red-800" };
        return <Badge className={`text-xs ${colors[s] ?? ""}`}>Stage {s}</Badge>;
      },
    },
    { accessorKey: "ead_idr", header: "EAD IDR", cell: ({ getValue }) => <span className="tabular-nums">{formatIDRAbbrev(parseDecimal(String(getValue())))}</span> },
    { accessorKey: "ecl_weighted", header: "ECL Weighted", cell: ({ getValue }) => <span className="tabular-nums">{formatIDRAbbrev(parseDecimal(String(getValue())))}</span> },
    {
      accessorKey: "fl_multiplier",
      header: "FL Multiplier",
      cell: ({ getValue }) => <span className="tabular-nums">{getValue() ? parseDecimal(String(getValue())).toFixed(2) : "—"}</span>,
    },
  ];

  return (
    <DashboardShell title="Risk Dashboard" subtitle={`ROLE-RISK · ${username}`} icon={Shield} dashboardLabel="Risk">
      {/* ROW 1: KPI Cards */}
      <GridCol span={3}>
        <KPICard
          title="Total ECL Weighted"
          value={formatIDRAbbrev(totalEcl)}
          valueAriaLabel={`${formatIDR(totalEcl)} total ECL weighted`}
          loading={eclDetail.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Stage 2 (SICR)"
          value={`${stageData[1].count.toLocaleString("id-ID")} instrumen`}
          valueAriaLabel={`${stageData[1].count} instrumen Stage 2`}
          status={stageData[1].count > 0 ? "warning" : "default"}
          loading={eclDetail.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Stage 3 (Default)"
          value={`${stageData[2].count.toLocaleString("id-ID")} instrumen`}
          valueAriaLabel={`${stageData[2].count} instrumen Stage 3`}
          status={stageData[2].count > 0 ? "danger" : "success"}
          loading={eclDetail.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Pemicu SICR Periode Ini"
          value={`${sicrRows.length} trigger`}
          valueAriaLabel={`${sicrRows.length} pemicu SICR dalam periode ini`}
          status={sicrRows.length > 0 ? "warning" : "default"}
          loading={sicrTriggers.isLoading}
        />
      </GridCol>

      {/* ROW 2: Stage Donut + Stage Movement */}
      <GridCol span={5}>
        <WidgetCard
          title="Distribusi Stage ECL"
          tooltip="Distribusi jumlah instrumen per stage ECL pada calc run terkini"
          dashboardLabel="Risk"
          isLoading={eclDetail.isLoading}
          isError={eclDetail.isError}
          onRetry={eclDetail.refetch}
        >
          {totalInstrumen === 0 ? (
            <WidgetEmpty
              message="Belum ada data ECL calc run untuk periode ini."
              ctaLabel="Jalankan ECL Calc Run →"
              ctaHref="/ecl/calc-runs"
            />
          ) : (
            <StageDistributionDonut
              data={stageData}
              totalCount={totalInstrumen}
              ariaLabel="Distribusi Stage ECL — BLIPS Risk Dashboard"
            />
          )}
        </WidgetCard>
      </GridCol>
      <GridCol span={7}>
        <WidgetCard
          title="Tren Perpindahan Stage ECL"
          tooltip="Jumlah instrumen per stage dalam 6 periode terakhir"
          dashboardLabel="Risk"
          isLoading={stageMovement.isLoading}
          isError={stageMovement.isError}
          onRetry={stageMovement.refetch}
          headerAction={
            <a href="/reports/rpt-14" className="text-xs text-primary hover:underline">
              Lihat RPT-14 →
            </a>
          }
        >
          <StageMovementBar
            data={movementChartData}
            ariaLabel="Tren Perpindahan Stage ECL — 6 periode terakhir"
          />
        </WidgetCard>
      </GridCol>

      {/* ROW 3: SICR Triggers + Calc-Run Status */}
      <GridCol span={4}>
        <WidgetCard
          title="Pemicu SICR Periode Ini"
          tooltip="Jumlah kejadian SICR trigger sejak awal periode berjalan"
          dashboardLabel="Risk"
          isLoading={sicrTriggers.isLoading}
          isError={sicrTriggers.isError}
          onRetry={sicrTriggers.refetch}
          headerAction={
            <a href="/reports/rpt-15" className="text-xs text-primary hover:underline">
              Lihat RPT-15 →
            </a>
          }
        >
          <div className="space-y-2">
            <KPICard
              title="Rating Downgrade ≥2 notch"
              value={String(sicrByType.rating_downgrade ?? 0)}
              valueAriaLabel={`${sicrByType.rating_downgrade ?? 0} rating downgrade`}
              status={(sicrByType.rating_downgrade ?? 0) > 0 ? "warning" : "default"}
            />
            <KPICard
              title="IG → Non-IG"
              value={String(sicrByType.ig_to_nonig ?? 0)}
              valueAriaLabel={`${sicrByType.ig_to_nonig ?? 0} perubahan IG ke non-IG`}
              status={(sicrByType.ig_to_nonig ?? 0) > 0 ? "warning" : "default"}
            />
            <KPICard
              title="DPD ≥ 30 hari"
              value={String(sicrByType.dpd_30 ?? 0)}
              valueAriaLabel={`${sicrByType.dpd_30 ?? 0} DPD 30 hari`}
              status={(sicrByType.dpd_30 ?? 0) > 0 ? "danger" : "default"}
            />
          </div>
        </WidgetCard>
      </GridCol>
      <GridCol span={8}>
        <WidgetCard
          title="Status Calc Run Terakhir"
          tooltip="Progress atau hasil ECL calculation run"
          dashboardLabel="Risk"
          isLoading={activeJobQuery.isLoading}
        >
          {activeJob ? (
            <JobProgressPanel
              jobId={activeJob.jobId}
              title={`ECL Calc Run — ${activeJob.jobId}`}
              showCancel={false}
              showBackground={true}
              onComplete={(result) => {
                const r = result as { calcRunId?: string; totalECL?: string } | null;
                notify.success(
                  `ECL Calc Run ${r?.calcRunId ?? activeJob.jobId} selesai. Total ECL weighted: ${r?.totalECL ? formatIDRAbbrev(parseDecimal(r.totalECL)) : "—"}.`,
                  { action: { label: "Lihat detail →", onClick: () => router.push(`/ecl/calc-runs/${r?.calcRunId ?? ""}`) } },
                );
              }}
              onFail={(err) => {
                notify.error({ code: "INTERNAL", message: String(err), traceId: "" });
              }}
            />
          ) : lastJob ? (
            <KPICard
              title="Last Calc Run"
              value={lastJob.status === "completed" ? "COMPLETED" : lastJob.status.toUpperCase()}
              subLabel={`${lastJob.jobId} — ${lastJob.completedAt?.slice(0, 16) ?? "—"} — ${(lastJob.result as { totalInstruments?: number })?.totalInstruments?.toLocaleString("id-ID") ?? "—"} instrumen`}
              status={lastJob.status === "completed" ? "success" : lastJob.status === "failed" ? "danger" : "default"}
            />
          ) : (
            <WidgetEmpty message="Belum ada calc run yang dijalankan." ctaLabel="Jalankan ECL Calc Run →" ctaHref="/ecl/calc-runs" />
          )}
        </WidgetCard>
      </GridCol>

      {/* ROW 4: Top-10 by ECL */}
      <GridCol span={12}>
        <WidgetCard
          title="Top-10 Instrumen by ECL Weighted"
          tooltip="10 instrumen dengan ECL weighted tertinggi"
          dashboardLabel="Risk"
          isLoading={eclTop10.isLoading}
          isError={eclTop10.isError}
          onRetry={eclTop10.refetch}
          headerAction={
            <a href="/reports/rpt-13" className="text-xs text-primary hover:underline">
              Lihat semua →
            </a>
          }
        >
          <RecentTransactionsList
            data={eclTop10.data?.data ?? []}
            columns={top10Columns}
            maxRows={10}
            emptyMessage="Belum ada hasil ECL yang tersedia."
            ariaLabel="Top-10 Instrumen by ECL Weighted — BLIPS Risk Dashboard"
          />
        </WidgetCard>
      </GridCol>

      {/* Active jobs */}
      <GridCol span={12}>
        <JobStatusList onViewAll={() => router.push("/jobs")} />
      </GridCol>
    </DashboardShell>
  );
}

"use client";

/**
 * P5-M15 — CFO Dashboard client component.
 * Widgets: W-CF-01..06
 */

import * as React from "react";
import { TrendingUp, Lock, AlertTriangle } from "lucide-react";
import { type ColumnDef } from "@tanstack/react-table";
import { useReportData } from "@/lib/hooks/useReportData";
import { parseDecimal, formatIDRAbbrev, formatIDR, formatDate } from "@/lib/format";
import { DashboardShell, GridCol } from "@/components/blips/dashboard/DashboardShell";
import { WidgetCard, WidgetEmpty } from "@/components/blips/dashboard/WidgetCard";
import { KPICard } from "@/components/blips/dashboard/KPICard";
import { ECLRollForwardLine } from "@/components/blips/dashboard/ECLRollForwardLine";
import { ScenarioSensitivityBar } from "@/components/blips/dashboard/ScenarioSensitivityBar";
import { PeriodeBukuTimeline } from "@/components/blips/dashboard/PeriodeBukuTimeline";
import { StageDistributionDonut } from "@/components/blips/dashboard/StageDistributionDonut";
import { RecentTransactionsList } from "@/components/blips/dashboard/RecentTransactionsList";
import { JobStatusList } from "@/components/blips/dashboard/JobStatusList";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useRouter } from "next/navigation";
import Link from "next/link";
import type { Rpt13Row, Rpt18Row, Rpt23Row, Rpt27Row } from "@/lib/schemas/dashboard.schema";

interface CfoDashboardClientProps {
  username: string;
  permissions: string[];
  userId: string;
  canHardClose: boolean;
  canSealRun: boolean;
  canApproveParam: boolean;
}

export function CfoDashboardClient({
  username,
  canHardClose,
  canSealRun,
}: CfoDashboardClientProps) {
  const router = useRouter();

  // RPT-13 — latest ECL snapshot
  const eclSnapshot = useReportData("rpt-13", { sort: "ecl_weighted:desc", limit: 200 });

  // RPT-18 — ECL roll-forward (YTD)
  const eclRollForward = useReportData("rpt-18", {
    sort: "tanggal:asc",
    limit: 50,
    "filter[ytd]": "true",
  });

  // RPT-27 — ECL scenario sensitivity
  const sensitivity = useReportData("rpt-27", { limit: 1 });

  // RPT-23 — Periode buku
  const periode = useReportData("rpt-23", {
    sort: "tanggal_close:desc",
    limit: 12,
  });

  // RPT-13 aggregates
  const eclRows: Rpt13Row[] = eclSnapshot.data?.data ?? [];
  const totalECL = eclRows.reduce((s, r) => s + parseDecimal(r.ecl_weighted), 0);
  const totalEAD = eclRows.reduce((s, r) => s + parseDecimal(r.ead_idr), 0);
  const coverageRatio = totalEAD > 0 ? (totalECL / totalEAD) * 100 : 0;
  const stage1 = eclRows.filter((r) => r.stage === 1);
  const stage2 = eclRows.filter((r) => r.stage === 2);
  const stage3 = eclRows.filter((r) => r.stage === 3);

  // RPT-18 roll-forward data
  const rollRows: Rpt18Row[] = eclRollForward.data?.data ?? [];
  const rollForwardData = rollRows.map((r) => ({
    tanggal: r.tanggal,
    ecl_movement_mtd: parseDecimal(r.ecl_movement ?? "0"),
    ecl_movement_ytd: parseDecimal(r.closing_ecl) - parseDecimal(rollRows[0]?.opening_ecl ?? "0"),
  }));
  const latestRoll = rollRows[rollRows.length - 1];
  const eclChange = latestRoll
    ? parseDecimal(latestRoll.ecl_movement ?? "0")
    : 0;

  // RPT-27 sensitivity
  const sensRows: Rpt27Row[] = sensitivity.data?.data ?? [];
  const sensRow = sensRows[0];
  const sensData = sensRow
    ? {
        good: parseDecimal(sensRow.ecl_fl_good_total),
        normal: parseDecimal(sensRow.ecl_fl_normal_total),
        bad: parseDecimal(sensRow.ecl_fl_bad_total),
        weighted: parseDecimal(sensRow.ecl_weighted_total),
        wGood: parseDecimal(sensRow.w_good ?? "0.25"),
        wNormal: parseDecimal(sensRow.w_normal ?? "0.5"),
        wBad: parseDecimal(sensRow.w_bad ?? "0.25"),
      }
    : null;

  // RPT-23 periode
  const periodeRows: Rpt23Row[] = periode.data?.data ?? [];
  const currentPeriode = periodeRows.find((r) => r.is_current);
  const periodeTimeline = periodeRows.map((r) => ({
    kode: r.kode,
    status: r.status,
    tanggalClose: r.tanggal_close ?? undefined,
  }));

  // Stage distribution for donut
  const stageDistData = [
    { stage: 1 as const, count: stage1.length, ecl: stage1.reduce((s, r) => s + parseDecimal(r.ecl_weighted), 0) },
    { stage: 2 as const, count: stage2.length, ecl: stage2.reduce((s, r) => s + parseDecimal(r.ecl_weighted), 0) },
    { stage: 3 as const, count: stage3.length, ecl: stage3.reduce((s, r) => s + parseDecimal(r.ecl_weighted), 0) },
  ].filter((s) => s.count > 0);

  // Top-10 ECL table
  const top10 = [...eclRows].sort((a, b) => parseDecimal(b.ecl_weighted) - parseDecimal(a.ecl_weighted)).slice(0, 10);
  const top10Columns: ColumnDef<Rpt13Row>[] = [
    { accessorKey: "kode_instrumen", header: "Kode", cell: ({ getValue }) => <span className="font-mono text-xs">{String(getValue())}</span> },
    { accessorKey: "nama", header: "Nama", cell: ({ getValue }) => <span className="line-clamp-1">{String(getValue() ?? "—")}</span> },
    { accessorKey: "stage", header: "Stage", cell: ({ getValue }) => <Badge variant={getValue() === 1 ? "default" : getValue() === 2 ? "secondary" : "destructive"}>S{String(getValue())}</Badge> },
    {
      accessorKey: "ead_idr",
      header: "EAD",
      cell: ({ getValue }) => (
        <span className="tabular-nums" aria-label={`EAD ${formatIDR(parseDecimal(String(getValue())))}`}>
          {formatIDRAbbrev(parseDecimal(String(getValue())))}
        </span>
      ),
    },
    {
      accessorKey: "ecl_weighted",
      header: "ECL Weighted",
      cell: ({ getValue }) => (
        <span className="tabular-nums font-medium" aria-label={`ECL ${formatIDR(parseDecimal(String(getValue())))}`}>
          {formatIDRAbbrev(parseDecimal(String(getValue())))}
        </span>
      ),
    },
  ];

  return (
    <DashboardShell
      title="CFO Dashboard"
      subtitle={`Eksekutif · ${username} · MFA Terverifikasi`}
      icon={TrendingUp}
      dashboardLabel="CFO"
      badge={<Badge variant="outline" className="ml-2 gap-1 text-xs"><Lock className="h-3 w-3" aria-hidden="true" />MFA</Badge>}
    >
      {/* ROW 1: KPI Cards */}
      <GridCol span={3}>
        <KPICard
          title="Total ECL Portfolio"
          value={formatIDRAbbrev(totalECL)}
          valueAriaLabel={`Total ECL Portfolio ${formatIDR(totalECL)}`}
          delta={eclChange !== 0 ? { value: eclChange, label: "pergerakan ECL bulan ini" } : undefined}
          status={totalECL > 0 ? "warning" : "success"}
          loading={eclSnapshot.isLoading}
          ariaLive="polite"
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Coverage Ratio (ECL/EAD)"
          value={`${coverageRatio.toFixed(2)}%`}
          valueAriaLabel={`Coverage Ratio ${coverageRatio.toFixed(2)} persen`}
          subLabel={`EAD ${formatIDRAbbrev(totalEAD)}`}
          status={coverageRatio < 1 ? "success" : coverageRatio < 3 ? "warning" : "danger"}
          loading={eclSnapshot.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Instrumen Stage 2+3"
          value={`${stage2.length + stage3.length} instrumen`}
          valueAriaLabel={`${stage2.length + stage3.length} instrumen di Stage 2 atau Stage 3`}
          subLabel={`Stage 2: ${stage2.length} · Stage 3: ${stage3.length}`}
          status={stage3.length > 0 ? "danger" : stage2.length > 5 ? "warning" : "success"}
          loading={eclSnapshot.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Status Periode"
          value={currentPeriode?.kode ?? "—"}
          subLabel={currentPeriode?.status ?? "Tidak ada periode aktif"}
          status={
            currentPeriode?.status === "HARD_CLOSED"
              ? "success"
              : currentPeriode?.status === "SOFT_CLOSED"
                ? "warning"
                : "default"
          }
          loading={periode.isLoading}
        />
      </GridCol>

      {/* Hard-close CTA banner */}
      {canHardClose && currentPeriode?.status === "SOFT_CLOSED" && (
        <GridCol span={12}>
          <div role="alert" className="flex items-center gap-3 rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900">
            <AlertTriangle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
            <span>Periode <strong>{currentPeriode.kode}</strong> dalam status Soft-Closed. Hard-close diperlukan sebelum laporan akhir dapat di-finalisasi.</span>
            <Button
              asChild
              variant="outline"
              size="sm"
              className="ml-auto border-amber-600 text-amber-700 hover:bg-amber-100"
            >
              <Link href="/periode-buku">Hard-close →</Link>
            </Button>
          </div>
        </GridCol>
      )}

      {/* ROW 2: Stage Distribution + Scenario Sensitivity */}
      <GridCol span={4}>
        <WidgetCard
          title="Distribusi Stage"
          tooltip="Distribusi instrumen berdasarkan IFRS 9 stage classification"
          dashboardLabel="CFO"
          isLoading={eclSnapshot.isLoading}
          isError={eclSnapshot.isError}
          onRetry={eclSnapshot.refetch}
          headerAction={<a href="/reports/rpt-13" className="text-xs text-primary hover:underline">Rincian →</a>}
        >
          {stageDistData.length === 0 ? (
            <WidgetEmpty message="Belum ada data ECL." />
          ) : (
            <StageDistributionDonut data={stageDistData} />
          )}
        </WidgetCard>
      </GridCol>
      <GridCol span={8}>
        <WidgetCard
          title="Sensitivitas Skenario ECL"
          tooltip="Perbandingan ECL weighted vs skenario Good, Normal, Bad (ALCO-approved weights)"
          dashboardLabel="CFO"
          isLoading={sensitivity.isLoading}
          isError={sensitivity.isError}
          onRetry={sensitivity.refetch}
          headerAction={<a href="/ecl/parameters" className="text-xs text-primary hover:underline">Parameter ALCO →</a>}
        >
          {sensData === null ? (
            <WidgetEmpty message="Belum ada data sensitivitas skenario." />
          ) : (
            <ScenarioSensitivityBar
              good={sensData.good}
              normal={sensData.normal}
              bad={sensData.bad}
              weighted={sensData.weighted}
              wGood={sensData.wGood}
              wNormal={sensData.wNormal}
              wBad={sensData.wBad}
            />
          )}
        </WidgetCard>
      </GridCol>

      {/* ROW 3: ECL Roll-Forward */}
      <GridCol span={12}>
        <WidgetCard
          title="ECL Roll-Forward (YTD)"
          tooltip="Pergerakan ECL kumulatif year-to-date"
          dashboardLabel="CFO"
          isLoading={eclRollForward.isLoading}
          isError={eclRollForward.isError}
          onRetry={eclRollForward.refetch}
          headerAction={<a href="/reports/rpt-18" className="text-xs text-primary hover:underline">Laporan lengkap →</a>}
        >
          {rollForwardData.length === 0 ? (
            <WidgetEmpty message="Belum ada data roll-forward." />
          ) : (
            <ECLRollForwardLine data={rollForwardData} mode="ytd" />
          )}
        </WidgetCard>
      </GridCol>

      {/* ROW 4: Top-10 ECL + Periode Timeline */}
      <GridCol span={7}>
        <WidgetCard
          title="Top-10 ECL Tertinggi"
          tooltip="10 instrumen dengan beban ECL tertinggi"
          dashboardLabel="CFO"
          isLoading={eclSnapshot.isLoading}
          isError={eclSnapshot.isError}
          onRetry={eclSnapshot.refetch}
          headerAction={<a href="/reports/rpt-13" className="text-xs text-primary hover:underline">Semua →</a>}
        >
          <RecentTransactionsList
            data={top10}
            columns={top10Columns}
            emptyMessage="Belum ada data ECL."
            ariaLabel="Top-10 Instrumen ECL Tertinggi — BLIPS CFO Dashboard"
          />
        </WidgetCard>
      </GridCol>
      <GridCol span={5}>
        <WidgetCard
          title="Timeline Periode Buku"
          tooltip="Status penutupan periode buku"
          dashboardLabel="CFO"
          isLoading={periode.isLoading}
          isError={periode.isError}
          onRetry={periode.refetch}
          headerAction={<a href="/periode-buku" className="text-xs text-primary hover:underline">Kelola →</a>}
        >
          <PeriodeBukuTimeline
            data={periodeTimeline}
            currentPeriodeKode={currentPeriode?.kode}
          />
        </WidgetCard>
      </GridCol>

      {/* Seal run CTA if eligible */}
      {canSealRun && (
        <GridCol span={12}>
          <div className="flex items-center gap-3 rounded-md border px-4 py-3 text-sm">
            <span className="text-muted-foreground">Ada calc run yang belum di-seal?</span>
            <Button asChild variant="outline" size="sm" className="ml-auto">
              <Link href="/ecl/calc-run">Lihat Calc Runs →</Link>
            </Button>
          </div>
        </GridCol>
      )}

      <GridCol span={12}>
        <JobStatusList onViewAll={() => router.push("/jobs")} />
      </GridCol>
    </DashboardShell>
  );
}

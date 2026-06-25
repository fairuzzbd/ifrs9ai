"use client";

/**
 * P5-M15 — Akuntansi Dashboard client component.
 */

import * as React from "react";
import { BookOpen, AlertTriangle, CheckCircle } from "lucide-react";
import { type ColumnDef } from "@tanstack/react-table";
import { useReportData } from "@/lib/hooks/useReportData";
import { parseDecimal, formatIDRAbbrev, formatIDR, formatDate } from "@/lib/format";
import { DashboardShell, GridCol } from "@/components/blips/dashboard/DashboardShell";
import { WidgetCard, WidgetEmpty } from "@/components/blips/dashboard/WidgetCard";
import { KPICard } from "@/components/blips/dashboard/KPICard";
import { WorkflowQueueList } from "@/components/blips/dashboard/WorkflowQueueList";
import { RecentTransactionsList } from "@/components/blips/dashboard/RecentTransactionsList";
import { GLDeliveryStatusGauge } from "@/components/blips/dashboard/GLDeliveryStatusGauge";
import { PeriodeBukuTimeline } from "@/components/blips/dashboard/PeriodeBukuTimeline";
import { JobStatusList } from "@/components/blips/dashboard/JobStatusList";
import { Badge } from "@/components/ui/badge";
import { useRouter } from "next/navigation";
import Link from "next/link";
import type { Rpt22Row, Rpt22bRow, Rpt23Row, Rpt26Row, Rpt05Row } from "@/lib/schemas/dashboard.schema";

interface AkuntansiDashboardClientProps {
  username: string;
  permissions: string[];
  userId: string;
  canApproveJurnal: boolean;
}

export function AkuntansiDashboardClient({
  username,
  canApproveJurnal,
}: AkuntansiDashboardClientProps) {
  const router = useRouter();
  const today = new Date().toISOString().slice(0, 10);

  const jurnalPending = useReportData("rpt-26", {
    "filter[entity_type]": "JURNAL",
    "filter[status]": "PENDING_APPROVAL",
    sort: "created_at:desc",
    limit: 20,
  });

  const glDelivery = useReportData("rpt-22b", { limit: 200 });

  const fxRate = useReportData("rpt-05", {
    sort: "tanggal:desc",
    limit: 5,
  });

  const periode = useReportData("rpt-23", {
    sort: "tanggal_close:desc",
    limit: 12,
  });

  const recentJurnal = useReportData("rpt-22", {
    sort: "posted_at:desc",
    limit: 20,
  });

  // GL delivery aggregates
  const glRows: Rpt22bRow[] = glDelivery.data?.data ?? [];
  const glTotals = glRows.reduce(
    (acc, r) => ({
      delivered: acc.delivered + r.delivered_count,
      failed: acc.failed + r.failed_count,
      pending: acc.pending + r.pending_count,
    }),
    { delivered: 0, failed: 0, pending: 0 },
  );
  const glTotal = glTotals.delivered + glTotals.failed + glTotals.pending;
  const glSuccessRate = glTotal > 0 ? (glTotals.delivered / glTotal) * 100 : 0;

  // FX freshness
  const fxRows: Rpt05Row[] = fxRate.data?.data ?? [];
  const latestFx = fxRows[0];
  const fxDate = latestFx?.tanggal;
  const isFxFresh = fxDate === today;
  const todayIsWeekend = [0, 6].includes(new Date().getDay());
  const fxStatus: "FRESH" | "STALE" | "WEEKEND" = isFxFresh
    ? "FRESH"
    : todayIsWeekend && fxDate === new Date(Date.now() - 86400000).toISOString().slice(0, 10)
      ? "WEEKEND"
      : "STALE";

  // Periode data
  const periodeRows: Rpt23Row[] = periode.data?.data ?? [];
  const currentPeriode = periodeRows.find((r) => r.is_current);
  const periodeTimeline = periodeRows.map((r) => ({
    kode: r.kode,
    status: r.status,
    tanggalClose: r.tanggal_close ?? undefined,
  }));

  // Pending jurnal count
  const pendingJurnalRows: Rpt26Row[] = jurnalPending.data?.data ?? [];
  const pendingCount = pendingJurnalRows.length;

  // Recent jurnal columns
  const jurnalColumns: ColumnDef<Rpt22Row>[] = [
    { accessorKey: "jurnal_id", header: "Jurnal ID", cell: ({ getValue }) => <span className="font-mono text-xs">{String(getValue()).slice(0, 8)}…</span> },
    { accessorKey: "event_code", header: "Kode Event" },
    {
      accessorKey: "kode_instrumen",
      header: "Instrumen",
      cell: ({ getValue, row }) => (
        <Link
          href={`/master/instrumen/${row.original.instrumen_id ?? ""}`}
          className="text-primary hover:underline"
        >
          {String(getValue() ?? "—")}
        </Link>
      ),
    },
    {
      accessorKey: "nominal_idr",
      header: "Nominal IDR",
      cell: ({ getValue }) => (
        <span className="tabular-nums text-right" aria-label={`Nominal: ${formatIDR(parseDecimal(String(getValue())))}`}>
          {formatIDRAbbrev(parseDecimal(String(getValue())))}
        </span>
      ),
    },
    { accessorKey: "posted_at", header: "Diposting", cell: ({ getValue }) => <span>{formatDate(String(getValue() ?? ""))}</span> },
    { accessorKey: "status_posting", header: "Status", cell: ({ getValue }) => <Badge variant="outline" className="text-xs">{String(getValue())}</Badge> },
  ];

  return (
    <DashboardShell
      title="Akuntansi Dashboard"
      subtitle={`ROLE-AKUN · ${username}`}
      icon={BookOpen}
      dashboardLabel="Akuntansi"
    >
      {/* ROW 1: KPI Cards */}
      <GridCol span={3}>
        <KPICard
          title="Jurnal Pending"
          value={`${pendingCount} jurnal`}
          valueAriaLabel={`${pendingCount} jurnal menunggu approval`}
          status={pendingCount > 0 ? "warning" : "success"}
          loading={jurnalPending.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Success Rate GL Delivery"
          value={`${glSuccessRate.toFixed(1)}%`}
          valueAriaLabel={`GL Delivery success rate ${glSuccessRate.toFixed(1)} persen`}
          subLabel={`${glTotals.delivered.toLocaleString("id-ID")} dari ${glTotal.toLocaleString("id-ID")}`}
          status={glSuccessRate >= 95 ? "success" : glSuccessRate >= 80 ? "warning" : "danger"}
          loading={glDelivery.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Kurs FX Terkini"
          value={fxStatus === "FRESH" ? "FRESH ✓" : fxStatus === "STALE" ? "STALE !" : "WEEKEND ○"}
          subLabel={latestFx ? `${latestFx.kode_mata_uang} ${parseDecimal(latestFx.kurs_idr).toLocaleString("id-ID")} — ${latestFx.sumber ?? "JISDOR"}` : "Tidak ada data"}
          status={fxStatus === "FRESH" ? "success" : fxStatus === "STALE" ? "danger" : "default"}
          loading={fxRate.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Status Periode"
          value={currentPeriode?.kode ?? "—"}
          subLabel={currentPeriode?.status ?? "Tidak ada periode aktif"}
          status={currentPeriode?.status === "OPEN" ? "success" : currentPeriode?.status === "SOFT_CLOSED" ? "warning" : "default"}
          loading={periode.isLoading}
        />
      </GridCol>

      {/* FX STALE banner */}
      {fxStatus === "STALE" && !fxRate.isLoading && (
        <GridCol span={12}>
          <div
            role="alert"
            className="flex items-center gap-2 rounded-md border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800"
          >
            <AlertTriangle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
            <span>
              FX Rate belum diperbarui hari ini. Upload manual via Pengaturan &gt; FX Rate.
            </span>
            <Link href="/master/kurs/upload" className="ml-2 underline hover:no-underline">
              Upload FX Rate →
            </Link>
          </div>
        </GridCol>
      )}

      {/* ROW 2: Jurnal Pending */}
      <GridCol span={12}>
        <WidgetCard
          title="Jurnal Menunggu Posting"
          tooltip="Dokumen yang menunggu review atau approval"
          dashboardLabel="Akuntansi"
          isLoading={jurnalPending.isLoading}
          isError={jurnalPending.isError}
          onRetry={jurnalPending.refetch}
          headerAction={
            pendingCount > 0 ? (
              <Badge variant="destructive" className="text-xs">
                {pendingCount} menunggu approval
              </Badge>
            ) : (
              <div className="flex items-center gap-1">
                <CheckCircle className="h-4 w-4 text-green-600" aria-hidden="true" />
                <span className="text-xs text-green-700">Semua clear</span>
              </div>
            )
          }
        >
          <WorkflowQueueList
            data={pendingJurnalRows}
            showApproveButton={canApproveJurnal}
            linkToFull="/reports/rpt-26"
            dashboardLabel="Akuntansi"
            emptyMessage="Tidak ada jurnal yang menunggu approval saat ini."
          />
        </WidgetCard>
      </GridCol>

      {/* ROW 3: GL Delivery + FX Freshness */}
      <GridCol span={5}>
        <WidgetCard
          title="Success Rate GL Delivery"
          tooltip="Persentase jurnal yang berhasil dikirim ke sistem GL"
          dashboardLabel="Akuntansi"
          isLoading={glDelivery.isLoading}
          isError={glDelivery.isError}
          onRetry={glDelivery.refetch}
        >
          <GLDeliveryStatusGauge
            delivered={glTotals.delivered}
            failed={glTotals.failed}
            pending={glTotals.pending}
          />
        </WidgetCard>
      </GridCol>
      <GridCol span={7}>
        <WidgetCard
          title="Kurs FX Terkini"
          tooltip="Status kurs BI JISDOR — FRESH = sudah diperbarui hari ini"
          dashboardLabel="Akuntansi"
          isLoading={fxRate.isLoading}
          isError={fxRate.isError}
          onRetry={fxRate.refetch}
        >
          {fxRows.length === 0 ? (
            <WidgetEmpty message="Tidak ada data kurs FX." ctaLabel="Upload FX Rate →" ctaHref="/master/kurs/upload" />
          ) : (
            <div className="space-y-2">
              {fxRows.map((r, i) => (
                <div key={i} className="flex items-center justify-between text-sm border-b pb-1 last:border-0">
                  <span className="text-muted-foreground">{r.tanggal}</span>
                  <span className="font-medium">{r.kode_mata_uang} {parseDecimal(r.kurs_idr).toLocaleString("id-ID")}</span>
                  <Badge variant="outline" className="text-xs">{r.sumber ?? "JISDOR"}</Badge>
                </div>
              ))}
            </div>
          )}
        </WidgetCard>
      </GridCol>

      {/* ROW 4: Periode Buku Timeline */}
      <GridCol span={12}>
        <WidgetCard
          title="Timeline Periode Buku"
          tooltip="Status penutupan 12 periode buku terakhir"
          dashboardLabel="Akuntansi"
          isLoading={periode.isLoading}
          isError={periode.isError}
          onRetry={periode.refetch}
          headerAction={
            <a href="/periode-buku" className="text-xs text-primary hover:underline">
              Lihat detail →
            </a>
          }
        >
          <PeriodeBukuTimeline
            data={periodeTimeline}
            currentPeriodeKode={currentPeriode?.kode}
          />
        </WidgetCard>
      </GridCol>

      {/* ROW 5: Recent Jurnal Log */}
      <GridCol span={12}>
        <WidgetCard
          title="Log Jurnal Terbaru"
          tooltip="Jurnal posting terbaru"
          dashboardLabel="Akuntansi"
          isLoading={recentJurnal.isLoading}
          isError={recentJurnal.isError}
          onRetry={recentJurnal.refetch}
          headerAction={
            <a href="/reports/rpt-22" className="text-xs text-primary hover:underline">
              Lihat semua →
            </a>
          }
        >
          <RecentTransactionsList
            data={recentJurnal.data?.data ?? []}
            columns={jurnalColumns}
            emptyMessage="Tidak ada jurnal yang tersedia untuk periode ini."
            linkToFull="/reports/rpt-22"
            ariaLabel="Log Jurnal Terbaru — BLIPS Akuntansi Dashboard"
          />
        </WidgetCard>
      </GridCol>

      <GridCol span={12}>
        <JobStatusList onViewAll={() => router.push("/jobs")} />
      </GridCol>
    </DashboardShell>
  );
}

"use client";

/**
 * P5-M15 — Audit Dashboard client component.
 * Widgets: W-AU-01..04
 */

import * as React from "react";
import { ShieldCheck } from "lucide-react";
import { type ColumnDef } from "@tanstack/react-table";
import { useReportData } from "@/lib/hooks/useReportData";
import { formatDateTime } from "@/lib/format";
import { DashboardShell, GridCol } from "@/components/blips/dashboard/DashboardShell";
import { WidgetCard, WidgetEmpty } from "@/components/blips/dashboard/WidgetCard";
import { KPICard } from "@/components/blips/dashboard/KPICard";
import { AuditLogVolumeArea } from "@/components/blips/dashboard/AuditLogVolumeArea";
import { HashChainStatusBadge } from "@/components/blips/dashboard/HashChainStatusBadge";
import { RecentTransactionsList } from "@/components/blips/dashboard/RecentTransactionsList";
import { Badge } from "@/components/ui/badge";
import Link from "next/link";
import type { Rpt25Row } from "@/lib/schemas/dashboard.schema";

interface AuditDashboardClientProps {
  username: string;
  permissions: string[];
  userId: string;
}

// RPT-25 with group_by=date for volume chart
interface AuditVolRow {
  tanggal: string;
  count: number;
}

// RPT-25 with group_by=action for top actions
interface AuditActionRow {
  action: string;
  count: number;
}

export function AuditDashboardClient({ username }: AuditDashboardClientProps) {
  // W-AU-01: Audit log volume (last 30 days, group by date)
  const auditVolume = useReportData("rpt-25", {
    "filter[group_by]": "date",
    "filter[days]": "30",
    sort: "tanggal:asc",
    limit: 30,
  });

  // W-AU-02: Hash chain status — special slug via rpt-25 hash endpoint
  // We use a separate params to get hash chain status
  const hashChain = useReportData("rpt-25", {
    "filter[hash_chain]": "true",
    limit: 1,
  });

  // W-AU-03: Recent audit events (last 50)
  const recentAudit = useReportData("rpt-25", {
    sort: "event_time:desc",
    limit: 50,
  });

  // W-AU-04: Top actions (group_by=action)
  const topActions = useReportData("rpt-25", {
    "filter[group_by]": "action",
    "filter[days]": "30",
    sort: "count:desc",
    limit: 10,
  });

  // Aggregates for volume chart
  const volumeRows = (auditVolume.data?.data ?? []) as unknown as AuditVolRow[];
  const totalEvents = volumeRows.reduce((s, r) => s + (r.count ?? 0), 0);

  // Hash chain status from first row
  const hcRows = hashChain.data?.data ?? [];
  const hcRow = hcRows[0] as unknown as { hash_chain_status?: string; last_verified_at?: string; mismatch_event_id?: string } | undefined;
  const hashStatus: "VERIFIED" | "MISMATCH" | "VERIFYING" | "UNKNOWN" =
    hcRow?.hash_chain_status === "VERIFIED"
      ? "VERIFIED"
      : hcRow?.hash_chain_status === "MISMATCH"
        ? "MISMATCH"
        : hashChain.isLoading
          ? "VERIFYING"
          : "UNKNOWN";

  // Recent audit table
  const auditRows: Rpt25Row[] = recentAudit.data?.data ?? [];
  const topActionRows = (topActions.data?.data ?? []) as unknown as AuditActionRow[];

  const auditColumns: ColumnDef<Rpt25Row>[] = [
    {
      accessorKey: "event_time",
      header: "Waktu",
      cell: ({ getValue }) => (
        <span className="text-xs text-muted-foreground whitespace-nowrap">
          {formatDateTime(String(getValue() ?? ""))}
        </span>
      ),
    },
    {
      accessorKey: "actor_username",
      header: "User",
      cell: ({ getValue }) => <span className="font-medium text-sm">{String(getValue() ?? "—")}</span>,
    },
    {
      accessorKey: "actor_role",
      header: "Role",
      cell: ({ getValue }) => (
        <Badge variant="outline" className="text-xs">
          {String(getValue() ?? "—")}
        </Badge>
      ),
    },
    {
      accessorKey: "action",
      header: "Aksi",
      cell: ({ getValue }) => (
        <span className="font-mono text-xs">{String(getValue() ?? "—")}</span>
      ),
    },
    {
      accessorKey: "entity_type",
      header: "Entitas",
      cell: ({ getValue }) => (
        <span className="text-xs text-muted-foreground">{String(getValue() ?? "—")}</span>
      ),
    },
    {
      accessorKey: "ip",
      header: "IP",
      cell: ({ getValue }) => (
        <span className="font-mono text-xs text-muted-foreground">{String(getValue() ?? "—")}</span>
      ),
    },
    {
      accessorKey: "trace_id",
      header: "Trace",
      cell: ({ getValue }) => {
        const v = String(getValue() ?? "");
        return (
          <span className="font-mono text-xs text-muted-foreground">
            {v ? v.slice(0, 8) + "…" : "—"}
          </span>
        );
      },
    },
  ];

  return (
    <DashboardShell
      title="Audit Dashboard"
      subtitle={`ROLE-AUDIT (read-only) · ${username}`}
      icon={ShieldCheck}
      dashboardLabel="Audit"
    >
      {/* ROW 1: KPI Cards */}
      <GridCol span={3}>
        <KPICard
          title="Total Events (30 hari)"
          value={totalEvents.toLocaleString("id-ID")}
          valueAriaLabel={`Total ${totalEvents.toLocaleString("id-ID")} audit events dalam 30 hari terakhir`}
          status="default"
          loading={auditVolume.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Status Hash Chain"
          value={hashStatus}
          valueAriaLabel={`Status integritas audit log: ${hashStatus}`}
          status={
            hashStatus === "VERIFIED"
              ? "success"
              : hashStatus === "MISMATCH"
                ? "danger"
                : "default"
          }
          loading={hashChain.isLoading}
          ariaLive={hashStatus === "MISMATCH" ? "assertive" : "polite"}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Top Action (30 hari)"
          value={topActionRows[0]?.action ?? "—"}
          valueAriaLabel={`Aksi terbanyak: ${topActionRows[0]?.action ?? "—"} sebanyak ${topActionRows[0]?.count ?? 0} kali`}
          subLabel={topActionRows[0] ? `${topActionRows[0].count.toLocaleString("id-ID")} kali` : ""}
          status="default"
          loading={topActions.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Verifikasi Terakhir"
          value={hcRow?.last_verified_at ? formatDateTime(hcRow.last_verified_at) : "—"}
          valueAriaLabel={`Hash chain terakhir diverifikasi pada ${hcRow?.last_verified_at ? formatDateTime(hcRow.last_verified_at) : "tidak diketahui"}`}
          status="default"
          loading={hashChain.isLoading}
        />
      </GridCol>

      {/* W-AU-01: Audit Log Volume Area */}
      <GridCol span={8}>
        <WidgetCard
          title="Volume Audit Log (30 Hari)"
          tooltip="Jumlah events per hari dalam 30 hari terakhir"
          dashboardLabel="Audit"
          isLoading={auditVolume.isLoading}
          isError={auditVolume.isError}
          onRetry={auditVolume.refetch}
          headerAction={<a href="/reports/rpt-25" className="text-xs text-primary hover:underline">Browser lengkap →</a>}
        >
          {volumeRows.length === 0 ? (
            <WidgetEmpty message="Belum ada data audit log." />
          ) : (
            <AuditLogVolumeArea data={volumeRows} totalEvents={totalEvents} />
          )}
        </WidgetCard>
      </GridCol>

      {/* W-AU-02: Hash Chain Status Badge */}
      <GridCol span={4}>
        <WidgetCard
          title="Integritas Audit Log"
          tooltip="Status verifikasi hash chain audit log"
          dashboardLabel="Audit"
          isLoading={hashChain.isLoading}
          isError={hashChain.isError}
          onRetry={hashChain.refetch}
          headerAction={
            <Link href="/audit/hash-verify" className="text-xs text-primary hover:underline">
              Verifikasi manual →
            </Link>
          }
        >
          <div className="flex flex-col items-center gap-4 py-4">
            <HashChainStatusBadge
              status={hashStatus}
              lastVerifiedAt={hcRow?.last_verified_at}
              mismatchEventId={hcRow?.mismatch_event_id}
            />
          </div>
        </WidgetCard>
      </GridCol>

      {/* W-AU-03: Top Actions */}
      <GridCol span={4}>
        <WidgetCard
          title="Aksi Terbanyak (30 hari)"
          tooltip="10 aksi dengan frekuensi tertinggi"
          dashboardLabel="Audit"
          isLoading={topActions.isLoading}
          isError={topActions.isError}
          onRetry={topActions.refetch}
        >
          {topActionRows.length === 0 ? (
            <WidgetEmpty message="Belum ada data." />
          ) : (
            <div className="space-y-1.5" role="list" aria-label="Top 10 Aksi Audit">
              {topActionRows.map((r, i) => (
                <div key={i} role="listitem" className="flex items-center gap-2 text-sm">
                  <span className="text-muted-foreground w-5 text-right">{i + 1}.</span>
                  <span className="font-mono flex-1 truncate">{r.action}</span>
                  <Badge variant="secondary" className="tabular-nums">
                    {r.count.toLocaleString("id-ID")}
                  </Badge>
                </div>
              ))}
            </div>
          )}
        </WidgetCard>
      </GridCol>

      {/* W-AU-04: Recent Audit Events */}
      <GridCol span={8}>
        <WidgetCard
          title="Event Audit Terbaru"
          tooltip="50 event audit log terbaru"
          dashboardLabel="Audit"
          isLoading={recentAudit.isLoading}
          isError={recentAudit.isError}
          onRetry={recentAudit.refetch}
          headerAction={<a href="/reports/rpt-25" className="text-xs text-primary hover:underline">Semua events →</a>}
        >
          <RecentTransactionsList
            data={auditRows}
            columns={auditColumns}
            emptyMessage="Belum ada audit log events."
            linkToFull="/reports/rpt-25"
            ariaLabel="Event Audit Terbaru — BLIPS Audit Dashboard"
          />
        </WidgetCard>
      </GridCol>
    </DashboardShell>
  );
}

"use client";

/**
 * P5-M15 — Treasury Dashboard client component.
 * Widgets: W-TR-01..05 (Eksposur, Jatuh Tempo, Pending Workflow, Recent Transactions).
 */

import * as React from "react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
  LabelList,
} from "recharts";
import { TrendingUp } from "lucide-react";
import { type ColumnDef } from "@tanstack/react-table";
import { useReportData } from "@/lib/hooks/useReportData";
import { parseDecimal, formatIDRAbbrev, formatIDR, formatDateTime } from "@/lib/format";
import { DashboardShell, GridCol } from "@/components/blips/dashboard/DashboardShell";
import { WidgetCard, WidgetEmpty } from "@/components/blips/dashboard/WidgetCard";
import { KPICard } from "@/components/blips/dashboard/KPICard";
import { MaturityBucketBar } from "@/components/blips/dashboard/MaturityBucketBar";
import { WorkflowQueueList } from "@/components/blips/dashboard/WorkflowQueueList";
import { RecentTransactionsList } from "@/components/blips/dashboard/RecentTransactionsList";
import { JobStatusList } from "@/components/blips/dashboard/JobStatusList";
import { Badge } from "@/components/ui/badge";
import { useRouter } from "next/navigation";
import type { Rpt06Row, Rpt10Row, Rpt01Row, Rpt26Row } from "@/lib/schemas/dashboard.schema";

interface TreasuryDashboardClientProps {
  username: string;
  permissions: string[];
  userId: string;
}

// Maturity bucket helper
function bucketMaturity(rows: Rpt10Row[]): { bucket: "≤30d" | "31-90d" | "91-180d" | ">180d"; nominalIdr: number; count: number }[] {
  const today = new Date();
  const buckets: Record<string, { nominalIdr: number; count: number }> = {
    "≤30d": { nominalIdr: 0, count: 0 },
    "31-90d": { nominalIdr: 0, count: 0 },
    "91-180d": { nominalIdr: 0, count: 0 },
    ">180d": { nominalIdr: 0, count: 0 },
  };

  for (const r of rows) {
    const jt = new Date(r.tanggal_jatuh_tempo);
    const days = Math.ceil((jt.getTime() - today.getTime()) / 86400000);
    const nominal = parseDecimal(r.nominal_idr);
    let key: keyof typeof buckets;
    if (days <= 30) key = "≤30d";
    else if (days <= 90) key = "31-90d";
    else if (days <= 180) key = "91-180d";
    else key = ">180d";
    buckets[key].nominalIdr += nominal;
    buckets[key].count += 1;
  }

  return Object.entries(buckets).map(([bucket, v]) => ({
    bucket: bucket as "≤30d" | "31-90d" | "91-180d" | ">180d",
    nominalIdr: v.nominalIdr,
    count: v.count,
  }));
}

const PORTFOLIO_COLORS = [
  "hsl(204, 86%, 53%)",
  "hsl(142, 76%, 36%)",
  "hsl(38, 92%, 50%)",
  "hsl(0, 72%, 51%)",
  "hsl(280, 65%, 60%)",
];

export function TreasuryDashboardClient({
  username,
  permissions,
}: TreasuryDashboardClientProps) {
  const router = useRouter();
  const today = new Date().toISOString().slice(0, 10);
  const plus90 = new Date(Date.now() + 90 * 86400000).toISOString().slice(0, 10);

  // Data fetches
  const instrumen = useReportData("rpt-01", {
    "filter[status]": "AKTIF",
    sort: "ead_idr:desc",
    limit: 200,
  });
  const maturities = useReportData("rpt-10", {
    "filter[tanggal_jatuh_tempo]": `gte:${today}`,
    sort: "tanggal_jatuh_tempo:asc",
    limit: 200,
  });
  const workflow = useReportData("rpt-26", {
    "filter[status]": "PENDING",
    sort: "created_at:desc",
    limit: 20,
  });
  const transactions = useReportData("rpt-06", {
    sort: "tanggal_penempatan:desc",
    limit: 20,
  });

  // KPI aggregates from instrumen data
  const instrumenRows: Rpt01Row[] = instrumen.data?.data ?? [];
  const totalEad = instrumenRows.reduce((s, r) => s + parseDecimal(r.ead_idr), 0);
  const totalCount = instrumenRows.length;

  // Maturity ≤30d
  const matRows: Rpt10Row[] = maturities.data?.data ?? [];
  const buckets = bucketMaturity(matRows);
  const due30d = buckets[0]?.nominalIdr ?? 0;
  const due30dCount = buckets[0]?.count ?? 0;

  // Workflow pending
  const wfRows: Rpt26Row[] = workflow.data?.data ?? [];
  const pendingCount = wfRows.length;

  // Transactions this month
  const txRows: Rpt06Row[] = transactions.data?.data ?? [];
  const thisMonth = new Date().toISOString().slice(0, 7);
  const txThisMonth = txRows.filter((r) =>
    r.tanggal_penempatan?.startsWith(thisMonth),
  ).length;

  // Eksposur portfolio by jenis
  const byJenis = instrumenRows.reduce<Record<string, number>>((acc, r) => {
    acc[r.jenis_instrumen] = (acc[r.jenis_instrumen] ?? 0) + parseDecimal(r.ead_idr);
    return acc;
  }, {});
  const jenisChartData = Object.entries(byJenis)
    .sort(([, a], [, b]) => b - a)
    .map(([name, value]) => ({ name, value }));

  // Eksposur by bank
  const byBank = instrumenRows.reduce<Record<string, number>>((acc, r) => {
    const key = r.nama_bank ?? r.counterparty ?? "Lainnya";
    acc[key] = (acc[key] ?? 0) + parseDecimal(r.ead_idr);
    return acc;
  }, {});
  const bankChartData = Object.entries(byBank)
    .sort(([, a], [, b]) => b - a)
    .slice(0, 10)
    .map(([name, value]) => ({ name, value }));

  const txColumns: ColumnDef<Rpt06Row>[] = [
    { accessorKey: "kode", header: "Kode", cell: ({ getValue }) => <span className="font-mono">{String(getValue())}</span> },
    { accessorKey: "jenis_instrumen", header: "Jenis" },
    { accessorKey: "counterparty", header: "Counterparty", cell: ({ getValue }) => <span>{String(getValue() ?? "—")}</span> },
    {
      accessorKey: "nominal_idr",
      header: "Nominal IDR",
      cell: ({ getValue }) => (
        <span
          className="text-right tabular-nums"
          aria-label={`Nominal: ${formatIDR(parseDecimal(String(getValue())))}`}
        >
          {formatIDRAbbrev(parseDecimal(String(getValue())))}
        </span>
      ),
    },
    { accessorKey: "tanggal_penempatan", header: "Tanggal", cell: ({ getValue }) => <span>{String(getValue() ?? "—").slice(0, 10)}</span> },
    { accessorKey: "status", header: "Status", cell: ({ getValue }) => <Badge variant="outline" className="text-xs">{String(getValue())}</Badge> },
  ];

  return (
    <DashboardShell
      title="Treasury Dashboard"
      subtitle={`ROLE-MAKER-TR / ROLE-APPR-TR · ${username}`}
      icon={TrendingUp}
      dashboardLabel="Treasury"
    >
      {/* ROW 1: KPI Cards (4 × col-3) */}
      <GridCol span={3}>
        <KPICard
          title="Total Portfolio NAV"
          value={formatIDRAbbrev(totalEad)}
          valueAriaLabel={`${formatIDR(totalEad)} total eksposur`}
          subLabel={`${totalCount.toLocaleString("id-ID")} instrumen aktif`}
          loading={instrumen.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Jatuh Tempo ≤30 Hari"
          value={formatIDRAbbrev(due30d)}
          valueAriaLabel={`${formatIDR(due30d)} jatuh tempo dalam 30 hari`}
          subLabel={`${due30dCount} instrumen`}
          status={due30d > 0 ? "warning" : "default"}
          loading={maturities.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Pending Review"
          value={`${pendingCount} dokumen`}
          valueAriaLabel={`${pendingCount} dokumen menunggu review`}
          status={pendingCount > 0 ? "warning" : "success"}
          loading={workflow.isLoading}
        />
      </GridCol>
      <GridCol span={3}>
        <KPICard
          title="Transaksi Bulan Ini"
          value={`${txThisMonth} transaksi`}
          valueAriaLabel={`${txThisMonth} transaksi dalam bulan ini`}
          loading={transactions.isLoading}
        />
      </GridCol>

      {/* ROW 2: Eksposur Portfolio + Jatuh Tempo */}
      <GridCol span={7}>
        <WidgetCard
          title="Eksposur Portfolio by Jenis"
          tooltip="Total EAD per jenis instrumen"
          dashboardLabel="Treasury"
          isLoading={instrumen.isLoading}
          isError={instrumen.isError}
          onRetry={instrumen.refetch}
          headerAction={
            <a href="/reports/rpt-01" className="text-xs text-primary hover:underline">
              Lihat RPT-01 →
            </a>
          }
        >
          {jenisChartData.length === 0 ? (
            <WidgetEmpty message="Belum ada data instrumen aktif." />
          ) : (
            <>
              <ResponsiveContainer width="100%" height={200}>
                <BarChart
                  data={jenisChartData}
                  layout="vertical"
                  role="img"
                  aria-label="Eksposur Portfolio by Jenis Instrumen"
                >
                  <title>Eksposur Portfolio by Jenis Instrumen</title>
                  <desc>Bar chart total EAD IDR per jenis instrumen aktif</desc>
                  <CartesianGrid strokeDasharray="3 3" horizontal={false} className="stroke-border" />
                  <XAxis
                    type="number"
                    tickFormatter={(v: number) => formatIDRAbbrev(v)}
                    tick={{ fontSize: 10 }}
                    aria-hidden="true"
                  />
                  <YAxis type="category" dataKey="name" width={80} tick={{ fontSize: 10 }} aria-hidden="true" />
                  <Tooltip formatter={(v: number) => [formatIDR(v), "EAD IDR"]} />
                  <Bar dataKey="value" radius={[0, 4, 4, 0]}>
                    {jenisChartData.map((_, i) => (
                      <Cell
                        key={i}
                        fill={PORTFOLIO_COLORS[i % PORTFOLIO_COLORS.length]}
                        aria-label={`${jenisChartData[i].name}: ${formatIDRAbbrev(jenisChartData[i].value)} EAD total`}
                      />
                    ))}
                    <LabelList
                      dataKey="value"
                      position="right"
                      formatter={(v: number) => formatIDRAbbrev(v)}
                      style={{ fontSize: 10 }}
                    />
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
              <table className="sr-only" aria-label="Data tabel eksposur portfolio per jenis">
                <thead><tr><th scope="col">Jenis Instrumen</th><th scope="col">EAD IDR</th></tr></thead>
                <tbody>
                  {jenisChartData.map((d) => (
                    <tr key={d.name}><td>{d.name}</td><td>{formatIDR(d.value)}</td></tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
        </WidgetCard>
      </GridCol>
      <GridCol span={5}>
        <WidgetCard
          title="Jatuh Tempo Mendatang"
          tooltip="Nominal instrumen yang jatuh tempo dalam 30/60/90 hari"
          dashboardLabel="Treasury"
          isLoading={maturities.isLoading}
          isError={maturities.isError}
          onRetry={maturities.refetch}
        >
          <MaturityBucketBar data={buckets} />
        </WidgetCard>
      </GridCol>

      {/* ROW 3: Eksposur by Bank + Pending Workflow */}
      <GridCol span={5}>
        <WidgetCard
          title="Eksposur by Bank/Counterparty"
          tooltip="Total EAD per bank atau counterparty"
          dashboardLabel="Treasury"
          isLoading={instrumen.isLoading}
          isError={instrumen.isError}
          onRetry={instrumen.refetch}
        >
          {bankChartData.length === 0 ? (
            <WidgetEmpty message="Belum ada data." />
          ) : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart
                data={bankChartData}
                layout="vertical"
                role="img"
                aria-label="Eksposur by Bank/Counterparty"
              >
                <title>Eksposur by Bank/Counterparty</title>
                <CartesianGrid strokeDasharray="3 3" horizontal={false} className="stroke-border" />
                <XAxis type="number" tickFormatter={(v: number) => formatIDRAbbrev(v)} tick={{ fontSize: 9 }} aria-hidden="true" />
                <YAxis type="category" dataKey="name" width={70} tick={{ fontSize: 9 }} aria-hidden="true" />
                <Tooltip formatter={(v: number) => [formatIDR(v), "EAD IDR"]} />
                <Bar dataKey="value" fill="hsl(204, 86%, 53%)" radius={[0, 4, 4, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </WidgetCard>
      </GridCol>
      <GridCol span={7}>
        <WidgetCard
          title="Antrian Menunggu Approval"
          tooltip="Dokumen yang menunggu review atau approval"
          dashboardLabel="Treasury"
          isLoading={workflow.isLoading}
          isError={workflow.isError}
          onRetry={workflow.refetch}
          headerAction={
            pendingCount > 0 ? (
              <Badge variant="destructive" className="text-xs">
                {pendingCount} menunggu
              </Badge>
            ) : undefined
          }
        >
          <WorkflowQueueList
            data={wfRows}
            showApproveButton={permissions.includes("jurnal.approve")}
            linkToFull="/reports/rpt-26"
            dashboardLabel="Treasury"
            emptyMessage="Tidak ada dokumen yang menunggu approval saat ini."
          />
        </WidgetCard>
      </GridCol>

      {/* ROW 4: Recent Transactions */}
      <GridCol span={12}>
        <WidgetCard
          title="Transaksi Terbaru"
          tooltip="Daftar transaksi terbaru dalam sistem"
          dashboardLabel="Treasury"
          isLoading={transactions.isLoading}
          isError={transactions.isError}
          onRetry={transactions.refetch}
          headerAction={
            <a href="/reports/rpt-06" className="text-xs text-primary hover:underline">
              Lihat semua →
            </a>
          }
        >
          <RecentTransactionsList
            data={txRows}
            columns={txColumns}
            emptyMessage="Belum ada transaksi dalam periode ini."
            linkToFull="/reports/rpt-06"
            ariaLabel="Transaksi Terbaru — BLIPS Treasury Dashboard"
          />
        </WidgetCard>
      </GridCol>

      {/* ROW 5: Active Jobs (collapsed if none) */}
      <GridCol span={12}>
        <JobStatusList onViewAll={() => router.push("/jobs")} />
      </GridCol>
    </DashboardShell>
  );
}

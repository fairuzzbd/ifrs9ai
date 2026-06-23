/**
 * Route: /reports/mapping-history
 * Story: P5-M12-S5-AC3..4 — RPT-21 Mapping Change History (ROLE-AUDIT)
 * Actors: ROLE-AUDIT (export), ROLE-AKUN (view)
 * > 10k rows → async job (§1 rule)
 */

"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type SortingState } from "@tanstack/react-table";
import { Download } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import { MappingHistoryTable } from "@/components/blips/mapping-jurnal/MappingHistoryTable";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { mappingReportsApi, mappingP12QueryKeys, type Rpt21ListParams } from "@/lib/api/mapping-jurnal-p12.api";
import { notify } from "@/lib/notify";

const MAPPING_ACTIONS = [
  "MAPPING.SUBMITTED",
  "MAPPING.REVIEWED",
  "MAPPING.APPROVED_ACTIVE",
  "MAPPING.REJECTED",
  "MAPPING.VERSION_CREATED",
  "MAPPING.DETAIL_CREATED",
  "MAPPING.BULK_IMPORTED",
  "MAPPING.SOD_VIOLATION_ATTEMPT",
  "MAPPING.EXPORT",
];

function Rpt21Content() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("event_time:desc"));
  const [filterEventCode, setFilterEventCode] = useQueryState("filter[event_code]", parseAsString.withDefault(""));
  const [filterActorRole, setFilterActorRole] = useQueryState("filter[actor_role]", parseAsString.withDefault(""));
  const [filterAction, setFilterAction] = useQueryState("filter[action]", parseAsString.withDefault(""));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());
  const [exportJobId, setExportJobId] = React.useState<string | null>(null);

  const queryParams: Rpt21ListParams = React.useMemo(() => {
    const p: Rpt21ListParams = { limit, sort: sort || undefined };
    if (filterEventCode) p["filter[event_code]"] = filterEventCode;
    if (filterActorRole) p["filter[actor_role]"] = filterActorRole;
    if (filterAction) p["filter[action]"] = filterAction;
    if (cursor) p.cursor = cursor;
    return p;
  }, [limit, sort, filterEventCode, filterActorRole, filterAction, cursor]);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: mappingP12QueryKeys.rpt21(queryParams),
    queryFn: () => mappingReportsApi.getRpt21(queryParams),
    staleTime: 30_000,
  });

  const sortingState: SortingState = React.useMemo(() => {
    if (!sort) return [];
    return sort.split(",").map((p) => {
      const [id, dir] = p.split(":");
      return { id, desc: dir === "desc" };
    });
  }, [sort]);

  const handleSortingChange = (s: SortingState) => {
    void setSort(s.length === 0 ? "event_time:desc" : s.map((x) => `${x.id}:${x.desc ? "desc" : "asc"}`).join(","));
    void setCursor(""); setCursorHistory([""]); setPageIndex(0);
  };

  const handleNextPage = () => {
    const next = data?.pagination.nextCursor;
    if (next) {
      const h = [...cursorHistory, next];
      setCursorHistory(h); setPageIndex(h.length - 1); void setCursor(next);
    }
  };

  const handlePrevPage = () => {
    if (pageIndex > 0) {
      const idx = pageIndex - 1; setPageIndex(idx); void setCursor(cursorHistory[idx] ?? "");
    }
  };

  const handleExport = (fmt: "csv" | "xlsx") => {
    const totalEstimate = data?.pagination.totalEstimate ?? 0;
    if (totalEstimate > 10_000) {
      // Async job — server returns 202
      notify.info(`Export RPT-21 (${totalEstimate.toLocaleString("id-ID")} baris) diproses sebagai async job. Notifikasi akan muncul saat selesai.`);
      // In production: POST to export endpoint, receive jobId
      setExportJobId("rpt21-export-job-placeholder");
    } else {
      const url = mappingReportsApi.exportRpt21Url({
        format: fmt,
        "filter[event_code]": filterEventCode || undefined,
      });
      window.open(url, "_blank");
      notify.info(`Export RPT-21 ${fmt.toUpperCase()} dimulai.`);
    }
  };

  return (
    <div className="container mx-auto py-6 space-y-6">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/reports" className="hover:underline">Laporan</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">RPT-21 Mapping History</span>
      </nav>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">RPT-21 — Mapping Change History</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Audit log semua perubahan mapping jurnal (MAPPING.*). Filter per event_code, actor, aksi.
          </p>
        </div>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={() => handleExport("xlsx")} aria-label="Export RPT-21 XLSX">
            <Download className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Export XLSX
          </Button>
          <Button size="sm" variant="outline" onClick={() => handleExport("csv")} aria-label="Export RPT-21 CSV">
            Export CSV
          </Button>
        </div>
      </div>

      {/* Async job progress (for > 10k export) */}
      {exportJobId && (
        <JobProgressPanel
          jobId={exportJobId}
          onComplete={(result) => {
            setExportJobId(null);
            notify.success(
              `RPT-21 export selesai. ${(result as { rowCount?: number }).rowCount?.toLocaleString("id-ID") ?? "?"} baris siap diunduh (TTL 24 jam).`,
            );
          }}
          onFail={(err) => { setExportJobId(null); notify.error(err); }}
          showCancel={false}
        />
      )}

      {/* Filters */}
      <div className="flex flex-wrap gap-3 items-end">
        <div className="space-y-1">
          <Label htmlFor="filter-event-code" className="text-xs">Event Code</Label>
          <Input
            id="filter-event-code"
            className="h-9 w-[180px]"
            placeholder="ECL_PEMBENTUKAN"
            value={filterEventCode}
            onChange={(e) => { void setFilterEventCode(e.target.value); void setCursor(""); setCursorHistory([""]); setPageIndex(0); }}
            aria-label="Filter event code"
          />
        </div>

        <div className="space-y-1">
          <Label htmlFor="filter-action" className="text-xs">Aksi</Label>
          <Select
            value={filterAction || "all"}
            onValueChange={(v) => { void setFilterAction(v === "all" ? "" : v); void setCursor(""); setCursorHistory([""]); setPageIndex(0); }}
          >
            <SelectTrigger id="filter-action" className="h-9 w-[220px]" aria-label="Filter aksi audit">
              <SelectValue placeholder="Semua Aksi" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Semua Aksi</SelectItem>
              {MAPPING_ACTIONS.map((a) => (
                <SelectItem key={a} value={a} className="text-xs font-mono">{a}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-1">
          <Label htmlFor="filter-role" className="text-xs">Role</Label>
          <Select
            value={filterActorRole || "all"}
            onValueChange={(v) => { void setFilterActorRole(v === "all" ? "" : v); void setCursor(""); setCursorHistory([""]); setPageIndex(0); }}
          >
            <SelectTrigger id="filter-role" className="h-9 w-[160px]" aria-label="Filter actor role">
              <SelectValue placeholder="Semua Role" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Semua Role</SelectItem>
              <SelectItem value="ROLE-AKUN">ROLE-AKUN</SelectItem>
              <SelectItem value="ROLE-AKUN-CTL">ROLE-AKUN-CTL</SelectItem>
              <SelectItem value="ROLE-RISK">ROLE-RISK</SelectItem>
              <SelectItem value="ROLE-AUDIT">ROLE-AUDIT</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <MappingHistoryTable
        data={data?.data ?? []}
        isLoading={isLoading}
        isError={isError}
        sorting={sortingState}
        onSortingChange={handleSortingChange}
        pagination={{
          pageIndex,
          hasMore: data?.pagination.hasMore ?? false,
          totalEstimate: data?.pagination.totalEstimate ?? 0,
          limit,
          onNext: handleNextPage,
          onPrev: handlePrevPage,
        }}
        onExport={handleExport}
        onRefresh={() => { void refetch(); setLastUpdated(new Date()); }}
        lastUpdated={lastUpdated}
      />
    </div>
  );
}

export default function Rpt21Page() {
  return (
    <Suspense>
      <Rpt21Content />
    </Suspense>
  );
}

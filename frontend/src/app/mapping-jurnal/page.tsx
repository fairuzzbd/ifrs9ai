/**
 * Route: /mapping-jurnal
 * Story: P5-M12-S1 — DataTable list semua event codes (sort/filter/export §1)
 * Actors: ROLE-AKUN (list + export), ROLE-AUDIT (read-only)
 */

"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { Upload } from "lucide-react";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { MappingStatusBadge } from "@/components/blips/mapping-jurnal/MappingStatusBadge";
import { MappingRegulatedBadge } from "@/components/blips/mapping-jurnal/MappingRegulatedBadge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

import {
  mappingP12Api,
  mappingP12QueryKeys,
  type MappingP12ListParams,
} from "@/lib/api/mapping-jurnal-p12.api";
import { notify } from "@/lib/notify";
import type { MappingP12HeaderSummary } from "@/lib/schemas/mapping-jurnal-p12.schema";
import { MAPPING_WORKFLOW_STATUS_LABELS } from "@/lib/schemas/mapping-jurnal-p12.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { format } from "date-fns";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("event_code:asc"));
  const [filterStatus, setFilterStatus] = useQueryState("filter[workflow_status]", parseAsString.withDefault(""));
  const [filterPath, setFilterPath] = useQueryState("filter[workflow_path]", parseAsString.withDefault(""));
  const [filterRegulated, setFilterRegulated] = useQueryState("filter[regulated_flag]", parseAsString.withDefault(""));
  const [filterEventCode, setFilterEventCode] = useQueryState("filter[event_code]", parseAsString.withDefault(""));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return {
    q, setQ,
    sort, setSort,
    filterStatus, setFilterStatus,
    filterPath, setFilterPath,
    filterRegulated, setFilterRegulated,
    filterEventCode, setFilterEventCode,
    cursor, setCursor,
    limit,
  };
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function MappingJurnalP12ListContent() {
  const filters = useFilters();
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const queryParams: MappingP12ListParams = React.useMemo(() => {
    const p: MappingP12ListParams = {
      limit: filters.limit,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterPath) p["filter[workflow_path]"] = filters.filterPath;
    if (filters.filterRegulated !== "") p["filter[regulated_flag]"] = filters.filterRegulated === "true";
    if (filters.filterEventCode) p["filter[event_code]"] = filters.filterEventCode;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: mappingP12QueryKeys.list(queryParams),
    queryFn: () => mappingP12Api.list(queryParams),
    staleTime: 30_000,
  });

  const sortingState: SortingState = React.useMemo(() => {
    if (!filters.sort) return [];
    return filters.sort.split(",").map((p) => {
      const [id, dir] = p.split(":");
      return { id, desc: dir === "desc" };
    });
  }, [filters.sort]);

  const handleSortingChange = (s: SortingState) => {
    void filters.setSort(
      s.length === 0 ? "event_code:asc" : s.map((x) => `${x.id}:${x.desc ? "desc" : "asc"}`).join(","),
    );
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const toggleSort = (colId: string) => {
    const cur = sortingState.find((s) => s.id === colId);
    if (!cur) handleSortingChange([{ id: colId, desc: false }]);
    else if (!cur.desc) handleSortingChange([{ id: colId, desc: true }]);
    else handleSortingChange([]);
  };

  const handleNextPage = () => {
    const next = data?.pagination.nextCursor;
    if (next) {
      const h = [...cursorHistory, next];
      setCursorHistory(h);
      setPageIndex(h.length - 1);
      void filters.setCursor(next);
    }
  };

  const handlePrevPage = () => {
    if (pageIndex > 0) {
      const idx = pageIndex - 1;
      setPageIndex(idx);
      void filters.setCursor(cursorHistory[idx] ?? "");
    }
  };

  const activeFilters: ActiveFilter[] = React.useMemo(() => {
    const f: ActiveFilter[] = [];
    if (filters.filterStatus)
      f.push({ key: "filter[workflow_status]", label: "Status", value: filters.filterStatus, displayValue: MAPPING_WORKFLOW_STATUS_LABELS[filters.filterStatus as keyof typeof MAPPING_WORKFLOW_STATUS_LABELS] ?? filters.filterStatus });
    if (filters.filterPath)
      f.push({ key: "filter[workflow_path]", label: "Jalur", value: filters.filterPath, displayValue: filters.filterPath });
    if (filters.filterRegulated !== "")
      f.push({ key: "filter[regulated_flag]", label: "Regulated", value: filters.filterRegulated, displayValue: filters.filterRegulated === "true" ? "Ya" : "Tidak" });
    if (filters.filterEventCode)
      f.push({ key: "filter[event_code]", label: "Event Code", value: filters.filterEventCode, displayValue: filters.filterEventCode });
    return f;
  }, [filters.filterStatus, filters.filterPath, filters.filterRegulated, filters.filterEventCode]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    if (key === "filter[workflow_path]") void filters.setFilterPath("");
    if (key === "filter[regulated_flag]") void filters.setFilterRegulated("");
    if (key === "filter[event_code]") void filters.setFilterEventCode("");
    void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterStatus(""); void filters.setFilterPath("");
    void filters.setFilterRegulated(""); void filters.setFilterEventCode("");
    void filters.setQ(""); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0);
  };

  const columns: ColumnDef<MappingP12HeaderSummary>[] = React.useMemo(
    () => [
      {
        id: "eventCode",
        header: () => <SortHeader label="Event Code" sortKey="event_code" sorting={sortingState} onToggle={toggleSort} />,
        accessorKey: "eventCode",
        cell: ({ row }) => (
          <Link
            href={`/mapping-jurnal/${row.original.eventCode}`}
            className="font-mono text-sm font-bold text-primary hover:underline"
            aria-label={`Lihat detail mapping ${row.original.eventCode}`}
          >
            {row.original.eventCode}
          </Link>
        ),
      },
      {
        id: "namaEvent",
        header: () => <SortHeader label="Nama Event" sortKey="nama_event" sorting={sortingState} onToggle={toggleSort} />,
        accessorKey: "namaEvent",
        cell: ({ row }) => (
          <span className="block max-w-[200px] truncate text-sm" title={row.original.namaEvent}>
            {row.original.namaEvent}
          </span>
        ),
      },
      {
        id: "workflowPath",
        header: "Jalur",
        cell: ({ row }) => (
          <MappingRegulatedBadge regulated={row.original.regulatedFlag} workflowPath={row.original.workflowPath} />
        ),
      },
      {
        id: "workflowStatus",
        header: () => <SortHeader label="Status" sortKey="workflow_status" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => <MappingStatusBadge status={row.original.workflowStatus} size="sm" />,
      },
      {
        id: "aktifFlag",
        header: () => <SortHeader label="Aktif" sortKey="aktif_flag" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => (
          <span className={`text-xs font-medium ${row.original.aktifFlag ? "text-green-700" : "text-muted-foreground"}`}>
            {row.original.aktifFlag ? "Ya" : "Tidak"}
          </span>
        ),
      },
      {
        id: "updatedAt",
        header: () => <SortHeader label="Diperbarui" sortKey="updated_at" sorting={sortingState} onToggle={toggleSort} />,
        accessorKey: "updatedAt",
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground whitespace-nowrap">
            {new Date(row.original.updatedAt).toLocaleDateString("id-ID", { timeZone: "Asia/Jakarta" })}
          </span>
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sortingState],
  );

  return (
    <div className="container mx-auto py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>APP-D</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Mapping Jurnal</span>
      </nav>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold">Mapping Jurnal</h1>
        <Button size="sm" variant="outline" asChild>
          <Link href="/mapping-jurnal/import" aria-label="Import bulk mapping XLSX">
            <Upload className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Import Bulk
          </Link>
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={data?.data ?? []}
        pagination={data?.pagination}
        isLoading={isLoading}
        isError={isError}
        error={null}
        sorting={sortingState}
        onSortingChange={handleSortingChange}
        searchValue={filters.q}
        onSearchChange={(v) => { void filters.setQ(v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}
        searchPlaceholder="Cari event code, nama event..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            <Select
              value={filters.filterStatus || "all"}
              onValueChange={(v) => { void filters.setFilterStatus(v === "all" ? "" : v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}
            >
              <SelectTrigger className="h-9 w-[200px]" aria-label="Filter status workflow">
                <SelectValue placeholder="Semua Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
                {Object.entries(MAPPING_WORKFLOW_STATUS_LABELS).map(([v, l]) => (
                  <SelectItem key={v} value={v}>{l}</SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={filters.filterPath || "all"}
              onValueChange={(v) => { void filters.setFilterPath(v === "all" ? "" : v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}
            >
              <SelectTrigger className="h-9 w-[160px]" aria-label="Filter jalur workflow">
                <SelectValue placeholder="Semua Jalur" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Jalur</SelectItem>
                <SelectItem value="4-eyes">4-eyes (Operasional)</SelectItem>
                <SelectItem value="6-eyes">6-eyes (Regulated)</SelectItem>
              </SelectContent>
            </Select>

            <Select
              value={filters.filterRegulated === "" ? "all" : filters.filterRegulated}
              onValueChange={(v) => { void filters.setFilterRegulated(v === "all" ? "" : v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}
            >
              <SelectTrigger className="h-9 w-[140px]" aria-label="Filter regulated flag">
                <SelectValue placeholder="Regulated" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua</SelectItem>
                <SelectItem value="true">Regulated</SelectItem>
                <SelectItem value="false">Operasional</SelectItem>
              </SelectContent>
            </Select>
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = mappingP12Api.exportUrl({
            format: fmt,
            "filter[workflow_status]": filters.filterStatus || undefined,
            "filter[regulated_flag]": filters.filterRegulated !== "" ? filters.filterRegulated === "true" : undefined,
          });
          window.open(url, "_blank");
          notify.info(`Export ${fmt.toUpperCase()} mapping jurnal dimulai.`);
        }}
        onRefresh={() => { void refetch(); setLastUpdated(new Date()); }}
        lastUpdated={lastUpdated}
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada mapping yang cocok dengan filter aktif."
            : "Belum ada mapping jurnal terdaftar."
        }
        onRetry={() => void refetch()}
      />
    </div>
  );
}

export default function MappingJurnalP12Page() {
  return (
    <Suspense>
      <MappingJurnalP12ListContent />
    </Suspense>
  );
}

"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Plus, RefreshCw, Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { WorkflowPathBadge } from "@/components/blips/jurnal/WorkflowPathBadge";
import { mappingApi, type MappingListParams } from "@/lib/api/jurnal.api";
import type { MappingWorkflowStatus } from "@/lib/schemas/jurnal.schema";
import { useJurnalStore } from "@/lib/stores/jurnal.store";
import { cn } from "@/lib/utils";

const STATUS_LABELS: Record<MappingWorkflowStatus, string> = {
  DRAFT: "Draft",
  PENDING_REVIEW: "Menunggu Review",
  PENDING_APPROVAL: "Menunggu Persetujuan",
  PENDING_APPROVAL_2: "Menunggu Persetujuan 2",
  APPROVED_ACTIVE: "Aktif",
  REJECTED: "Ditolak",
  RETURNED: "Dikembalikan",
  WITHDRAWN: "Ditarik",
};

const STATUS_VARIANT: Record<MappingWorkflowStatus, string> = {
  DRAFT: "outline",
  PENDING_REVIEW: "secondary",
  PENDING_APPROVAL: "secondary",
  PENDING_APPROVAL_2: "secondary",
  APPROVED_ACTIVE: "default",
  REJECTED: "destructive",
  RETURNED: "destructive",
  WITHDRAWN: "outline",
};

export default function MappingJurnalListPage() {
  const router = useRouter();
  const { mappingFilters, setMappingFilters } = useJurnalStore();
  const [search, setSearch] = React.useState(mappingFilters.q ?? "");
  const [statusFilter, setStatusFilter] = React.useState(
    mappingFilters.workflowStatus ?? "",
  );
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([]);

  const params: MappingListParams = {
    cursor,
    limit: 50,
    sort: "created_at:desc",
    q: search || undefined,
    "filter[workflow_status]": statusFilter || undefined,
  };

  const { data, isFetching, refetch } = useQuery({
    queryKey: ["mapping-jurnal-list", params],
    queryFn: () => mappingApi.list(params),
  });

  const rows = data?.data ?? [];
  const pagination = data?.pagination;

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setMappingFilters({ q: search, workflowStatus: statusFilter });
    setCursor(null);
    setCursorHistory([]);
  };

  const handleNext = () => {
    if (!pagination?.nextCursor) return;
    if (cursor) setCursorHistory((h) => [...h, cursor]);
    setCursor(pagination.nextCursor ?? null);
  };

  const handlePrev = () => {
    const history = [...cursorHistory];
    const prev = history.pop() ?? null;
    setCursorHistory(history);
    setCursor(prev);
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-6 py-4">
        <div>
          <h1 className="text-xl font-semibold">Mapping Jurnal</h1>
          <p className="text-sm text-muted-foreground">
            Template mapping event code ke baris jurnal DEBIT/KREDIT
          </p>
        </div>
        <Button onClick={() => router.push("/jrnl/mapping/new")}>
          <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
          Buat Mapping
        </Button>
      </div>

      {/* Filter bar */}
      <form
        onSubmit={handleSearch}
        className="flex flex-wrap gap-3 px-6 py-3 border-b bg-muted/30"
      >
        <Input
          placeholder="Cari kode event atau nama..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-8 w-64"
          aria-label="Cari mapping jurnal"
        />
        <Select
          value={statusFilter}
          onValueChange={setStatusFilter}
        >
          <SelectTrigger className="h-8 w-44" aria-label="Filter status">
            <SelectValue placeholder="Semua Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">Semua Status</SelectItem>
            {(Object.keys(STATUS_LABELS) as MappingWorkflowStatus[]).map((s) => (
              <SelectItem key={s} value={s}>
                {STATUS_LABELS[s]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button type="submit" size="sm" variant="outline" className="h-8">
          Cari
        </Button>
        <div className="ml-auto flex gap-2">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-8"
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label="Refresh data"
          >
            <RefreshCw
              className={cn("h-4 w-4", isFetching && "animate-spin")}
              aria-hidden="true"
            />
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-8"
            asChild
          >
            <a
              href={mappingApi.exportUrl({
                ...params,
                format: "xlsx",
              })}
              download
              aria-label="Export ke Excel"
            >
              <Download className="h-4 w-4 mr-1.5" aria-hidden="true" />
              Export
            </a>
          </Button>
        </div>
      </form>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <Table>
          <TableHeader className="sticky top-0 bg-background z-10">
            <TableRow>
              <TableHead className="w-40">Kode Event</TableHead>
              <TableHead>Nama Event</TableHead>
              <TableHead className="w-28">Kategori</TableHead>
              <TableHead className="w-24">Path</TableHead>
              <TableHead className="w-32">Status</TableHead>
              <TableHead className="w-24">Baris</TableHead>
              <TableHead className="w-36">Dibuat</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isFetching && rows.length === 0 ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 7 }).map((_, j) => (
                    <TableCell key={j}>
                      <div className="h-4 w-full animate-pulse rounded bg-muted" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className="py-12 text-center text-sm text-muted-foreground"
                >
                  Tidak ada data yang cocok
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow
                  key={row.id}
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() => router.push(`/jrnl/mapping/${row.id}`)}
                  role="link"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      router.push(`/jrnl/mapping/${row.id}`);
                    }
                  }}
                >
                  <TableCell className="font-mono text-xs font-medium">
                    {row.eventCode}
                  </TableCell>
                  <TableCell className="text-sm">{row.namaEvent}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {row.kategoriEvent}
                  </TableCell>
                  <TableCell>
                    <WorkflowPathBadge path={row.workflowPath} size="sm" />
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        STATUS_VARIANT[row.workflowStatus] as
                          | "default"
                          | "secondary"
                          | "destructive"
                          | "outline"
                      }
                      className="text-xs"
                    >
                      {STATUS_LABELS[row.workflowStatus]}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs">{row.detailCount}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {new Date(row.createdAt).toLocaleDateString("id-ID")}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between border-t px-6 py-3">
        <p className="text-xs text-muted-foreground">
          {pagination?.totalEstimate != null
            ? `Estimasi total: ${pagination.totalEstimate.toLocaleString("id-ID")} data`
            : ""}
        </p>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={cursorHistory.length === 0}
            onClick={handlePrev}
          >
            Sebelumnya
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={!pagination?.hasMore}
            onClick={handleNext}
          >
            Berikutnya
          </Button>
        </div>
      </div>
    </div>
  );
}

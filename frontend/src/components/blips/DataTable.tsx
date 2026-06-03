"use client";

import * as React from "react";
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import {
  ArrowUp,
  ArrowDown,
  ArrowUpDown,
  RefreshCw,
  Download,
  Search,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import type { Pagination, SortSpec } from "@/lib/api";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface FilterConfig {
  key: string;
  label: string;
}

export interface ActiveFilter {
  key: string;
  label: string;
  value: string;
  displayValue: string;
}

export interface DataTableProps<TData> {
  columns: ColumnDef<TData>[];
  data: TData[];
  pagination?: Pagination;
  isLoading?: boolean;
  isError?: boolean;
  error?: Error | null;
  // Sort
  sorting?: SortingState;
  onSortingChange?: (sorting: SortingState) => void;
  // Search
  searchValue?: string;
  onSearchChange?: (value: string) => void;
  searchPlaceholder?: string;
  // Active filter chips
  activeFilters?: ActiveFilter[];
  onRemoveFilter?: (key: string) => void;
  onClearFilters?: () => void;
  // Filter panel slot
  filterPanel?: React.ReactNode;
  // Pagination
  onNextPage?: () => void;
  onPrevPage?: () => void;
  canPrevPage?: boolean;
  pageNumber?: number;
  // Export
  onExport?: (format: "csv" | "xlsx") => void;
  // Actions
  onRefresh?: () => void;
  lastUpdated?: Date;
  // Create button
  createButton?: React.ReactNode;
  // Empty state
  emptyMessage?: string;
  // Error retry
  onRetry?: () => void;
}

// ---------------------------------------------------------------------------
// Sort header button
// ---------------------------------------------------------------------------

function SortHeader({
  label,
  sortKey,
  sorting,
  onToggle,
}: {
  label: string;
  sortKey: string;
  sorting: SortingState;
  onToggle: (key: string) => void;
}) {
  const current = sorting.find((s) => s.id === sortKey);
  return (
    <button
      className="group flex items-center gap-1 font-medium hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      onClick={() => onToggle(sortKey)}
      aria-label={`Urutkan berdasarkan ${label}`}
    >
      {label}
      {current?.desc === false ? (
        <ArrowUp className="h-3.5 w-3.5 text-primary" aria-hidden />
      ) : current?.desc === true ? (
        <ArrowDown className="h-3.5 w-3.5 text-primary" aria-hidden />
      ) : (
        <ArrowUpDown className="h-3.5 w-3.5 text-muted-foreground group-hover:text-foreground" aria-hidden />
      )}
    </button>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function DataTable<TData>({
  columns,
  data,
  pagination,
  isLoading = false,
  isError = false,
  error,
  sorting = [],
  onSortingChange,
  searchValue = "",
  onSearchChange,
  searchPlaceholder = "Cari...",
  activeFilters = [],
  onRemoveFilter,
  onClearFilters,
  filterPanel,
  onNextPage,
  onPrevPage,
  canPrevPage = false,
  pageNumber = 1,
  onExport,
  onRefresh,
  lastUpdated,
  createButton,
  emptyMessage = "Tidak ada data yang cocok.",
  onRetry,
}: DataTableProps<TData>) {
  const [localSearch, setLocalSearch] = React.useState(searchValue);
  const searchDebounce = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  React.useEffect(() => {
    setLocalSearch(searchValue);
  }, [searchValue]);

  const handleSearchInput = (val: string) => {
    setLocalSearch(val);
    if (searchDebounce.current) clearTimeout(searchDebounce.current);
    searchDebounce.current = setTimeout(() => {
      onSearchChange?.(val);
    }, 300);
  };

  const table = useReactTable({
    data,
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualSorting: true,
    manualPagination: true,
    state: { sorting },
    onSortingChange: (updater) => {
      const next =
        typeof updater === "function" ? updater(sorting) : updater;
      onSortingChange?.(next);
    },
  });

  const totalEstimate = pagination?.totalEstimate;
  const totalPages = totalEstimate
    ? Math.ceil(totalEstimate / (pagination?.limit ?? 50))
    : "?";

  return (
    <div className="space-y-4">
      {/* Action bar */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          {createButton}
        </div>
        <div className="flex items-center gap-2">
          {lastUpdated && (
            <span className="text-xs text-muted-foreground">
              Terakhir: {lastUpdated.toLocaleTimeString("id-ID", { hour: "2-digit", minute: "2-digit" })} WIB
            </span>
          )}
          {onRefresh && (
            <Button
              variant="ghost"
              size="sm"
              onClick={onRefresh}
              aria-label="Muat ulang data"
              title="Muat ulang data"
            >
              <RefreshCw className="h-4 w-4" aria-hidden />
            </Button>
          )}
          {onExport && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm">
                  <Download className="mr-1.5 h-4 w-4" aria-hidden />
                  Export
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => onExport("csv")}>
                  CSV
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => onExport("xlsx")}>
                  Excel (XLSX)
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </div>

      {/* Filter bar */}
      <div className="space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative flex-1 min-w-[200px] max-w-sm">
            <Search
              className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
              aria-hidden
            />
            <Input
              placeholder={searchPlaceholder}
              value={localSearch}
              onChange={(e) => handleSearchInput(e.target.value)}
              className="pl-9 pr-9"
              aria-label="Cari data"
            />
            {localSearch && (
              <button
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                onClick={() => handleSearchInput("")}
                aria-label="Hapus pencarian"
              >
                <X className="h-4 w-4" aria-hidden />
              </button>
            )}
          </div>
          {filterPanel}
        </div>

        {/* Filter chips */}
        {activeFilters.length > 0 && (
          <div className="flex flex-wrap items-center gap-2">
            {activeFilters.map((f) => (
              <span
                key={f.key}
                className="inline-flex items-center gap-1 rounded-md border bg-secondary px-2 py-1 text-xs font-medium"
              >
                {f.label}: {f.displayValue}
                <button
                  className="ml-0.5 rounded-sm hover:bg-muted"
                  onClick={() => onRemoveFilter?.(f.key)}
                  aria-label={`Hapus filter ${f.label}`}
                >
                  <X className="h-3 w-3" aria-hidden />
                </button>
              </span>
            ))}
            <button
              className="text-xs text-muted-foreground underline hover:text-foreground"
              onClick={onClearFilters}
            >
              Hapus semua
            </button>
          </div>
        )}
      </div>

      {/* Table */}
      <div className="overflow-x-auto rounded-lg border" aria-busy={isLoading}>
        <table className="w-full text-sm">
          <thead className="border-b bg-muted/50">
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id}>
                {hg.headers.map((header) => (
                  <th
                    key={header.id}
                    className="h-12 px-4 text-left align-middle font-medium text-muted-foreground"
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(
                          header.column.columnDef.header,
                          header.getContext(),
                        )}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {isError ? (
              <tr>
                <td
                  colSpan={columns.length}
                  className="px-4 py-10 text-center"
                >
                  <div className="space-y-2">
                    <p className="font-medium text-destructive">
                      Gagal memuat data
                    </p>
                    <p className="text-sm text-muted-foreground">
                      {error?.message ?? "Terjadi kesalahan saat menghubungi server."}
                    </p>
                    {onRetry && (
                      <Button variant="outline" size="sm" onClick={onRetry}>
                        Coba Lagi
                      </Button>
                    )}
                  </div>
                </td>
              </tr>
            ) : isLoading ? (
              Array.from({ length: 7 }).map((_, i) => (
                <tr key={i} className="border-b">
                  {columns.map((_, j) => (
                    <td key={j} className="px-4 py-3">
                      <Skeleton className="h-4 w-full" />
                    </td>
                  ))}
                </tr>
              ))
            ) : table.getRowModel().rows.length === 0 ? (
              <tr>
                <td
                  colSpan={columns.length}
                  className="px-4 py-12 text-center"
                >
                  <div className="space-y-2">
                    <p className="text-sm text-muted-foreground">
                      {emptyMessage}
                    </p>
                    {activeFilters.length > 0 && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={onClearFilters}
                      >
                        Hapus semua filter
                      </Button>
                    )}
                  </div>
                </td>
              </tr>
            ) : (
              table.getRowModel().rows.map((row) => (
                <tr
                  key={row.id}
                  className="h-12 border-b transition-colors hover:bg-muted/40"
                >
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} className="px-4 py-2 align-middle">
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext(),
                      )}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination footer */}
      {pagination && (
        <div className="flex items-center justify-between text-sm text-muted-foreground">
          <div className="flex items-center gap-2">
            <span>{data.length} dari ~{totalEstimate ?? "?"} total</span>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={onPrevPage}
              disabled={!canPrevPage || isLoading}
              aria-label="Halaman sebelumnya"
            >
              Prev
            </Button>
            <span className="text-sm">
              Halaman {pageNumber} / ~{totalPages}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={onNextPage}
              disabled={!pagination.hasMore || isLoading}
              aria-label="Halaman berikutnya"
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Re-export SortHeader for use in column definitions
// ---------------------------------------------------------------------------

export { SortHeader };

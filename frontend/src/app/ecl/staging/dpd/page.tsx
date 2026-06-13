"use client";

import * as React from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus, Pencil, Trash2 } from "lucide-react";

import { stagingApi } from "@/lib/api/staging.api";
import type { DpdRecord } from "@/lib/schemas/staging.schema";
import { dpdRecordFormSchema, type DpdRecordForm } from "@/lib/schemas/staging.schema";
import { DataTable } from "@/components/blips/DataTable";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { StageBadge } from "@/components/blips/StageBadge";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";

// ---------------------------------------------------------------------------
// Columns
// ---------------------------------------------------------------------------

function buildColumns(
  onEdit: (row: DpdRecord) => void,
  onDelete: (id: string) => void,
  canEdit: boolean,
): ColumnDef<DpdRecord>[] {
  return [
    {
      id: "kodeInstrumen",
      header: "Instrumen",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-sm font-medium">
          {row.original.kodeInstrumen ?? row.original.instrumenId.slice(0, 8)}
        </span>
      ),
    },
    {
      id: "periode",
      accessorKey: "periode",
      header: "Periode",
      enableSorting: true,
      cell: ({ row }) => <span className="text-sm">{row.original.periode}</span>,
    },
    {
      id: "dpdValue",
      accessorKey: "dpdValue",
      header: "DPD (hari)",
      enableSorting: true,
      cell: ({ row }) => (
        <span
          className={`text-sm font-mono font-medium ${
            row.original.dpdValue >= 90
              ? "text-red-600"
              : row.original.dpdValue >= 30
              ? "text-amber-600"
              : "text-foreground"
          }`}
        >
          {row.original.dpdValue}
        </span>
      ),
    },
    {
      id: "currentStage",
      header: "Stage Saat Ini",
      cell: ({ row }) => {
        const s = row.original.currentStage;
        if (!s) return <span className="text-muted-foreground text-xs">—</span>;
        const num = parseInt(s.replace("STAGE_", ""), 10) as 1 | 2 | 3;
        return <StageBadge stage={num} size="sm" />;
      },
    },
    {
      id: "source",
      accessorKey: "source",
      header: "Sumber",
      cell: ({ row }) => (
        <span className="text-xs">
          {row.original.source === "MANUAL" ? "Manual" : "APP-B"}
        </span>
      ),
    },
    {
      id: "catatan",
      header: "Catatan",
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground line-clamp-1 max-w-xs">
          {row.original.catatan ?? "—"}
        </span>
      ),
    },
    ...(canEdit
      ? [
          {
            id: "actions",
            header: "",
            cell: ({ row }: { row: { original: DpdRecord } }) => (
              <div className="flex gap-1">
                {row.original.source === "MANUAL" && (
                  <>
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={`Edit DPD periode ${row.original.periode}`}
                      onClick={() => onEdit(row.original)}
                    >
                      <Pencil className="h-4 w-4" aria-hidden="true" />
                    </Button>
                    <AlertDialog>
                      <AlertDialogTrigger asChild>
                        <Button
                          size="icon"
                          variant="ghost"
                          className="text-destructive"
                          aria-label={`Hapus DPD periode ${row.original.periode}`}
                        >
                          <Trash2 className="h-4 w-4" aria-hidden="true" />
                        </Button>
                      </AlertDialogTrigger>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>Hapus DPD Record?</AlertDialogTitle>
                          <AlertDialogDescription>
                            Data DPD periode {row.original.periode} akan dihapus
                            (soft-delete). Tindakan ini bisa memengaruhi staging
                            instrumen.
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>Batal</AlertDialogCancel>
                          <AlertDialogAction
                            className="bg-destructive text-destructive-foreground"
                            onClick={() => onDelete(row.original.id)}
                          >
                            Hapus
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
                  </>
                )}
              </div>
            ),
          } as ColumnDef<DpdRecord>,
        ]
      : []),
  ];
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function DpdEntryPage() {
  const queryClient = useQueryClient();
  const { can } = usePermissions();

  const [sheetOpen, setSheetOpen] = React.useState(false);
  const [editingRecord, setEditingRecord] = React.useState<DpdRecord | null>(null);
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [pageNumber, setPageNumber] = React.useState(1);
  const [prevCursors, setPrevCursors] = React.useState<string[]>([]);
  const [searchValue, setSearchValue] = React.useState("");
  const [sourceFilter, setSourceFilter] = React.useState("");

  const params = {
    limit: 50,
    sort: "periode:desc",
    ...(searchValue && { q: searchValue }),
    ...(sourceFilter && { "filter[source]": sourceFilter }),
    ...(cursor && { cursor }),
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["dpd-records", params],
    queryFn: () => stagingApi.listDpdRecords(params),
  });

  const form = useForm<DpdRecordForm>({
    resolver: zodResolver(dpdRecordFormSchema),
    defaultValues: { instrumenId: "", periode: "", dpdValue: 0, catatan: "" },
  });

  React.useEffect(() => {
    if (editingRecord) {
      form.reset({
        instrumenId: editingRecord.instrumenId,
        periode: editingRecord.periode,
        dpdValue: editingRecord.dpdValue,
        catatan: editingRecord.catatan ?? "",
      });
    } else {
      form.reset({ instrumenId: "", periode: "", dpdValue: 0, catatan: "" });
    }
  }, [editingRecord, form]);

  const createMutation = useMutation({
    mutationFn: (data: DpdRecordForm) =>
      stagingApi.createDpdRecord(
        {
          instrumenId: data.instrumenId,
          periode: data.periode,
          dpdValue: data.dpdValue,
          source: "MANUAL",
          catatan: data.catatan,
        },
        uuidv4(),
      ),
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: ["dpd-records"] });
      notify.success(`DPD record untuk periode ${res.data.periode} berhasil disimpan.`);
      setSheetOpen(false);
      form.reset();
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: DpdRecordForm }) =>
      stagingApi.updateDpdRecord(
        id,
        {
          dpdValue: data.dpdValue,
          catatan: data.catatan,
        },
        uuidv4(),
      ),
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: ["dpd-records"] });
      notify.success(`DPD record periode ${res.data.periode} berhasil diperbarui.`);
      setSheetOpen(false);
      setEditingRecord(null);
      form.reset();
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => stagingApi.deleteDpdRecord(id, uuidv4()),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["dpd-records"] });
      notify.success("DPD record berhasil dihapus.");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const onSubmit = (values: DpdRecordForm) => {
    if (editingRecord) {
      updateMutation.mutate({ id: editingRecord.id, data: values });
    } else {
      createMutation.mutate(values);
    }
  };

  const handleEdit = (record: DpdRecord) => {
    setEditingRecord(record);
    setSheetOpen(true);
  };

  const handleNew = () => {
    setEditingRecord(null);
    setSheetOpen(true);
  };

  const activeFilters: ActiveFilter[] = [];
  if (sourceFilter) {
    activeFilters.push({
      key: "source",
      label: "Sumber",
      value: sourceFilter,
      displayValue: sourceFilter === "MANUAL" ? "Manual" : "APP-B",
    });
  }

  const handleExport = (format: "csv" | "xlsx") => {
    const url = `/api/v1/ecl/dpd/records/export?format=${format}`;
    window.open(url, "_blank");
  };

  const handleNextPage = () => {
    const next = data?.pagination?.nextCursor ?? null;
    if (next) {
      setPrevCursors((p) => [...p, cursor ?? ""]);
      setCursor(next);
      setPageNumber((n) => n + 1);
    }
  };

  const handlePrevPage = () => {
    const prev = prevCursors[prevCursors.length - 1] ?? null;
    setPrevCursors((p) => p.slice(0, -1));
    setCursor(prev);
    setPageNumber((n) => Math.max(1, n - 1));
  };

  const columns = buildColumns(handleEdit, (id) => deleteMutation.mutate(id), can("ecl_dpd.write"));
  const isMutating = createMutation.isPending || updateMutation.isPending;

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">DPD Entry</h1>
          <p className="text-sm text-muted-foreground">
            Input manual DPD (Days Past Due) per instrumen per periode.
            DPD ≥ 30 hari dapat memicu migrasi ke Stage 2.
            DPD ≥ 90 hari memicu Stage 3.
          </p>
        </div>
        {can("ecl_dpd.write") && (
          <Button onClick={handleNew}>
            <Plus className="h-4 w-4 mr-1" aria-hidden="true" />
            Input DPD
          </Button>
        )}
      </div>

      <DataTable
        columns={columns}
        data={data?.data ?? []}
        pagination={data?.pagination}
        isLoading={isLoading}
        isError={isError}
        searchValue={searchValue}
        onSearchChange={setSearchValue}
        searchPlaceholder="Cari instrumen atau periode..."
        activeFilters={activeFilters}
        onRemoveFilter={(key) => { if (key === "source") setSourceFilter(""); }}
        onClearFilters={() => setSourceFilter("")}
        onExport={handleExport}
        onRefresh={() => void refetch()}
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageNumber > 1}
        pageNumber={pageNumber}
        emptyMessage="Belum ada data DPD. Klik 'Input DPD' untuk menambah."
        onRetry={() => void refetch()}
      />

      {/* Sheet form */}
      <Sheet open={sheetOpen} onOpenChange={(open) => { setSheetOpen(open); if (!open) setEditingRecord(null); }}>
        <SheetContent side="right" className="w-full sm:max-w-md">
          <SheetHeader>
            <SheetTitle>
              {editingRecord ? "Edit DPD Record" : "Input DPD Baru"}
            </SheetTitle>
            <SheetDescription>
              {editingRecord
                ? `Edit DPD untuk instrumen pada periode ${editingRecord.periode}.`
                : "Masukkan data DPD manual untuk satu instrumen per periode."}
            </SheetDescription>
          </SheetHeader>

          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(onSubmit)}
              className="mt-4 space-y-4"
            >
              <FormField
                control={form.control}
                name="instrumenId"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>ID Instrumen</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="UUID instrumen"
                        {...field}
                        disabled={!!editingRecord}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="periode"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Periode (YYYY-MM-DD)</FormLabel>
                    <FormControl>
                      <Input
                        type="date"
                        {...field}
                        disabled={!!editingRecord}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="dpdValue"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>DPD (hari)</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min={0}
                        {...field}
                        onChange={(e) =>
                          field.onChange(parseInt(e.target.value, 10) || 0)
                        }
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="catatan"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Catatan (opsional)</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={2}
                        placeholder="Keterangan tambahan..."
                        {...field}
                        value={field.value ?? ""}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="flex gap-2 pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => { setSheetOpen(false); setEditingRecord(null); }}
                  disabled={isMutating}
                >
                  Batal
                </Button>
                <Button type="submit" disabled={isMutating}>
                  {isMutating ? (
                    <>
                      <span
                        className="h-4 w-4 mr-2 border-2 border-current border-t-transparent rounded-full animate-spin"
                        aria-hidden="true"
                      />
                      Menyimpan...
                    </>
                  ) : editingRecord ? (
                    "Simpan Perubahan"
                  ) : (
                    "Simpan DPD"
                  )}
                </Button>
              </div>
            </form>
          </Form>
        </SheetContent>
      </Sheet>
    </div>
  );
}

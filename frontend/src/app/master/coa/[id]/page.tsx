"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { Pencil, MoreHorizontal, ChevronRight } from "lucide-react";
import { v4 as uuidv4 } from "uuid";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { MakerReviewerApproverPanel } from "@/components/blips/MakerReviewerApproverPanel";

import { coaApi } from "@/lib/api/coa.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { CoADetail, TipeAkun } from "@/lib/schemas/coa.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Detail field row
// ---------------------------------------------------------------------------

function DetailRow({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
        {label}
      </span>
      <span className="text-sm font-medium">{value ?? "—"}</span>
    </div>
  );
}

function formatDt(iso: string | null | undefined): string {
  if (!iso) return "—";
  try {
    return format(parseISO(iso), "dd MMM yyyy, HH:mm 'WIB'");
  } catch {
    return iso;
  }
}

// ---------------------------------------------------------------------------
// Tipe badge
// ---------------------------------------------------------------------------

const TIPE_COLORS: Record<TipeAkun, string> = {
  ASET: "bg-blue-50 text-blue-700 border-blue-200",
  LIABILITAS: "bg-red-50 text-red-700 border-red-200",
  EKUITAS: "bg-purple-50 text-purple-700 border-purple-200",
  PENDAPATAN: "bg-green-50 text-green-700 border-green-200",
  BEBAN: "bg-amber-50 text-amber-700 border-amber-200",
  KONTINJEN: "bg-gray-50 text-gray-700 border-gray-200",
};

// ---------------------------------------------------------------------------
// Ancestor breadcrumb
// ---------------------------------------------------------------------------

function AncestorBreadcrumb({
  ancestors,
  currentKode,
}: {
  ancestors: CoADetail["ancestors"];
  currentKode: string;
}) {
  if (!ancestors || ancestors.length === 0) return null;
  return (
    <div className="flex items-center flex-wrap gap-1 text-xs text-muted-foreground">
      <span className="font-medium">Hierarki:</span>
      {ancestors.map((a, i) => (
        <React.Fragment key={a.id}>
          {i > 0 && <ChevronRight className="h-3 w-3" aria-hidden />}
          <Link
            href={`/master/coa/${a.id}`}
            className="hover:underline font-mono"
          >
            {a.kodeAkun}
          </Link>
        </React.Fragment>
      ))}
      <ChevronRight className="h-3 w-3" aria-hidden />
      <span className="font-mono font-medium text-foreground">{currentKode}</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function CoADetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["coa", id],
    queryFn: () => coaApi.get(id),
    enabled: !!id,
  });

  const item: CoADetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["coa", id] });
    void queryClient.invalidateQueries({ queryKey: ["coa"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await coaApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(
        `Akun ${item.kodeAkun} berhasil disubmit untuk review.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleReview = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      const res = await coaApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Akun ${item.kodeAkun} berhasil di-review. Status: ${res.data.currentState}.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleApprove = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await coaApi.approve(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Akun ${item.kodeAkun} berhasil disetujui dan sekarang aktif.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleReject = async (comment: string) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await coaApi.reject(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.warning(
        `Akun ${item.kodeAkun} dikembalikan ke maker dengan alasan penolakan.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!item) return;
    setDeleting(true);
    try {
      await coaApi.delete(id, uuidv4());
      notify.destructive(`Akun ${item.kodeAkun} berhasil dihapus.`);
      router.push("/master/coa");
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setDeleting(false);
      setDeleteOpen(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Loading
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
          <div className="space-y-4">
            {Array.from({ length: 10 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
          <Skeleton className="h-64 w-full" />
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">
          Gagal memuat data akun.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/coa">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("coa") && isDraft;
  const canDelete = perms.canDelete("coa") && isDraft;
  const canSubmit = perms.canSubmit("coa") && isDraft;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/coa" className="hover:underline">
          Chart of Accounts
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{item.kodeAkun}</span>
      </nav>

      {/* Ancestor hierarchy breadcrumb */}
      {item.ancestors && item.ancestors.length > 0 && (
        <AncestorBreadcrumb
          ancestors={item.ancestors}
          currentKode={item.kodeAkun}
        />
      )}

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3 flex-wrap">
          <h1 className="text-2xl font-semibold">
            <code className="font-mono">{item.kodeAkun}</code>
            <span className="ml-2 text-muted-foreground font-normal">
              {item.namaAkun}
            </span>
          </h1>
          <span
            className={cn(
              "inline-flex items-center rounded border px-1.5 py-0.5 text-xs font-medium",
              TIPE_COLORS[item.tipeAkun],
            )}
          >
            {item.tipeAkun}
          </span>
          <WorkflowStatusBadge status={item.workflowStatus} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/coa/${id}/edit`}>
                <Pencil className="mr-1.5 h-4 w-4" aria-hidden />
                Edit
              </Link>
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="h-9 w-9"
                aria-label="Aksi lainnya"
              >
                <MoreHorizontal className="h-4 w-4" aria-hidden />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {canSubmit && (
                <DropdownMenuItem onClick={handleSubmit}>
                  {item.workflowStatus === "RETURNED"
                    ? "Kirim Ulang untuk Review"
                    : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/coa/${id}/history`}>
                  Lihat Riwayat Audit
                </Link>
              </DropdownMenuItem>
              {canDelete && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive"
                    onClick={() => setDeleteOpen(true)}
                  >
                    Hapus
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* 2-col layout */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
        {/* Left */}
        <div className="space-y-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Detail Akun
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Kode Akun"
                  value={
                    <code className="font-mono font-bold">{item.kodeAkun}</code>
                  }
                />
                <DetailRow label="Nama Akun" value={item.namaAkun} />
                <DetailRow label="Tipe Akun" value={item.tipeAkun} />
                <DetailRow label="Sub Tipe" value={item.subTipeAkun} />
                <DetailRow
                  label="Kategori Investasi"
                  value={item.kategoriInvestasi}
                />
                <DetailRow label="Mata Uang Native" value={item.matauangNative} />
                <DetailRow label="Posisi Normal" value={item.posisiNormal} />
                <DetailRow
                  label="Status Aktif"
                  value={
                    <span
                      className={
                        item.aktifFlag
                          ? "text-green-700 font-medium"
                          : "text-muted-foreground"
                      }
                    >
                      {item.aktifFlag ? "Ya" : "Tidak"}
                    </span>
                  }
                />
                {item.parentKodeAkun && (
                  <DetailRow
                    label="Akun Induk"
                    value={
                      item.parentAkunId ? (
                        <Link
                          href={`/master/coa/${item.parentAkunId}`}
                          className="hover:underline font-mono text-primary"
                        >
                          {item.parentKodeAkun}
                          {item.parentNamaAkun && (
                            <span className="ml-1 font-normal text-foreground">
                              — {item.parentNamaAkun}
                            </span>
                          )}
                        </Link>
                      ) : (
                        item.parentKodeAkun
                      )
                    }
                  />
                )}
                <DetailRow label="Sumber CoA" value={item.sumberCoa} />
                <DetailRow
                  label="Tgl Mulai Aktif"
                  value={item.tanggalMulaiAktif}
                />
              </div>
            </CardContent>
          </Card>

          {/* Metadata */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Metadata
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow label="Dibuat oleh" value={item.createdBy} />
                <DetailRow
                  label="Dibuat pada"
                  value={formatDt(item.createdAt)}
                />
                <DetailRow label="Diperbarui oleh" value={item.updatedBy} />
                <DetailRow
                  label="Diperbarui pada"
                  value={formatDt(item.updatedAt)}
                />
                <DetailRow label="Versi" value={item.rowVersion} />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/coa/${id}/history`}
            className="text-sm text-primary hover:underline"
          >
            Lihat Riwayat Audit &rarr;
          </Link>
        </div>

        {/* Right: workflow */}
        <div>
          <Card>
            <CardContent className="pt-6">
              {item.workflow ? (
                <MakerReviewerApproverPanel
                  workflowData={item.workflow}
                  currentUserId={perms.userId}
                  entityStatus={item.workflowStatus}
                  submitting={workflowSubmitting}
                  onSubmit={handleSubmit}
                  onReview={handleReview}
                  onApprove={handleApprove}
                  onReject={handleReject}
                />
              ) : (
                <div className="space-y-2">
                  <p className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                    Proses Persetujuan
                  </p>
                  <Separator />
                  <p className="text-sm text-muted-foreground">
                    Data belum disubmit ke workflow.
                  </p>
                  {canSubmit && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={workflowSubmitting}
                      onClick={handleSubmit}
                    >
                      {workflowSubmitting ? "Memproses..." : "Kirim untuk Review"}
                    </Button>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Delete confirmation */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hapus Akun {item.kodeAkun}?</DialogTitle>
            <DialogDescription>
              Akun <strong>{item.namaAkun}</strong> ({item.kodeAkun}) akan
              dihapus dari sistem (soft-delete).
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteOpen(false)}
              disabled={deleting}
            >
              Batal
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleting}
            >
              {deleting ? "Menghapus..." : "Hapus"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

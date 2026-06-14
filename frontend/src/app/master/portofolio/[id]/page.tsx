"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { Pencil, MoreHorizontal } from "lucide-react";
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

import { portofolioApi } from "@/lib/api/portofolio.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  BM_CATEGORY_LABEL,
  BM_CATEGORY_PSAK71,
  type PortofolioDetail,
} from "@/lib/schemas/portofolio.schema";
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

// BM colors reused from list
const BM_COLORS: Record<string, string> = {
  HTC: "bg-green-50 text-green-700 border-green-200",
  HTCS: "bg-blue-50 text-blue-700 border-blue-200",
  OTHER: "bg-orange-50 text-orange-700 border-orange-200",
};

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function PortofolioDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["portofolio", id],
    queryFn: () => portofolioApi.get(id),
    enabled: !!id,
  });

  const item: PortofolioDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["portofolio", id] });
    void queryClient.invalidateQueries({ queryKey: ["portofolio"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await portofolioApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(
        `Portofolio ${item.kodePortofolio} berhasil disubmit untuk review. Menunggu Finance Controller.`,
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
      const res = await portofolioApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Portofolio ${item.kodePortofolio} berhasil di-review. Status: ${res.data.currentState}.`,
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
      await portofolioApi.approve(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Portofolio ${item.kodePortofolio} berhasil disetujui. Sekarang aktif dan dapat digunakan.`,
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
      await portofolioApi.reject(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.warning(
        `Portofolio ${item.kodePortofolio} dikembalikan ke maker dengan alasan penolakan.`,
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
      await portofolioApi.delete(id, uuidv4());
      notify.destructive(`Portofolio ${item.kodePortofolio} berhasil dihapus.`);
      router.push("/master/portofolio");
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
  // Loading / error states
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
          <div className="space-y-4">
            {Array.from({ length: 8 }).map((_, i) => (
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
          Gagal memuat data portofolio {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/portofolio">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("portofolio") && isDraft;
  const canDelete = perms.canDelete("portofolio") && isDraft;
  const canSubmit = perms.canSubmit("portofolio") && isDraft;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/portofolio" className="hover:underline">
          Portofolio
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">
          {item.kodePortofolio}
        </span>
      </nav>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-2xl font-semibold">
            Portofolio: {item.nama}
          </h1>
          <code className="rounded bg-muted px-2 py-0.5 font-mono text-sm">
            {item.kodePortofolio}
          </code>
          {/* BM badge */}
          <span
            className={cn(
              "inline-flex items-center rounded border px-2 py-0.5 text-xs font-medium",
              BM_COLORS[item.bmCategoryDefault] ??
                "bg-muted text-muted-foreground",
            )}
            title={
              BM_CATEGORY_PSAK71[
                item.bmCategoryDefault as keyof typeof BM_CATEGORY_PSAK71
              ]
            }
          >
            {item.bmCategoryDefault}
          </span>
          <WorkflowStatusBadge status={item.workflowStatus} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/portofolio/${id}/edit`}>
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
                <DropdownMenuItem
                  onClick={handleSubmit}
                  disabled={workflowSubmitting}
                >
                  {item.workflowStatus === "RETURNED"
                    ? "Kirim Ulang untuk Review"
                    : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/portofolio/${id}/history`}>
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
        {/* Left: detail cards */}
        <div className="space-y-6">
          {/* Identitas */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Identitas Portofolio
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Kode Portofolio"
                  value={
                    <code className="font-mono font-bold">
                      {item.kodePortofolio}
                    </code>
                  }
                />
                <DetailRow label="Nama" value={item.nama} />
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
                <DetailRow
                  label="Periode Review Terakhir"
                  value={item.periodeReviewTerakhir ?? "—"}
                />
              </div>
              <div className="mt-4">
                <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide block mb-1">
                  Tujuan Pengelolaan
                </span>
                <p className="text-sm whitespace-pre-wrap">
                  {item.tujuanPengelolaan}
                </p>
              </div>
            </CardContent>
          </Card>

          {/* Business Model */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Klasifikasi Business Model (PSAK 71)
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="BM Category Default"
                  value={
                    <span className="flex items-center gap-2">
                      <span
                        className={cn(
                          "inline-flex items-center rounded border px-2 py-0.5 text-xs font-medium",
                          BM_COLORS[item.bmCategoryDefault] ??
                            "bg-muted text-muted-foreground",
                        )}
                      >
                        {item.bmCategoryDefault}
                      </span>
                      <span className="text-muted-foreground text-xs">
                        {BM_CATEGORY_LABEL[item.bmCategoryDefault]}
                      </span>
                    </span>
                  }
                />
                <DetailRow
                  label="Klasifikasi PSAK 71"
                  value={
                    <span className="text-primary font-medium">
                      {BM_CATEGORY_PSAK71[item.bmCategoryDefault]}
                    </span>
                  }
                />
                <DetailRow
                  label="Benchmark"
                  value={item.benchmark ?? "—"}
                />
                <DetailRow
                  label="Kompensasi Manager Basis"
                  value={item.kompensasiManagerBasis ?? "—"}
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
            href={`/master/portofolio/${id}/history`}
            className="text-sm text-primary hover:underline"
          >
            Lihat Riwayat Audit &rarr;
          </Link>
        </div>

        {/* Right: workflow panel */}
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
                      {workflowSubmitting
                        ? "Memproses..."
                        : "Kirim untuk Review"}
                    </Button>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Delete confirmation dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hapus Portofolio {item.kodePortofolio}?</DialogTitle>
            <DialogDescription>
              Portofolio <strong>{item.nama}</strong> ({item.kodePortofolio})
              akan dihapus dari sistem (soft-delete).
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

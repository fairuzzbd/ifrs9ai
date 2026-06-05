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

import { periodeBukuApi } from "@/lib/api/periode-buku.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { PeriodeBukuDetail, StatusPeriode } from "@/lib/schemas/periode-buku.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Helpers
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

function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  try {
    return format(parseISO(iso), "dd MMM yyyy");
  } catch {
    return iso ?? "—";
  }
}

function StatusPeriodeBadge({ status }: { status: StatusPeriode }) {
  const config: Record<StatusPeriode, { label: string; className: string }> = {
    OPEN: {
      label: "Buka",
      className: "bg-green-50 text-green-700 border-green-200",
    },
    SOFT_CLOSED: {
      label: "Soft-Close",
      className: "bg-amber-50 text-amber-700 border-amber-200",
    },
    CLOSED: {
      label: "Ditutup",
      className: "bg-red-50 text-red-700 border-red-200",
    },
  };
  const c = config[status] ?? {
    label: status,
    className: "bg-muted text-muted-foreground",
  };
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium",
        c.className,
      )}
    >
      {c.label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function PeriodeBukuDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["periode-buku", id],
    queryFn: () => periodeBukuApi.get(id),
    enabled: !!id,
  });

  const item: PeriodeBukuDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["periode-buku", id] });
    void queryClient.invalidateQueries({ queryKey: ["periode-buku"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await periodeBukuApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(
        `Periode ${item.periodeIdKode} berhasil disubmit untuk review.`,
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
      const res = await periodeBukuApi.review(
        id,
        {
          comment,
          signatureMethod: "JWT_STANDARD",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.success(
        `Periode ${item.periodeIdKode} berhasil di-review. Status: ${res.data.currentState}.`,
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
      const res = await periodeBukuApi.approve(
        id,
        {
          comment,
          // Step-up MFA required per DEC-027 — ApprovalWithSignature passes JWT_STEP_UP
          signatureMethod: "JWT_STEP_UP",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.success(
        `Periode ${item.periodeIdKode} berhasil disetujui. Status workflow: ${res.data.currentState}.`,
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
      await periodeBukuApi.reject(
        id,
        {
          comment,
          signatureMethod: "JWT_STANDARD",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.warning(
        `Periode ${item.periodeIdKode} dikembalikan ke maker dengan alasan penolakan.`,
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
      await periodeBukuApi.delete(id, uuidv4());
      notify.destructive(`Periode ${item.periodeIdKode} berhasil dihapus.`);
      router.push("/master/periode-buku");
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
          Gagal memuat data periode buku.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/periode-buku">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("periode") && isDraft;
  const canDelete = perms.canDelete("periode") && isDraft;
  const canSubmit = perms.canSubmit("periode") && isDraft;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/periode-buku" className="hover:underline">
          Periode Buku
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">
          {item.periodeIdKode}
        </span>
      </nav>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3 flex-wrap">
          <h1 className="text-2xl font-semibold">
            Periode:{" "}
            <code className="font-mono">{item.periodeIdKode}</code>
          </h1>
          <StatusPeriodeBadge status={item.statusPeriode} />
          <WorkflowStatusBadge
            status={item.workflowStatus as Parameters<typeof WorkflowStatusBadge>[0]["status"]}
          />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/periode-buku/${id}/edit`}>
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
                  onClick={() => void handleSubmit()}
                  disabled={workflowSubmitting}
                >
                  {item.workflowStatus === "RETURNED"
                    ? "Kirim Ulang untuk Review"
                    : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/periode-buku/${id}/history`}>
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

      {/* Body: 2-col layout */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
        {/* Left: detail */}
        <div className="space-y-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Identitas Periode
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Kode Periode"
                  value={
                    <code className="font-mono font-bold">
                      {item.periodeIdKode}
                    </code>
                  }
                />
                <DetailRow label="Tipe Periode" value={item.tipePeriode} />
                <DetailRow
                  label="Tahun Buku"
                  value={String(item.tahunBuku)}
                />
                {item.tipePeriode === "BULANAN" && (
                  <DetailRow
                    label="Bulan"
                    value={
                      item.bulan !== null
                        ? `Bulan ${String(item.bulan).padStart(2, "0")}`
                        : null
                    }
                  />
                )}
                {item.tipePeriode === "TRIWULANAN" && (
                  <DetailRow
                    label="Triwulan"
                    value={item.triwulan !== null ? `Q${item.triwulan}` : null}
                  />
                )}
                <DetailRow
                  label="Tanggal Mulai"
                  value={formatDate(item.tanggalMulai)}
                />
                <DetailRow
                  label="Tanggal Akhir"
                  value={formatDate(item.tanggalAkhir)}
                />
                <DetailRow
                  label="Status Periode"
                  value={<StatusPeriodeBadge status={item.statusPeriode} />}
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
                <DetailRow
                  label="Dibuat oleh"
                  value={item.createdBy ?? "—"}
                />
                <DetailRow
                  label="Dibuat pada"
                  value={formatDt(item.createdAt)}
                />
                <DetailRow
                  label="Diperbarui oleh"
                  value={item.updatedBy ?? "—"}
                />
                <DetailRow
                  label="Diperbarui pada"
                  value={formatDt(item.updatedAt)}
                />
                <DetailRow
                  label="Versi"
                  value={String(item.rowVersion)}
                />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/periode-buku/${id}/history`}
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
                  workflowData={item.workflow as Parameters<typeof MakerReviewerApproverPanel>[0]["workflowData"]}
                  currentUserId={perms.userId}
                  entityStatus={item.workflowStatus as Parameters<typeof MakerReviewerApproverPanel>[0]["entityStatus"]}
                  submitting={workflowSubmitting}
                  onSubmit={() => void handleSubmit()}
                  onReview={(comment) => void handleReview(comment)}
                  onApprove={(comment) => void handleApprove(comment)}
                  onReject={(comment) => void handleReject(comment)}
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
                      onClick={() => void handleSubmit()}
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

      {/* Delete dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hapus Periode {item.periodeIdKode}?</DialogTitle>
            <DialogDescription>
              Periode <strong>{item.periodeIdKode}</strong> akan dihapus dari
              sistem (soft-delete).
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
              onClick={() => void handleDelete()}
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

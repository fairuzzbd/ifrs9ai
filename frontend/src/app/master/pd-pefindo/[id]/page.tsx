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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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

import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { MakerReviewerApproverPanel } from "@/components/blips/MakerReviewerApproverPanel";

import { pdPefindoApi } from "@/lib/api/pd-pefindo.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { PDPefindoDetail, WorkflowStatusData } from "@/lib/schemas/pd-pefindo.schema";

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
    return iso;
  }
}

function formatPD(val: string | null | undefined): string {
  if (!val) return "—";
  const n = parseFloat(val);
  if (isNaN(n)) return val;
  return `${(n * 100).toFixed(4)}% (${val})`;
}

// ---------------------------------------------------------------------------
// Adapter: map PDPefindoDetail.workflow → WorkflowStatus shape expected by panel
// ---------------------------------------------------------------------------

function toWorkflowStatus(wf: WorkflowStatusData | null) {
  if (!wf) return null;
  return {
    currentState: wf.currentState,
    workflowEyes: wf.workflowEyes,
    makerId: wf.makerId,
    reviewerId: wf.reviewerId,
    approverId: wf.approverId,
    history: wf.history.map((h) => ({
      action: h.action as "SUBMIT" | "REVIEW" | "APPROVE" | "REJECT",
      userId: h.userId,
      username: h.username,
      role: h.role,
      signedAt: h.signedAt,
      signatureHash: h.signatureHash,
      comment: h.comment,
    })),
  };
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function PDPefindoDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["pd-pefindo", id],
    queryFn: () => pdPefindoApi.get(id),
    enabled: !!id,
  });

  const item: PDPefindoDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["pd-pefindo", id] });
    void queryClient.invalidateQueries({ queryKey: ["pd-pefindo"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await pdPefindoApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(
        `PD Pefindo ${item.rating} berhasil disubmit untuk review Risk Officer.`,
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
      const res = await pdPefindoApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `PD Pefindo ${item.rating} berhasil di-review. Status: ${res.data.currentState}.`,
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
      // Determine if this is approve (step 3) or approve2 (step 4 for 6-eyes)
      const isApprove2 = item.workflowStatus === "PENDING_APPROVAL_2";
      const fn = isApprove2 ? pdPefindoApi.approve2 : pdPefindoApi.approve;
      const res = await fn(
        id,
        { comment, signatureMethod: "JWT_STEP_UP", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `PD Pefindo ${item.rating} berhasil disetujui${isApprove2 ? " (final)." : "."} Status: ${res.data.currentState}.`,
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
      await pdPefindoApi.reject(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.warning(
        `PD Pefindo ${item.rating} dikembalikan ke maker dengan alasan penolakan.`,
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
      await pdPefindoApi.delete(id, uuidv4());
      notify.destructive(`PD Pefindo ${item.rating} berhasil dihapus.`);
      router.push("/master/pd-pefindo");
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
          Gagal memuat data PD Pefindo {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/pd-pefindo">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("ecl_parameter") && isDraft;
  const canDelete = perms.canDelete("ecl_parameter") && isDraft;
  const canSubmit = perms.canSubmit("ecl_parameter") && isDraft;

  const workflowData = toWorkflowStatus(item.workflow);

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/pd-pefindo" className="hover:underline">
          PD Pefindo
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{item.rating}</span>
      </nav>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">
            PD Pefindo — <code className="font-mono">{item.rating}</code>
          </h1>
          <WorkflowStatusBadge
            status={
              item.workflowStatus as import("@/lib/schemas/mata-uang.schema").MasterWorkflowState
            }
          />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/pd-pefindo/${id}/edit`}>
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
                <Link href={`/master/pd-pefindo/${id}/history`}>
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
        {/* Left: detail */}
        <div className="space-y-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Identitas & Periode
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Rating Pefindo"
                  value={
                    <code className="font-mono font-bold text-sm">
                      {item.rating}
                    </code>
                  }
                />
                <DetailRow label="Sumber" value={item.sumber} />
                <DetailRow
                  label="Tanggal Publikasi"
                  value={formatDate(item.tanggalPublikasi)}
                />
                <DetailRow
                  label="Periode Berlaku Dari"
                  value={formatDate(item.periodeBerlakuDari)}
                />
                <DetailRow
                  label="Periode Berlaku Sampai"
                  value={formatDate(item.periodeBerlakuSampai)}
                />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Nilai PD (Probability of Default)
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <DetailRow
                  label="PD 12 Bulan (Stage 1)"
                  value={
                    <span className="font-mono">
                      {formatPD(item.pd12Month)}
                    </span>
                  }
                />
                <DetailRow
                  label="PD Lifetime 3 Tahun"
                  value={
                    <span className="font-mono">
                      {formatPD(item.pdLifetime3Y)}
                    </span>
                  }
                />
                <DetailRow
                  label="PD Lifetime 5 Tahun"
                  value={
                    <span className="font-mono">
                      {formatPD(item.pdLifetime5Y)}
                    </span>
                  }
                />
                <DetailRow
                  label="PD Lifetime 7 Tahun"
                  value={
                    <span className="font-mono">
                      {formatPD(item.pdLifetime7Y)}
                    </span>
                  }
                />
                <DetailRow
                  label="PD Lifetime 10 Tahun"
                  value={
                    <span className="font-mono">
                      {formatPD(item.pdLifetime10Y)}
                    </span>
                  }
                />
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Metadata
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow label="Dibuat oleh" value={item.createdBy ?? "—"} />
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
                <DetailRow label="Versi" value={item.rowVersion} />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/pd-pefindo/${id}/history`}
            className="text-sm text-primary hover:underline"
          >
            Lihat Riwayat Audit &rarr;
          </Link>
        </div>

        {/* Right: workflow panel (6-eyes) */}
        <div>
          <Card>
            <CardContent className="pt-6">
              {workflowData ? (
                <MakerReviewerApproverPanel
                  workflowData={workflowData}
                  currentUserId={perms.userId}
                  entityStatus={
                    item.workflowStatus as import("@/lib/schemas/mata-uang.schema").MasterWorkflowState
                  }
                  submitting={workflowSubmitting}
                  onSubmit={handleSubmit}
                  onReview={handleReview}
                  onApprove={handleApprove}
                  onReject={handleReject}
                />
              ) : (
                <div className="space-y-2">
                  <p className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                    Proses Persetujuan (6-eyes)
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

      {/* Delete dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              Hapus PD Pefindo — {item.rating}?
            </DialogTitle>
            <DialogDescription>
              Record PD untuk rating <strong>{item.rating}</strong> (periode{" "}
              {item.periodeBerlakuDari}) akan dihapus (soft-delete).
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

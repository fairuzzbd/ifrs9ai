"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { Pencil, MoreHorizontal, Info } from "lucide-react";
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

import { impactPdApi } from "@/lib/api/impact-pd.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { ImpactPdDetail } from "@/lib/schemas/impact-pd.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
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
// Page
// ---------------------------------------------------------------------------

export default function ImpactPdDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["impact-pd", id],
    queryFn: () => impactPdApi.get(id),
    enabled: !!id,
  });

  const item: ImpactPdDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["impact-pd", id] });
    void queryClient.invalidateQueries({ queryKey: ["impact-pd"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await impactPdApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(`Impact PD berhasil disubmit untuk review.`);
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
      const res = await impactPdApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(`Impact PD berhasil di-review. Status: ${res.data.currentState}.`);
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
      const res = await impactPdApi.approve(
        id,
        { comment, signatureMethod: "JWT_STEP_UP", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Impact PD berhasil disetujui (Approve 1). Status: ${res.data.currentState}.`,
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

  const handleApprove2 = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      const res = await impactPdApi.approve2(
        id,
        { comment, signatureMethod: "JWT_STEP_UP", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Impact PD berhasil disetujui final (Approve 2). Status: ${res.data.currentState}.`,
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
      await impactPdApi.reject(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.warning(`Impact PD dikembalikan ke maker dengan alasan penolakan.`);
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
      await impactPdApi.delete(id, uuidv4());
      notify.destructive(`Impact PD berhasil dihapus.`);
      router.push("/master/impact-pd");
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
  // Loading / Error
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-72" />
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
          <div className="space-y-4">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
          <Skeleton className="h-48 w-full" />
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">Gagal memuat data Impact PD.</p>
        <Button variant="outline" asChild>
          <Link href="/master/impact-pd">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isEditable = item.workflowStatus === "DRAFT" || item.workflowStatus === "REJECTED";
  const canEdit = perms.canUpdate("ecl_parameter") && isEditable;
  const canDelete = perms.canDelete("ecl_parameter") && isEditable;
  const canSubmit = perms.canSubmit("ecl_parameter") && isEditable;

  const statusStr = item.workflowStatus as string;
  const multiplierNum = Number(item.impactMultiplier);

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/impact-pd" className="hover:underline">
          Impact PD
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{id.slice(0, 8)}&hellip;</span>
      </nav>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">Impact PD</h1>
          <span
            className={cn(
              "font-mono tabular-nums text-lg font-bold",
              multiplierNum < 1
                ? "text-green-700"
                : multiplierNum > 1
                  ? "text-red-700"
                  : "text-muted-foreground",
            )}
          >
            {item.impactMultiplier}
          </span>
          <WorkflowStatusBadge status={statusStr} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/impact-pd/${id}/edit`}>
                <Pencil className="mr-1.5 h-4 w-4" aria-hidden />
                Edit
              </Link>
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="icon" className="h-9 w-9" aria-label="Aksi lainnya">
                <MoreHorizontal className="h-4 w-4" aria-hidden />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {canSubmit && (
                <DropdownMenuItem onClick={handleSubmit}>
                  {item.workflowStatus === "REJECTED"
                    ? "Kirim Ulang untuk Review"
                    : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/impact-pd/${id}/history`}>Lihat Riwayat Audit</Link>
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

      {/* DEC-010 inline note */}
      <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-xs text-blue-800">
        <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
        <span>
          <strong>DEC-010:</strong> Impact PD multiplier global per periode buku. Range 0.5–2.0.
          Persetujuan 6-eyes + step-up MFA (ROLE-RISK Approve 1, ROLE-ALCO Approve 2 Final).
        </span>
      </div>

      {/* Body: 2-col layout */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
        {/* Left: detail */}
        <div className="space-y-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Detail Parameter
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Periode ID"
                  value={<code className="font-mono text-xs">{item.periodeId}</code>}
                />
                <DetailRow
                  label="Impact Multiplier"
                  value={
                    <span
                      className={cn(
                        "font-mono tabular-nums text-base font-bold",
                        multiplierNum < 1
                          ? "text-green-700"
                          : multiplierNum > 1
                            ? "text-red-700"
                            : "text-muted-foreground",
                      )}
                    >
                      {item.impactMultiplier}
                    </span>
                  }
                />
                <DetailRow
                  label="Interpretasi"
                  value={
                    <span className="text-sm">
                      {multiplierNum < 1
                        ? "PD berkurang — kondisi ekonomi membaik"
                        : multiplierNum > 1
                          ? "PD meningkat — kondisi ekonomi memburuk"
                          : "Netral — tidak ada penyesuaian FL"}
                    </span>
                  }
                />
              </div>
            </CardContent>
          </Card>

          {/* Catatan */}
          {item.catatan && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                  Catatan
                </CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-sm whitespace-pre-wrap">{item.catatan}</p>
              </CardContent>
            </Card>
          )}

          {/* Metadata */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Metadata
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow label="Dibuat oleh" value={item.createdBy ?? "—"} />
                <DetailRow label="Dibuat pada" value={formatDt(item.createdAt)} />
                <DetailRow label="Diperbarui oleh" value={item.updatedBy ?? "—"} />
                <DetailRow label="Diperbarui pada" value={formatDt(item.updatedAt)} />
                <DetailRow label="Versi" value={item.rowVersion} />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/impact-pd/${id}/history`}
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
                  workflowData={item.workflow as unknown as Parameters<typeof MakerReviewerApproverPanel>[0]["workflowData"]}
                  currentUserId={perms.userId}
                  entityStatus={statusStr as Parameters<typeof MakerReviewerApproverPanel>[0]["entityStatus"]}
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

              {/* Approve 2 — final step */}
              {statusStr === "PENDING_APPROVAL_2" && perms.canApprove("ecl_parameter") && (
                <div className="mt-4 pt-4 border-t">
                  <p className="mb-2 text-sm text-muted-foreground">
                    Menunggu persetujuan final ALCO (step-up MFA wajib)
                  </p>
                  <Button
                    size="sm"
                    onClick={() => void handleApprove2(undefined)}
                    disabled={workflowSubmitting}
                  >
                    {workflowSubmitting ? "Memproses..." : "Approve 2 (Final — ALCO)"}
                  </Button>
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
            <DialogTitle>Hapus Impact PD?</DialogTitle>
            <DialogDescription>
              Record ini akan dihapus (soft-delete). Tindakan ini dapat dibatalkan oleh administrator.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={deleting}>
              Batal
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
              {deleting ? "Menghapus..." : "Hapus"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

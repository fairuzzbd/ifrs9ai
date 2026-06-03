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

import { impactMevPdApi } from "@/lib/api/impact-mev-pd.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { ImpactMevPdDetail } from "@/lib/schemas/impact-mev-pd.schema";
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

export default function ImpactMevPdDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["impact-mev-pd", id],
    queryFn: () => impactMevPdApi.get(id),
    enabled: !!id,
  });

  const item: ImpactMevPdDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["impact-mev-pd", id] });
    void queryClient.invalidateQueries({ queryKey: ["impact-mev-pd"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await impactMevPdApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(`Impact MEV-PD ${item.skenario} berhasil disubmit untuk review.`);
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
      const res = await impactMevPdApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Impact MEV-PD ${item.skenario} berhasil di-review. Status: ${res.data.currentState}.`,
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
      const res = await impactMevPdApi.approve(
        id,
        { comment, signatureMethod: "JWT_STEP_UP", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Impact MEV-PD ${item.skenario} berhasil disetujui (Approve 1). Status: ${res.data.currentState}.`,
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
      const res = await impactMevPdApi.approve2(
        id,
        { comment, signatureMethod: "JWT_STEP_UP", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Impact MEV-PD ${item.skenario} berhasil disetujui (Approve 2 — Final). Status: ${res.data.currentState}.`,
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
      await impactMevPdApi.reject(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.warning(`Impact MEV-PD ${item.skenario} dikembalikan ke maker.`);
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
      await impactMevPdApi.delete(id, uuidv4());
      notify.destructive(`Impact MEV-PD ${item.skenario} berhasil dihapus.`);
      router.push("/master/impact-mev-pd");
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
            {Array.from({ length: 6 }).map((_, i) => (
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
          Gagal memuat data Impact MEV-PD.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/impact-mev-pd">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isEditable =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "REJECTED";
  const canEdit = perms.canUpdate("ecl_parameter") && isEditable;
  const canDelete = perms.canDelete("ecl_parameter") && isEditable;
  const canSubmit = perms.canSubmit("ecl_parameter") && isEditable;

  const statusStr = item.workflowStatus as string;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/impact-mev-pd" className="hover:underline">
          Impact MEV-PD
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{id.slice(0, 8)}&hellip;</span>
      </nav>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">
            Impact MEV-PD &mdash; {item.skenario}
          </h1>
          <span
            className={cn(
              "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-semibold",
              item.skenario === "GOOD"
                ? "bg-green-100 text-green-800"
                : "bg-red-100 text-red-800",
            )}
          >
            {item.skenario}
          </span>
          <WorkflowStatusBadge status={statusStr} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/impact-mev-pd/${id}/edit`}>
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
                <Link href={`/master/impact-mev-pd/${id}/history`}>
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

      {/* DEC-010 inline note */}
      <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-xs text-blue-800">
        <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
        <span>
          <strong>DEC-010:</strong> Parameter ECL FL multiplier. Persetujuan 6-eyes diperlukan.
          Approve 1 &amp; 2 memerlukan step-up MFA (ROLE-RISK + ROLE-ALCO).
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
                <DetailRow label="Skenario" value={item.skenario} />
                <DetailRow label="Periode ID" value={
                  <code className="font-mono text-xs">{item.periodeId}</code>
                } />
                <DetailRow
                  label="Impact Multiplier"
                  value={
                    <span className="font-mono tabular-nums text-base font-bold">
                      {item.impactMultiplier}
                    </span>
                  }
                />
                <DetailRow
                  label="Interpretasi"
                  value={
                    <span className={cn(
                      "text-sm",
                      Number(item.impactMultiplier) < 1
                        ? "text-green-700"
                        : Number(item.impactMultiplier) > 1
                          ? "text-red-700"
                          : "text-muted-foreground",
                    )}>
                      {Number(item.impactMultiplier) < 1
                        ? "PD berkurang (kondisi lebih baik)"
                        : Number(item.impactMultiplier) > 1
                          ? "PD meningkat (kondisi lebih buruk)"
                          : "Netral (NORMAL)"}
                    </span>
                  }
                />
              </div>
            </CardContent>
          </Card>

          {/* MEV Components */}
          {item.mevComponentsJson && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                  Komponen MEV
                </CardTitle>
              </CardHeader>
              <CardContent>
                <pre className="overflow-auto rounded-md border bg-muted/30 p-3 text-xs font-mono">
                  {(() => {
                    try {
                      return JSON.stringify(JSON.parse(item.mevComponentsJson!), null, 2);
                    } catch {
                      return item.mevComponentsJson;
                    }
                  })()}
                </pre>
              </CardContent>
            </Card>
          )}

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
            href={`/master/impact-mev-pd/${id}/history`}
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

              {/* Approve 2 button — visible only at PENDING_APPROVAL_2 */}
              {statusStr === "PENDING_APPROVAL_2" && perms.canApprove("ecl_parameter") && (
                <div className="mt-4 pt-4 border-t">
                  <p className="mb-2 text-sm text-muted-foreground">
                    Menunggu persetujuan final (Approve 2 — ROLE-ALCO, step-up MFA wajib)
                  </p>
                  <Button
                    size="sm"
                    onClick={() => void handleApprove2(undefined)}
                    disabled={workflowSubmitting}
                  >
                    {workflowSubmitting ? "Memproses..." : "Approve 2 (Final)"}
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
            <DialogTitle>Hapus Impact MEV-PD?</DialogTitle>
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

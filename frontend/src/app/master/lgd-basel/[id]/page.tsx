"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { Pencil, MoreHorizontal, AlertTriangle } from "lucide-react";
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

import { lgdBaselApi } from "@/lib/api/lgd-basel.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  TIPE_EKSPOSUR_LABELS,
  lgdDecimalToPercent,
  type LGDBaselDetail,
} from "@/lib/schemas/lgd-basel.schema";

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

function fmtDate(val: string | null | undefined): string {
  if (!val) return "—";
  try {
    return format(parseISO(val), "dd MMM yyyy");
  } catch {
    return val;
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function LGDBaselDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["lgd-basel", id],
    queryFn: () => lgdBaselApi.get(id),
    enabled: !!id,
  });

  const item: LGDBaselDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["lgd-basel", id] });
    void queryClient.invalidateQueries({ queryKey: ["lgd-basel"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await lgdBaselApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(
        `LGD pool ${TIPE_EKSPOSUR_LABELS[item.tipeEksposur]} berhasil disubmit untuk review. Menunggu Risk Officer.`,
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
      const res = await lgdBaselApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `LGD pool ${TIPE_EKSPOSUR_LABELS[item.tipeEksposur]} berhasil di-review. Status: ${res.data.currentState}.`,
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
      const res = await lgdBaselApi.approve(
        id,
        {
          comment,
          // DEC-027: approve step requires JWT_STEP_UP MFA
          signatureMethod: "JWT_STEP_UP",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.success(
        `LGD pool ${TIPE_EKSPOSUR_LABELS[item.tipeEksposur]} disetujui (Approval 1). Status: ${res.data.currentState}.`,
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
      const res = await lgdBaselApi.approve2(
        id,
        {
          comment,
          // DEC-027: approve2 step also requires JWT_STEP_UP MFA
          signatureMethod: "JWT_STEP_UP",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.success(
        `LGD pool ${TIPE_EKSPOSUR_LABELS[item.tipeEksposur]} disetujui final (Approval 2). Parameter ECL sekarang aktif.`,
        {
          action: {
            label: "Lihat riwayat",
            onClick: () => router.push(`/master/lgd-basel/${id}/history`),
          },
        },
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
      await lgdBaselApi.reject(
        id,
        {
          comment,
          signatureMethod: "JWT_STANDARD",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.warning(
        `LGD pool ${TIPE_EKSPOSUR_LABELS[item.tipeEksposur]} dikembalikan ke maker.`,
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
      await lgdBaselApi.softDelete(id, uuidv4());
      notify.destructive(`LGD pool ${TIPE_EKSPOSUR_LABELS[item.tipeEksposur]} berhasil dihapus.`);
      router.push("/master/lgd-basel");
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
            {Array.from({ length: 7 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
          <Skeleton className="h-80 w-full" />
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">
          Gagal memuat data LGD pool {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/lgd-basel">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("ecl_parameter") && isDraft;
  const canDelete = perms.canDelete("ecl_parameter") && isDraft;
  const canSubmit = perms.canSubmit("ecl_parameter") && isDraft;

  const tipeLabel = TIPE_EKSPOSUR_LABELS[item.tipeEksposur] ?? item.tipeEksposur;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/lgd-basel" className="hover:underline">
          LGD Basel Pool
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{tipeLabel}</span>
      </nav>

      {/* ECL parameter warning banner */}
      <div
        role="note"
        className="flex items-start gap-3 rounded-md border border-amber-300 bg-amber-50 px-4 py-3"
      >
        <AlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
          aria-hidden
        />
        <p className="text-sm text-amber-800">
          <strong>Parameter ECL</strong> — perubahan LGD pool ini akan mempengaruhi
          semua kalkulasi ECL Stage 1/2/3. Workflow 6-eyes berlaku: dua tahap approval
          dengan MFA step-up wajib (DEC-027).
        </p>
      </div>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-2xl font-semibold">
            LGD Pool: {tipeLabel}
          </h1>
          <WorkflowStatusBadge status={item.workflowStatus} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/lgd-basel/${id}/edit`}>
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
                <DropdownMenuItem onClick={handleSubmit} disabled={workflowSubmitting}>
                  {item.workflowStatus === "RETURNED"
                    ? "Kirim Ulang untuk Review"
                    : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/lgd-basel/${id}/history`}>
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
                Detail Parameter LGD
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow label="Tipe Eksposur" value={tipeLabel} />
                <DetailRow
                  label="LGD"
                  value={
                    <span className="font-mono font-bold text-lg">
                      {lgdDecimalToPercent(item.lgd)}%
                    </span>
                  }
                />
                <DetailRow
                  label="LGD (desimal)"
                  value={
                    <code className="font-mono text-xs text-muted-foreground">
                      {item.lgd}
                    </code>
                  }
                />
                <DetailRow label="Sumber" value={
                  <span className="font-mono text-xs border rounded px-1.5 py-0.5">
                    {item.sumber}
                  </span>
                } />
                <DetailRow
                  label="Berlaku Dari"
                  value={fmtDate(item.periodeBerlakuDari)}
                />
                <DetailRow
                  label="Berlaku Sampai"
                  value={
                    item.periodeBerlakuSampai ? (
                      fmtDate(item.periodeBerlakuSampai)
                    ) : (
                      <span className="text-green-700 font-medium">
                        Sekarang (tidak ada tanggal berakhir)
                      </span>
                    )
                  }
                />
                {item.karakteristik && (
                  <div className="col-span-2 flex flex-col gap-0.5">
                    <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                      Karakteristik
                    </span>
                    <p className="text-sm whitespace-pre-wrap">{item.karakteristik}</p>
                  </div>
                )}
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
                <DetailRow label="Dibuat pada" value={formatDt(item.createdAt)} />
                <DetailRow label="Diperbarui oleh" value={item.updatedBy} />
                <DetailRow label="Diperbarui pada" value={formatDt(item.updatedAt)} />
                <DetailRow label="Versi" value={item.rowVersion} />
                <DetailRow label="ID" value={
                  <code className="font-mono text-xs text-muted-foreground break-all">
                    {item.id}
                  </code>
                } />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/lgd-basel/${id}/history`}
            className="text-sm text-primary hover:underline"
          >
            Lihat Riwayat Audit &rarr;
          </Link>
        </div>

        {/* Right: 6-eyes workflow panel */}
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
                  onApprove2={handleApprove2}
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

      {/* Delete confirmation dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hapus LGD Pool {tipeLabel}?</DialogTitle>
            <DialogDescription>
              LGD pool untuk <strong>{tipeLabel}</strong> (LGD:{" "}
              {lgdDecimalToPercent(item.lgd)}%) akan dihapus (soft-delete). Jika
              ada referensi dari kalkulasi ECL, penghapusan akan ditolak (409).
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

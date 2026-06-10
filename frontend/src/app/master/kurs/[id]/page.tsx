"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { Pencil, MoreHorizontal, Lock } from "lucide-react";
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

import { kursApi } from "@/lib/api/kurs.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  formatKursTengah,
  formatKursTable,
  SUMBER_KURS_LABELS,
  type KursDetail,
} from "@/lib/schemas/kurs.schema";
import type { WorkflowStatus } from "@/lib/schemas/mata-uang.schema";

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
// Adapter: KursWorkflowStatus → WorkflowStatus (mata-uang type used by panel)
// ---------------------------------------------------------------------------

function toWorkflowStatus(kurs: KursDetail): WorkflowStatus | null {
  if (!kurs.workflow) return null;
  return {
    currentState: kurs.workflow.currentState,
    workflowEyes: kurs.workflow.workflowEyes,
    makerId: kurs.workflow.makerId,
    reviewerId: kurs.workflow.reviewerId,
    approverId: kurs.workflow.approverId,
    history: kurs.workflow.history.map((h) => ({
      action: h.action,
      userId: h.userId,
      username: h.username,
      role: h.role,
      signedAt: h.signedAt,
      signatureHash: h.signatureHash,
      comment: h.comment,
    })),
  } as WorkflowStatus;
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function KursDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["kurs", id],
    queryFn: () => kursApi.get(id),
    enabled: !!id,
  });

  const item: KursDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["kurs", id] });
    void queryClient.invalidateQueries({ queryKey: ["kurs"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmitWorkflow = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await kursApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(`Kurs ${item.fxRateIdKode} berhasil disubmit untuk review.`);
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
      const res = await kursApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(`Kurs ${item.fxRateIdKode} berhasil di-review. Status: ${res.data.currentState}.`);
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
      await kursApi.approve(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(`Kurs ${item.fxRateIdKode} berhasil disetujui. Kurs aktif dan dapat digunakan.`);
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
      await kursApi.reject(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.warning(`Kurs ${item.fxRateIdKode} dikembalikan ke maker dengan alasan penolakan.`);
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
      await kursApi.delete(id, uuidv4());
      notify.destructive(`Kurs ${item.fxRateIdKode} berhasil dihapus.`);
      router.push("/master/kurs");
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
        <Skeleton className="h-8 w-72" />
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
        <p className="text-sm text-destructive">Gagal memuat data kurs {id}.</p>
        <Button variant="outline" asChild>
          <Link href="/master/kurs">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft = item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("kurs") && isDraft && !item.lockedFlag;
  const canDelete = perms.canDelete("kurs") && isDraft && !item.lockedFlag;
  const canSubmit = perms.canSubmit("kurs") && isDraft && !item.lockedFlag;

  const workflowForPanel = toWorkflowStatus(item);

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/kurs" className="hover:underline">
          Kurs
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{item.fxRateIdKode}</span>
      </nav>

      {/* Locked banner */}
      {item.lockedFlag && (
        <div
          role="alert"
          className="flex items-start gap-3 rounded-md border border-slate-300 bg-slate-50 px-4 py-3"
        >
          <Lock className="mt-0.5 h-4 w-4 shrink-0 text-slate-600" aria-hidden />
          <p className="text-sm text-slate-800">
            Kurs ini terkunci — periode buku terkait sudah di-hard-close. Data read-only.
          </p>
        </div>
      )}

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">
            Kurs {item.kodeMataUang} — {item.tanggalBerlaku}
          </h1>
          <WorkflowStatusBadge status={item.workflowStatus} />
          {item.lockedFlag && (
            <span className="inline-flex items-center gap-1 rounded-full border border-slate-300 bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700">
              <Lock className="h-3 w-3" aria-hidden />
              Terkunci
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/kurs/${id}/edit`}>
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
                <DropdownMenuItem onClick={handleSubmitWorkflow}>
                  {item.workflowStatus === "RETURNED"
                    ? "Kirim Ulang untuk Review"
                    : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/kurs/${id}/history`}>Lihat Riwayat Audit</Link>
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
        {/* Left */}
        <div className="space-y-6">
          {/* Identifikasi */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Identifikasi
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="ID Kurs"
                  value={<code className="font-mono text-sm">{item.fxRateIdKode}</code>}
                />
                <DetailRow label="Mata Uang" value={
                  <code className="font-mono font-bold">{item.kodeMataUang}</code>
                } />
                <DetailRow label="Tanggal Berlaku" value={item.tanggalBerlaku} />
                <DetailRow label="Periode Bulanan ID" value={
                  <code className="font-mono text-xs text-muted-foreground">{item.periodeBulananId}</code>
                } />
                <DetailRow label="Sumber Kurs" value={
                  <span className="rounded-md border px-1.5 py-0.5 text-xs">
                    {SUMBER_KURS_LABELS[item.sumberKurs] ?? item.sumberKurs}
                  </span>
                } />
                <DetailRow label="Status Terkunci" value={
                  item.lockedFlag ? (
                    <span className="text-slate-700 flex items-center gap-1">
                      <Lock className="h-3 w-3" aria-hidden /> Ya
                    </span>
                  ) : "Tidak"
                } />
              </div>
            </CardContent>
          </Card>

          {/* Nilai Kurs */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Nilai Kurs (IDR per 1 unit {item.kodeMataUang})
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-3 gap-4">
                <div className="flex flex-col gap-1">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                    Kurs Beli
                  </span>
                  <span className="font-mono text-sm font-medium text-right">
                    {item.kursBeli ? formatKursTable(item.kursBeli) : "—"}
                  </span>
                </div>
                <div className="flex flex-col gap-1 border-x px-4">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                    Kurs Tengah
                  </span>
                  <span className="font-mono text-base font-bold text-right text-primary">
                    {formatKursTengah(item.kursTengah)}
                  </span>
                </div>
                <div className="flex flex-col gap-1">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                    Kurs Jual
                  </span>
                  <span className="font-mono text-sm font-medium text-right">
                    {item.kursJual ? formatKursTable(item.kursJual) : "—"}
                  </span>
                </div>
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
                <DetailRow label="Dibuat oleh" value={item.createdBy ?? "—"} />
                <DetailRow label="Dibuat pada" value={formatDt(item.createdAt)} />
                <DetailRow label="Diperbarui oleh" value={item.updatedBy ?? "—"} />
                <DetailRow label="Diperbarui pada" value={formatDt(item.updatedAt)} />
                <DetailRow label="Versi" value={item.rowVersion} />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/kurs/${id}/history`}
            className="text-sm text-primary hover:underline"
          >
            Lihat Riwayat Audit &rarr;
          </Link>
        </div>

        {/* Right: workflow panel */}
        <div>
          <Card>
            <CardContent className="pt-6">
              {workflowForPanel ? (
                <MakerReviewerApproverPanel
                  workflowData={workflowForPanel}
                  currentUserId={perms.userId}
                  entityStatus={item.workflowStatus}
                  submitting={workflowSubmitting}
                  onSubmit={handleSubmitWorkflow}
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
                      onClick={handleSubmitWorkflow}
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
            <DialogTitle>Hapus Kurs {item.fxRateIdKode}?</DialogTitle>
            <DialogDescription>
              Kurs <strong>{item.kodeMataUang}</strong> tanggal{" "}
              <strong>{item.tanggalBerlaku}</strong> akan dihapus dari sistem (soft-delete).
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

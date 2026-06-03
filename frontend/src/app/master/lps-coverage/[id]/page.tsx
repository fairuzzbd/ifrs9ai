"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { Pencil, MoreHorizontal, AlertTriangle, Info } from "lucide-react";
import { v4 as uuidv4 } from "uuid";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { Badge } from "@/components/ui/badge";
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

import { lpsCoverageApi } from "@/lib/api/lps-coverage.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  formatIDR,
  type LPSCoverageDetail,
} from "@/lib/schemas/lps-coverage.schema";

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

function formatDate(d: string | null | undefined): string {
  if (!d) return "—";
  try {
    return format(parseISO(d), "dd MMM yyyy");
  } catch {
    return d;
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function LPSCoverageDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["lps-coverage", id],
    queryFn: () => lpsCoverageApi.get(id),
    enabled: !!id,
  });

  const item: LPSCoverageDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["lps-coverage", id] });
    void queryClient.invalidateQueries({ queryKey: ["lps-coverage"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await lpsCoverageApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(
        `LPS Coverage ${formatIDR(item.coverageAmount)} berhasil disubmit untuk review. Menunggu Finance Controller.`,
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
      const res = await lpsCoverageApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `LPS Coverage berhasil di-review. Status: ${res.data.currentState}.`,
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
      const res = await lpsCoverageApi.approve(
        id,
        { comment, signatureMethod: "JWT_STEP_UP", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `LPS Coverage cap ${formatIDR(item.coverageAmount)} berhasil disetujui. Status: ${res.data.currentState}. Coverage aktif pada periode yang berlaku.`,
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
      await lpsCoverageApi.reject(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.warning(
        `LPS Coverage dikembalikan ke maker dengan alasan penolakan.`,
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
      await lpsCoverageApi.delete(id, uuidv4());
      notify.destructive(
        `LPS Coverage cap ${formatIDR(item.coverageAmount)} berhasil dihapus.`,
      );
      router.push("/master/lps-coverage");
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
  // Loading / error
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
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
          Gagal memuat data LPS Coverage {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/lps-coverage">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const isActive =
    item.periodeBlakuSampai === null && item.workflowStatus === "APPROVED";
  const canEdit = perms.canUpdate("ecl_parameter") && isDraft;
  const canDelete = perms.canDelete("ecl_parameter") && isDraft;
  const canSubmit = perms.canSubmit("ecl_parameter") && isDraft;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/lps-coverage" className="hover:underline">
          LPS Coverage
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{id}</span>
      </nav>

      {/* DEC-014 prominent banner */}
      <div
        role="note"
        aria-label="Peringatan parameter ECL DEC-014"
        className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3"
      >
        <AlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
          aria-hidden
        />
        <p className="text-sm text-amber-800">
          <strong>Parameter ECL — DEC-014:</strong> LPS coverage cap ini
          digunakan oleh LPS Aggregator dalam kalkulasi ECL Stage 1/2/3.
          Eksposur (Cash + Deposito per nasabah per bank) di atas cap{" "}
          <strong>{formatIDR(item.coverageAmount)}</strong> akan dikenakan ECL.
          Eksposur di bawah cap dijamin LPS, ECL = 0. Perubahan cap memerlukan
          persetujuan ALCO.
        </p>
      </div>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">
            LPS Coverage: {formatIDR(item.coverageAmount)}
          </h1>
          {isActive && (
            <Badge className="bg-green-100 text-green-800 border-green-300 hover:bg-green-100 text-xs font-semibold">
              AKTIF
            </Badge>
          )}
          <WorkflowStatusBadge status={item.workflowStatus} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/lps-coverage/${id}/edit`}>
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
                <Link href={`/master/lps-coverage/${id}/history`}>
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
        {/* Left: detail cards */}
        <div className="space-y-6">
          {/* Coverage Detail */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Detail Coverage LPS
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow
                  label="Coverage Cap"
                  value={
                    <span className="font-mono font-bold text-base tabular-nums">
                      {formatIDR(item.coverageAmount)}
                    </span>
                  }
                />
                <DetailRow
                  label="Mata Uang"
                  value={
                    <Badge variant="outline" className="font-mono text-xs">
                      IDR
                    </Badge>
                  }
                />
                <DetailRow
                  label="Berlaku Dari"
                  value={formatDate(item.periodeBlakuDari)}
                />
                <DetailRow
                  label="Berlaku Sampai"
                  value={
                    item.periodeBlakuSampai ? (
                      formatDate(item.periodeBlakuSampai)
                    ) : (
                      <span className="inline-flex items-center gap-1.5 text-green-700 font-semibold">
                        <Info className="h-3.5 w-3.5" aria-hidden />
                        Tidak ditentukan (Periode Aktif)
                      </span>
                    )
                  }
                />
                {item.regulasiReferensi && (
                  <div className="col-span-2 flex flex-col gap-0.5">
                    <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                      Referensi Regulasi
                    </span>
                    <span className="text-sm font-medium whitespace-pre-wrap">
                      {item.regulasiReferensi}
                    </span>
                  </div>
                )}
              </div>
            </CardContent>
          </Card>

          {/* ECL Impact info */}
          <Card className="border-amber-200 bg-amber-50/30">
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-amber-700 font-semibold flex items-center gap-1.5">
                <AlertTriangle className="h-4 w-4" aria-hidden />
                Dampak terhadap ECL Calculation
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-2 text-sm text-amber-900">
                <p>
                  <strong>Coverage cap saat ini:</strong>{" "}
                  {formatIDR(item.coverageAmount)} per nasabah per bank (sesuai
                  DEC-014).
                </p>
                <ul className="list-disc pl-4 space-y-1">
                  <li>
                    Eksposur ≤ cap → ECL = 0 (dijamin LPS, tidak masuk
                    perhitungan)
                  </li>
                  <li>
                    Eksposur &gt; cap → ECL dihitung dari selisih (excess
                    exposure × PD × LGD)
                  </li>
                  <li>
                    Berlaku untuk produk Cash dan Deposito yang diaggregasi per
                    (nasabah, bank)
                  </li>
                </ul>
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
                <DetailRow label="ID" value={<code className="font-mono text-xs">{item.id}</code>} />
                <DetailRow label="Versi" value={item.rowVersion} />
                <DetailRow label="Dibuat oleh" value={item.createdBy} />
                <DetailRow label="Dibuat pada" value={formatDt(item.createdAt)} />
                <DetailRow label="Diperbarui oleh" value={item.updatedBy} />
                <DetailRow label="Diperbarui pada" value={formatDt(item.updatedAt)} />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/lps-coverage/${id}/history`}
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

          {/* MFA step-up info for approver */}
          {item.workflowStatus === "PENDING_APPROVAL" && (
            <div className="mt-3 flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-xs text-blue-800">
              <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
              <span>
                Approval parameter ECL memerlukan <strong>MFA step-up</strong>{" "}
                (DEC-027). Pastikan Anda sudah memverifikasi MFA sebelum
                menekan tombol Setujui.
              </span>
            </div>
          )}
        </div>
      </div>

      {/* Delete confirmation dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hapus LPS Coverage Cap?</DialogTitle>
            <DialogDescription>
              Record coverage{" "}
              <strong>{formatIDR(item.coverageAmount)}</strong> akan dihapus
              dari sistem (soft-delete). Tindakan ini dapat dibatalkan oleh
              administrator.
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

"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { Pencil, MoreHorizontal, Shield } from "lucide-react";
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

import { mataUangApi } from "@/lib/api/mata-uang.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { MataUangDetail } from "@/lib/schemas/mata-uang.schema";

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
      <span className="text-sm font-medium">{value}</span>
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

export default function MataUangDetailPage() {
  const { kode } = useParams<{ kode: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["mata-uang", kode],
    queryFn: () => mataUangApi.get(kode),
    enabled: !!kode,
  });

  const item: MataUangDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["mata-uang", kode] });
    void queryClient.invalidateQueries({ queryKey: ["mata-uang"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await mataUangApi.submit(
        kode,
        { rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Mata uang ${kode} berhasil disubmit untuk review. Menunggu Finance Controller.`,
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
      const res = await mataUangApi.review(
        kode,
        {
          comment,
          signatureMethod: "JWT_STANDARD",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.success(
        `Mata uang ${kode} berhasil di-review. Status: ${res.data.currentState}.`,
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
      const res = await mataUangApi.approve(
        kode,
        {
          comment,
          signatureMethod: "JWT_STANDARD",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.success(
        `Mata uang ${kode} berhasil disetujui. Sekarang aktif dan dapat digunakan.`,
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
      await mataUangApi.reject(
        kode,
        {
          comment,
          signatureMethod: "JWT_STANDARD",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.warning(
        `Mata uang ${kode} dikembalikan ke maker dengan alasan penolakan.`,
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
      await mataUangApi.delete(kode, uuidv4());
      notify.destructive(`Mata uang ${kode} berhasil dihapus.`);
      router.push("/master/mata-uang");
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
          Gagal memuat data mata uang {kode}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/mata-uang">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("mata_uang") && isDraft;
  const canDelete =
    perms.canDelete("mata_uang") && isDraft && !item.isSystemCurrency;
  const canSubmit = perms.canSubmit("mata_uang") && isDraft;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/mata-uang" className="hover:underline">
          Mata Uang
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{kode}</span>
      </nav>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">
            Mata Uang: {item.namaMataUang} ({kode})
          </h1>
          {item.isSystemCurrency && (
            <span className="flex items-center gap-1 rounded-full border border-primary/30 bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
              <Shield className="h-3 w-3" aria-hidden />
              Mata uang fungsional sistem
            </span>
          )}
          <WorkflowStatusBadge status={item.workflowStatus} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/mata-uang/${kode}/edit`}>
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
                <Link href={`/master/mata-uang/${kode}/history`}>
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
                Detail Entitas
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow label="Kode Mata Uang" value={
                  <code className="font-mono font-bold">{item.kodeMataUang}</code>
                } />
                <DetailRow label="Nama" value={item.namaMataUang} />
                <DetailRow label="Simbol" value={item.simbol} />
                <DetailRow label="Decimal Places" value={item.decimalPlaces} />
                <DetailRow
                  label="Sumber Kurs"
                  value={item.sumberKursDefault.replace(/_/g, " ")}
                />
                <DetailRow
                  label="Frekuensi"
                  value={item.frekuensiUpdate.replace(/_/g, " ")}
                />
                <DetailRow
                  label="Tgl Mulai Aktif"
                  value={item.tanggalMulaiAktif}
                />
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
            href={`/master/mata-uang/${kode}/history`}
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
            <DialogTitle>Hapus Mata Uang {kode}?</DialogTitle>
            <DialogDescription>
              Mata uang <strong>{item.namaMataUang}</strong> ({kode}) akan
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

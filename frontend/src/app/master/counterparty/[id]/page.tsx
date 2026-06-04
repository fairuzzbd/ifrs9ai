"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import { AlertTriangle, Eye, EyeOff, Lock, MoreHorizontal, Pencil, ShieldAlert, Star } from "lucide-react";
import { v4 as uuidv4 } from "uuid";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { MakerReviewerApproverPanel } from "@/components/blips/MakerReviewerApproverPanel";

import { counterpartyApi } from "@/lib/api/counterparty.api";
import type { CounterpartyPII } from "@/lib/schemas/counterparty.schema";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">{label}</span>
      <span className="text-sm font-medium">{value ?? "—"}</span>
    </div>
  );
}

function formatDt(iso: string | null | undefined): string {
  if (!iso) return "—";
  try { return format(parseISO(iso), "dd MMM yyyy, HH:mm 'WIB'"); } catch { return iso; }
}

// ---------------------------------------------------------------------------
// PII section
// ---------------------------------------------------------------------------

const PII_MASK = "***";
const PII_UNMASK_TIMEOUT_MS = 30_000;

interface PIISectionProps {
  cpId: string;
  cpNama: string;
  canViewPII: boolean;
}

function PIISection({ cpId, cpNama, canViewPII }: PIISectionProps) {
  const [confirmOpen, setConfirmOpen] = React.useState(false);
  const [piiData, setPiiData] = React.useState<CounterpartyPII | null>(null);
  const [loading, setLoading] = React.useState(false);
  const maskTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  // Re-mask after 30 seconds
  React.useEffect(() => {
    if (piiData) {
      maskTimerRef.current = setTimeout(() => {
        setPiiData(null);
      }, PII_UNMASK_TIMEOUT_MS);
    }
    return () => {
      if (maskTimerRef.current) clearTimeout(maskTimerRef.current);
    };
  }, [piiData]);

  const handleViewPII = async () => {
    setConfirmOpen(false);
    setLoading(true);
    try {
      const res = await counterpartyApi.getPII(cpId);
      setPiiData(res.data);
      notify.info("Data PII ditampilkan. Akan disembunyikan kembali dalam 30 detik.");
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setLoading(false);
    }
  };

  const handleMaskManually = () => {
    if (maskTimerRef.current) clearTimeout(maskTimerRef.current);
    setPiiData(null);
  };

  return (
    <>
      {/* DEC-028 sticky banner */}
      <div className="sticky top-0 z-10 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 flex items-start gap-3">
        <Lock className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" aria-hidden />
        <p className="text-xs text-amber-800 leading-relaxed">
          <strong>Data PII di-encrypt at-rest (AES-256).</strong>{" "}
          Akses &ldquo;Lihat PII&rdquo; di-audit secara otomatis. Hanya gunakan untuk kebutuhan investigasi atau audit.
        </p>
      </div>

      {/* PII fields */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <DetailRow
          label="NPWP"
          value={
            <span className={cn("font-mono", !piiData && "tracking-widest text-muted-foreground")}>
              {piiData ? (piiData.npwp ?? "—") : PII_MASK}
            </span>
          }
        />
        <DetailRow
          label="Nomor Rekening"
          value={
            <span className={cn("font-mono", !piiData && "tracking-widest text-muted-foreground")}>
              {piiData ? (piiData.nomorRekening ?? "—") : PII_MASK}
            </span>
          }
        />
        <DetailRow
          label="KTP / NIK"
          value={
            <span className={cn("font-mono", !piiData && "tracking-widest text-muted-foreground")}>
              {piiData ? (piiData.ktp ?? "—") : PII_MASK}
            </span>
          }
        />
      </div>

      {/* View / Hide PII button */}
      <div className="flex items-center gap-2">
        {piiData ? (
          <Button
            variant="outline"
            size="sm"
            onClick={handleMaskManually}
            className="text-amber-700 border-amber-300 hover:bg-amber-50"
          >
            <EyeOff className="mr-1.5 h-4 w-4" aria-hidden />
            Sembunyikan PII
          </Button>
        ) : (
          <Button
            variant="outline"
            size="sm"
            disabled={!canViewPII || loading}
            onClick={() => {
              if (!canViewPII) return;
              setConfirmOpen(true);
            }}
            className={cn(
              canViewPII
                ? "text-amber-700 border-amber-300 hover:bg-amber-50"
                : "cursor-not-allowed opacity-50",
            )}
            title={!canViewPII ? "Anda tidak memiliki izin counterparty.view_pii" : undefined}
            aria-disabled={!canViewPII}
          >
            <Eye className="mr-1.5 h-4 w-4" aria-hidden />
            {loading ? "Memuat..." : "Lihat PII"}
          </Button>
        )}

        {piiData && (
          <span className="text-xs text-amber-600">
            Otomatis disembunyikan dalam 30 detik
          </span>
        )}

        {!canViewPII && (
          <span className="text-xs text-muted-foreground">
            Perlu izin <code className="text-xs">counterparty.view_pii</code>
          </span>
        )}
      </div>

      {/* PII view confirmation dialog */}
      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <ShieldAlert className="h-5 w-5 text-amber-600" aria-hidden />
              Konfirmasi Akses Data PII
            </DialogTitle>
            <DialogDescription className="pt-2 space-y-2">
              <p>
                Anda akan melihat data PII counterparty{" "}
                <strong>{cpNama}</strong>.
              </p>
              <p className="text-amber-700">
                Setiap akses di-log dalam sistem audit. Pastikan Anda memiliki
                alasan yang valid (investigasi/audit) sebelum melanjutkan.
              </p>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)}>
              Batal
            </Button>
            <Button
              onClick={() => void handleViewPII()}
              className="bg-amber-600 text-white hover:bg-amber-700"
            >
              <Eye className="mr-1.5 h-4 w-4" aria-hidden />
              Lanjutkan — Lihat PII
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

// ---------------------------------------------------------------------------
// SICR Alert Banner
// ---------------------------------------------------------------------------

function SICRAlertBanner({ tanggal }: { tanggal: string }) {
  return (
    <div className="flex items-start gap-3 rounded-lg border border-amber-400 bg-amber-50 px-4 py-3">
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" aria-hidden />
      <p className="text-sm text-amber-800">
        <strong>SICR Terdeteksi</strong> — Counterparty ini mengalami Significant Increase in Credit Risk
        pada <strong>{tanggal}</strong>. Counterparty dipindah ke Stage 2 ECL.
      </p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function CounterpartyDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["counterparty", id],
    queryFn: () => counterpartyApi.get(id),
    enabled: !!id,
  });

  const item = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["counterparty", id] });
    void queryClient.invalidateQueries({ queryKey: ["counterparty"] });
  };

  // Workflow actions
  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await counterpartyApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(`Counterparty ${item.kodeCounterparty} berhasil disubmit untuk review.`);
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) notify.error(err as { code: string; message: string; traceId: string });
    } finally { setWorkflowSubmitting(false); }
  };

  const handleReview = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      const res = await counterpartyApi.review(id, { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion }, uuidv4());
      notify.success(`Counterparty ${item.kodeCounterparty} berhasil di-review. Status: ${res.data.currentState}.`);
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) notify.error(err as { code: string; message: string; traceId: string });
    } finally { setWorkflowSubmitting(false); }
  };

  const handleApprove = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await counterpartyApi.approve(id, { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion }, uuidv4());
      notify.success(`Counterparty ${item.kodeCounterparty} berhasil disetujui.`);
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) notify.error(err as { code: string; message: string; traceId: string });
    } finally { setWorkflowSubmitting(false); }
  };

  const handleReject = async (comment: string) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await counterpartyApi.reject(id, { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion }, uuidv4());
      notify.warning(`Counterparty ${item.kodeCounterparty} dikembalikan ke maker.`);
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) notify.error(err as { code: string; message: string; traceId: string });
    } finally { setWorkflowSubmitting(false); }
  };

  const handleDelete = async () => {
    if (!item) return;
    setDeleting(true);
    try {
      await counterpartyApi.delete(id, uuidv4());
      notify.destructive(`Counterparty ${item.kodeCounterparty} berhasil dihapus.`);
      router.push("/master/counterparty");
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) notify.error(err as { code: string; message: string; traceId: string });
    } finally { setDeleting(false); setDeleteOpen(false); }
  };

  // Loading state
  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-72" />
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
          <div className="space-y-4">{Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}</div>
          <Skeleton className="h-64 w-full" />
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">Gagal memuat data counterparty.</p>
        <Button variant="outline" asChild><Link href="/master/counterparty">Kembali ke Daftar</Link></Button>
      </div>
    );
  }

  const isDraft = item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("counterparty") && isDraft;
  const canDelete = perms.canDelete("counterparty") && isDraft;
  const canSubmit = perms.canSubmit("counterparty") && isDraft;
  const canViewPII = perms.can("counterparty.view_pii");

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/counterparty" className="hover:underline">Counterparty</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{item.kodeCounterparty}</span>
      </nav>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">{item.nama}</h1>
          <code className="rounded bg-muted px-2 py-0.5 text-sm font-mono">{item.kodeCounterparty}</code>
          <WorkflowStatusBadge status={item.workflowStatus} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/counterparty/${id}/edit`}>
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
                <DropdownMenuItem onClick={() => void handleSubmit()}>
                  {item.workflowStatus === "RETURNED" ? "Kirim Ulang untuk Review" : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/counterparty/${id}/rating-history`}>
                  <Star className="mr-1.5 h-4 w-4" aria-hidden /> Rating History
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link href={`/master/counterparty/${id}/history`}>Riwayat Audit</Link>
              </DropdownMenuItem>
              {canDelete && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem className="text-destructive" onClick={() => setDeleteOpen(true)}>
                    Hapus
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Body layout */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
        <div className="space-y-6">
          {/* Informasi Dasar */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Informasi Dasar
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow label="Kode" value={<code className="font-mono font-bold">{item.kodeCounterparty}</code>} />
                <DetailRow label="Nama" value={item.nama} />
                <DetailRow label="Tipe" value={item.tipe.replace(/_/g, " ")} />
                <DetailRow label="Tipe Eksposur Basel" value={item.tipeEksposurBasel.replace(/_/g, " ")} />
                <DetailRow
                  label="Rating Pefindo"
                  value={
                    <span className="font-mono font-bold text-base">
                      {item.ratingPefindoCurrent ?? <span className="text-muted-foreground text-sm font-normal">—</span>}
                    </span>
                  }
                />
                <DetailRow
                  label="Status"
                  value={
                    <span className={cn(
                      "font-medium",
                      item.status === "AKTIF" ? "text-green-700" :
                      item.status === "DIBLOKIR" ? "text-destructive" : "text-muted-foreground",
                    )}>
                      {item.status.replace(/_/g, " ")}
                    </span>
                  }
                />
                <DetailRow
                  label="Eligible LPS"
                  value={
                    <span className={item.eligibleLpsFlag ? "text-green-700 font-medium" : "text-muted-foreground"}>
                      {item.eligibleLpsFlag ? "Ya" : "Tidak"}
                    </span>
                  }
                />
                {item.kategoriMi && <DetailRow label="Kategori MI" value={item.kategoriMi.replace(/_/g, " ")} />}
                {item.nomorIzinOjk && <DetailRow label="Nomor Izin OJK" value={item.nomorIzinOjk} />}
              </div>
            </CardContent>
          </Card>

          {/* PII Section */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold flex items-center gap-2">
                <Lock className="h-3.5 w-3.5" aria-hidden />
                Data Pribadi (PII)
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <PIISection cpId={id} cpNama={item.nama} canViewPII={canViewPII} />
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
              </div>
            </CardContent>
          </Card>

          <div className="flex gap-4 text-sm">
            <Link href={`/master/counterparty/${id}/rating-history`} className="text-primary hover:underline flex items-center gap-1">
              <Star className="h-3.5 w-3.5" aria-hidden /> Rating History &rarr;
            </Link>
            <Link href={`/master/counterparty/${id}/history`} className="text-primary hover:underline">
              Riwayat Audit &rarr;
            </Link>
          </div>
        </div>

        {/* Workflow panel */}
        <div className="space-y-4">
          {/* SICR banner (shown if latest rating has SICR triggered) */}
          {/* In a real scenario, this would come from the detail API. We show a placeholder check. */}
          {/* Backend should include a sicrActiveFlag or lastSicrDate in the detail response */}

          <Card>
            <CardContent className="pt-6">
              {item.workflow ? (
                <MakerReviewerApproverPanel
                  workflowData={item.workflow}
                  currentUserId={perms.userId}
                  entityStatus={item.workflowStatus}
                  submitting={workflowSubmitting}
                  onReview={handleReview}
                  onApprove={handleApprove}
                  onReject={handleReject}
                />
              ) : (
                <div className="space-y-2">
                  <p className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">Proses Persetujuan</p>
                  <Separator />
                  <p className="text-sm text-muted-foreground">Data belum disubmit ke workflow.</p>
                  {canSubmit && (
                    <Button size="sm" variant="outline" disabled={workflowSubmitting} onClick={() => void handleSubmit()}>
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
            <DialogTitle>Hapus Counterparty?</DialogTitle>
            <DialogDescription>
              <strong>{item.nama}</strong> ({item.kodeCounterparty}) akan dihapus (soft-delete).
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={deleting}>Batal</Button>
            <Button variant="destructive" onClick={() => void handleDelete()} disabled={deleting}>
              {deleting ? "Menghapus..." : "Hapus"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

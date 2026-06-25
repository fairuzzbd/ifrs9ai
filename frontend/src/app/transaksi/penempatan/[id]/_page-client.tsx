"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ChevronRight, Edit, Lock } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { PenempatanStatusBadge } from "@/components/blips/penempatan/PenempatanStatusBadge";
import { PenempatanWorkflowPanel } from "@/components/blips/penempatan/PenempatanWorkflowPanel";
import { EIRPreviewSidePanel } from "@/components/blips/penempatan/EIRPreviewSidePanel";
import { TerminateActionDialog } from "@/components/blips/penempatan/TerminateActionDialog";
import { SubmitDialog } from "@/components/blips/penempatan/dialogs/SubmitDialog";
import { ReviewDialog } from "@/components/blips/penempatan/dialogs/ReviewDialog";
import { ApproveDialog } from "@/components/blips/penempatan/dialogs/ApproveDialog";
import { RejectDialog } from "@/components/blips/penempatan/dialogs/RejectDialog";
import { WithdrawDialog } from "@/components/blips/penempatan/dialogs/WithdrawDialog";
import { TerminateApproveDialog } from "@/components/blips/penempatan/dialogs/TerminateApproveDialog";
import { SodBlockBanner } from "@/components/blips/SodBlockBanner";
import { PeriodeLockBanner } from "@/components/blips/periode-close/PeriodeLockBanner";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { JSONBTreeView } from "@/components/blips/JSONBTreeView";
import { notify } from "@/lib/notify";
import { penempatanApi } from "@/lib/api/penempatan.api";
import type { PenempatanDeposito, AuditTimelineEvent } from "@/lib/schemas/penempatan.schema";
import { isApiError } from "@/lib/api";
import { format } from "date-fns";
import type { EirPreviewResult } from "@/lib/schemas/penempatan.schema";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatIdr(value: number): string {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(value);
}

function formatDate(s: string | null | undefined): string {
  if (!s) return "-";
  try {
    return format(new Date(s), "d MMMM yyyy");
  } catch {
    return s;
  }
}

function formatRate(value: number | null | undefined): string {
  if (value == null) return "-";
  return `${(value * 100).toFixed(8)}%`;
}

// ---------------------------------------------------------------------------
// Field row
// ---------------------------------------------------------------------------

function FieldRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="grid grid-cols-2 gap-4 py-2">
      <dt className="text-sm text-gray-500">{label}</dt>
      <dd className="text-sm text-gray-900">{value}</dd>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Audit timeline event row
// ---------------------------------------------------------------------------

function AuditEventRow({ event, showJsonb }: { event: AuditTimelineEvent; showJsonb: boolean }) {
  const [expanded, setExpanded] = React.useState(false);

  const actionColor: Record<string, string> = {
    "PENEMPATAN.CREATE": "bg-blue-100 text-blue-700",
    "PENEMPATAN.SUBMIT": "bg-amber-100 text-amber-700",
    "PENEMPATAN.REVIEW": "bg-purple-100 text-purple-700",
    "PENEMPATAN.APPROVE": "bg-green-100 text-green-700",
    "PENEMPATAN.REJECT": "bg-red-100 text-red-700",
    "PENEMPATAN.WITHDRAW": "bg-gray-100 text-gray-600",
    "PENEMPATAN.EIR_COMPUTED": "bg-teal-100 text-teal-700",
  };

  return (
    <div className="border-b last:border-0 py-3">
      <div className="flex items-start gap-3">
        <span
          className={`rounded px-2 py-0.5 text-xs font-medium ${actionColor[event.action] ?? "bg-gray-100 text-gray-600"}`}
        >
          {event.action.replace("PENEMPATAN.", "")}
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-xs text-gray-500">
            {format(new Date(event.eventTime), "d MMM yyyy HH:mm:ss")} — {event.actorUsername}{" "}
            <span className="text-gray-400">({event.actorRole})</span>
          </p>
          {event.comment && (
            <p className="mt-1 text-sm text-gray-700">{event.comment}</p>
          )}
          {event.signatureHash && (
            <p className="mt-0.5 text-xs font-mono text-gray-400">
              Sig: {event.signatureHash.slice(0, 16)}...
            </p>
          )}
          {showJsonb && (event.beforeJsonb || event.afterJsonb) && (
            <button
              type="button"
              className="mt-1 text-xs text-blue-500 hover:underline"
              onClick={() => setExpanded((v) => !v)}
              aria-expanded={expanded}
            >
              {expanded ? "Sembunyikan" : "Lihat Detail"}
            </button>
          )}
          {expanded && showJsonb && (
            <div className="mt-2 space-y-2">
              {event.beforeJsonb && (
                <div>
                  <p className="text-xs font-medium text-gray-500 mb-1">Sebelum:</p>
                  <JSONBTreeView data={event.beforeJsonb} />
                </div>
              )}
              {event.afterJsonb && (
                <div>
                  <p className="text-xs font-medium text-gray-500 mb-1">Sesudah:</p>
                  <JSONBTreeView data={event.afterJsonb} />
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

interface PageProps {
  params: { id: string };
}

export default function PenempatanDetailPage({ params }: PageProps) {
  const router = useRouter();
  const { id } = params;

  const [penempatan, setPenempatan] = React.useState<PenempatanDeposito | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [auditEvents, setAuditEvents] = React.useState<AuditTimelineEvent[]>([]);
  const [hashChainValid, setHashChainValid] = React.useState<boolean | null>(null);
  const [eirPreview, setEirPreview] = React.useState<EirPreviewResult | null>(null);
  const [eirComputeJobId, setEirComputeJobId] = React.useState<string | null>(null);
  const [actionLoading, setActionLoading] = React.useState(false);

  // Dialog state
  const [showSubmit, setShowSubmit] = React.useState(false);
  const [showReview, setShowReview] = React.useState(false);
  const [showApprove, setShowApprove] = React.useState(false);
  const [showReject, setShowReject] = React.useState(false);
  const [showWithdraw, setShowWithdraw] = React.useState(false);
  const [showTerminate, setShowTerminate] = React.useState(false);
  const [showTerminateApprove, setShowTerminateApprove] = React.useState(false);
  const [showTerminateReview, setShowTerminateReview] = React.useState(false);
  const [showTerminateReject, setShowTerminateReject] = React.useState(false);

  // Current user (simplified)
  const currentUserId =
    typeof window !== "undefined" ? localStorage.getItem("blips_user_id") : null;

  // ── Load data ──────────────────────────────────────────────────────────────

  const loadPenempatan = React.useCallback(async () => {
    setLoading(true);
    try {
      const res = await penempatanApi.get(id);
      setPenempatan(res.data);
    } catch (err) {
      if (isApiError(err)) notify.error(err);
    } finally {
      setLoading(false);
    }
  }, [id]);

  const loadAuditTimeline = React.useCallback(async () => {
    try {
      const res = await penempatanApi.auditTimeline(id);
      setAuditEvents(res.data);
      setHashChainValid(res.hashChainValid);
    } catch {
      // audit tab silently fails for roles without access
    }
  }, [id]);

  React.useEffect(() => {
    void loadPenempatan();
    void loadAuditTimeline();
  }, [loadPenempatan, loadAuditTimeline]);

  // ── SoD checks ────────────────────────────────────────────────────────────

  const isMaker = penempatan?.makerId === currentUserId;
  const isReviewer = penempatan?.reviewerId === currentUserId;
  const ws = penempatan?.workflowStatus;

  const canReview = !isMaker;
  const canApprove = !isMaker && !isReviewer;
  const canTerminateReview = !isMaker;
  const canTerminateApprove =
    !isMaker && penempatan?.terminateReviewerId !== currentUserId;

  // ── Action handlers ────────────────────────────────────────────────────────

  const withActionLoading = async (fn: () => Promise<void>) => {
    setActionLoading(true);
    try {
      await fn();
      await loadPenempatan();
    } finally {
      setActionLoading(false);
    }
  };

  const handleSubmit = async (comment: string) => {
    if (!penempatan) return;
    await withActionLoading(async () => {
      await penempatanApi.submit(penempatan.id, { comment, signatureMethod: "JWT_STANDARD" });
      notify.success(
        `Penempatan ${penempatan.kodeTransaksi} berhasil disubmit. Menunggu review dari Treasury Approver.`,
      );
    });
  };

  const handleReview = async (comment: string) => {
    if (!penempatan) return;
    await withActionLoading(async () => {
      await penempatanApi.review(penempatan.id, { comment, signatureMethod: "JWT_STANDARD" });
      notify.success(`Review ${penempatan.kodeTransaksi} berhasil. Menunggu persetujuan Treasury Manager.`);
    });
  };

  const handleApprove = async (comment: string, stepUpToken: string) => {
    if (!penempatan) return;
    await withActionLoading(async () => {
      const res = await penempatanApi.approve(
        penempatan.id,
        { comment, signatureMethod: "JWT_STEP_UP" },
        stepUpToken,
      );
      const jobId = res.data.eirComputeJobId;
      if (jobId) {
        setEirComputeJobId(jobId);
        notify.success(
          `Penempatan ${penempatan.kodeTransaksi} disetujui. EIR sedang dihitung (lihat progress di tab EIR).`,
        );
      } else {
        notify.success(
          `Penempatan ${penempatan.kodeTransaksi} disetujui. Instrumen FVTPL — EIR dan ECL staging tidak diterapkan (PSAK 71 §5.5.15).`,
        );
      }
    });
  };

  const handleReject = async (comment: string) => {
    if (!penempatan) return;
    await withActionLoading(async () => {
      await penempatanApi.reject(penempatan.id, { comment, signatureMethod: "JWT_STANDARD" });
      notify.success(`Penempatan ${penempatan.kodeTransaksi} ditolak. Status kembali ke Konsep.`);
    });
  };

  const handleWithdraw = async () => {
    if (!penempatan) return;
    await withActionLoading(async () => {
      await penempatanApi.withdraw(penempatan.id);
      notify.success(`Penempatan ${penempatan.kodeTransaksi} berhasil dibatalkan.`);
      router.push("/transaksi/penempatan");
    });
  };

  const handleTerminate = async (reason: string) => {
    if (!penempatan) return;
    await withActionLoading(async () => {
      await penempatanApi.terminate(penempatan.id, {
        terminateReason: reason,
        signatureMethod: "JWT_STANDARD",
      });
      notify.success(
        `Proposal terminasi ${penempatan.kodeTransaksi} berhasil diajukan. Menunggu review.`,
      );
    });
  };

  const handleTerminateReview = async (comment: string) => {
    if (!penempatan) return;
    await withActionLoading(async () => {
      await penempatanApi.terminateReview(penempatan.id, { comment, signatureMethod: "JWT_STANDARD" });
      notify.success(`Review terminasi ${penempatan.kodeTransaksi} berhasil. Menunggu persetujuan akhir.`);
    });
  };

  const handleTerminateApprove = async (comment: string, stepUpToken: string) => {
    if (!penempatan) return;
    await withActionLoading(async () => {
      await penempatanApi.terminateApprove(
        penempatan.id,
        { comment, signatureMethod: "JWT_STEP_UP" },
        stepUpToken,
      );
      notify.success(
        `Terminasi ${penempatan.kodeTransaksi} disetujui. Proses derecognition di-queue (P5-M9).`,
      );
    });
  };

  const handleTerminateReject = async (comment: string) => {
    if (!penempatan) return;
    await withActionLoading(async () => {
      await penempatanApi.terminateReject(penempatan.id, { comment, signatureMethod: "JWT_STANDARD" });
      notify.success(`Proposal terminasi ${penempatan.kodeTransaksi} ditolak. Instrumen tetap Aktif.`);
    });
  };

  const handleEirPreview = async () => {
    if (!penempatan) return;
    try {
      const res = await penempatanApi.eirPreview(penempatan.id);
      setEirPreview(res.data);
    } catch (err) {
      if (isApiError(err)) notify.error(err);
    }
  };

  // ── Loading state ─────────────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-8 w-64 bg-gray-200 rounded" />
          <div className="h-32 bg-gray-200 rounded" />
        </div>
      </div>
    );
  }

  if (!penempatan) {
    return (
      <div className="p-6">
        <p className="text-gray-500">Penempatan tidak ditemukan.</p>
        <Button asChild className="mt-4">
          <Link href="/transaksi/penempatan">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isSealed =
    ws === "MATURED" || ws === "TERMINATED" || ws === "CANCELLED";

  return (
    <div className="p-6 space-y-4">
      {/* Periode lock banner */}
      {penempatan.periodeId && (
        <PeriodeLockBanner periodeId={penempatan.periodeId} />
      )}
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="flex items-center gap-1 text-sm text-gray-500">
        <Link href="/transaksi/penempatan" className="hover:underline">Penempatan Deposito</Link>
        <ChevronRight className="h-4 w-4" aria-hidden="true" />
        <span className="text-gray-900">{penempatan.kodeTransaksi}</span>
      </nav>

      {/* Sticky header card */}
      <Card className="sticky top-0 z-10 bg-white shadow-sm">
        <CardContent className="pt-4 pb-3">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-3 flex-wrap">
                <h1 className="text-lg font-bold font-mono">{penempatan.kodeTransaksi}</h1>
                <PenempatanStatusBadge status={penempatan.workflowStatus} />
              </div>
              <div className="mt-1 flex flex-wrap gap-4 text-sm text-gray-600">
                <span>{penempatan.counterpartyBankNama ?? "-"}</span>
                <span>{penempatan.instrumenNama ?? penempatan.instrumenKode ?? "-"}</span>
                <span className="font-semibold">{formatIdr(penempatan.nominalIdr)}</span>
              </div>
              <div className="mt-1 flex flex-wrap gap-4 text-xs text-gray-500">
                <span>Periode: {penempatan.periodeLabel ?? "-"}</span>
                <span>Penempatan: {formatDate(penempatan.tanggalPenempatan)}</span>
                <span>Jatuh Tempo: {formatDate(penempatan.tanggalJatuhTempo)}</span>
                <span>Kupon: {formatRate(penempatan.kuponPersen)}</span>
                {penempatan.klasifikasiPsak71 && (
                  <span className="rounded bg-gray-100 px-2 py-0.5 font-medium">
                    {penempatan.klasifikasiPsak71}
                  </span>
                )}
                {penempatan.eirAwal && (
                  <span>EIR: {formatRate(penempatan.eirAwal)}</span>
                )}
              </div>
            </div>

            {/* Sealed indicator */}
            {isSealed && (
              <div className="flex items-center gap-1 text-sm text-gray-500">
                <Lock className="h-4 w-4" aria-hidden="true" />
                <span>Terkunci</span>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Tabs + Action panel */}
      <div className="flex gap-6">
        {/* Tabs (main content) */}
        <div className="flex-1 min-w-0">
          <Tabs defaultValue="detail">
            <TabsList aria-label="Tab detail penempatan">
              <TabsTrigger value="detail">Detail</TabsTrigger>
              <TabsTrigger value="workflow">Workflow</TabsTrigger>
              <TabsTrigger value="eir">EIR &amp; Amortisasi</TabsTrigger>
              <TabsTrigger value="audit">Audit Trail</TabsTrigger>
            </TabsList>

            {/* ── Detail tab ────────────────────────────────────────────── */}
            <TabsContent value="detail">
              <Card>
                <CardContent className="pt-4">
                  <dl className="divide-y divide-gray-100">
                    <FieldRow label="Instrumen" value={`${penempatan.instrumenKode ?? ""} ${penempatan.instrumenNama ?? ""}`} />
                    <FieldRow label="Bank Counterparty" value={penempatan.counterpartyBankNama ?? "-"} />
                    <FieldRow label="Mata Uang" value={penempatan.mataUangKode ?? "-"} />
                    <FieldRow label="Nominal IDR" value={<span className="font-mono">{formatIdr(penempatan.nominalIdr)}</span>} />
                    {penempatan.nominalFcy && (
                      <FieldRow label="Nominal FCY" value={<span className="font-mono">{penempatan.nominalFcy.toFixed(4)}</span>} />
                    )}
                    {penempatan.kursPenempatan && (
                      <FieldRow label="Kurs Penempatan" value={<span className="font-mono">{penempatan.kursPenempatan.toFixed(8)}</span>} />
                    )}
                    <FieldRow label="Biaya Transaksi" value={<span className="font-mono">{formatIdr(penempatan.biayaTransaksiIdr)}</span>} />
                    <FieldRow label="Tenor" value={`${penempatan.tenorBulan} bulan`} />
                    <FieldRow label="Tanggal Penempatan" value={formatDate(penempatan.tanggalPenempatan)} />
                    <FieldRow label="Tanggal Jatuh Tempo" value={formatDate(penempatan.tanggalJatuhTempo)} />
                    <FieldRow label="Kupon" value={formatRate(penempatan.kuponPersen)} />
                    <FieldRow label="EIR Awal" value={penempatan.eirAwal ? formatRate(penempatan.eirAwal) : "Belum dihitung"} />
                    <FieldRow label="No. Ref. Bank" value={penempatan.nomorReferensiBankIn ?? "-"} />
                    <FieldRow label="Rekening Settlement" value={penempatan.settlementAccount ?? "-"} />
                    <FieldRow label="Klasifikasi PSAK 71" value={penempatan.klasifikasiPsak71 ?? "-"} />
                    <FieldRow label="Catatan" value={penempatan.catatan ?? "-"} />
                  </dl>
                </CardContent>
              </Card>
            </TabsContent>

            {/* ── Workflow tab ──────────────────────────────────────────── */}
            <TabsContent value="workflow">
              <Card>
                <CardContent className="pt-4">
                  <PenempatanWorkflowPanel penempatan={penempatan} />
                </CardContent>
              </Card>
            </TabsContent>

            {/* ── EIR & Amortisasi tab ──────────────────────────────────── */}
            <TabsContent value="eir">
              <EIRPreviewSidePanel
                workflowStatus={penempatan.workflowStatus}
                klasifikasiPsak71={penempatan.klasifikasiPsak71}
                eirAwal={penempatan.eirAwal}
                eirPreviewResult={eirPreview}
                eirComputeJobId={eirComputeJobId}
                onRequestPreview={handleEirPreview}
                onEirJobComplete={async () => {
                  notify.success(`EIR awal ${penempatan.kodeTransaksi} berhasil dihitung.`);
                  await loadPenempatan();
                  setEirComputeJobId(null);
                }}
              />

              {/* EIR compute job progress */}
              {eirComputeJobId && (
                <div className="mt-4">
                  <JobProgressPanel
                    jobId={eirComputeJobId}
                    title="Menghitung EIR Awal"
                    onComplete={async () => {
                      notify.success(`EIR awal ${penempatan.kodeTransaksi} berhasil dihitung.`);
                      await loadPenempatan();
                      setEirComputeJobId(null);
                    }}
                  />
                </div>
              )}
            </TabsContent>

            {/* ── Audit Trail tab ───────────────────────────────────────── */}
            <TabsContent value="audit">
              <Card>
                <CardContent className="pt-4">
                  {/* Hash chain badge */}
                  {hashChainValid !== null && (
                    <div
                      className={`mb-4 inline-flex items-center gap-2 rounded-full px-3 py-1 text-sm ${
                        hashChainValid
                          ? "bg-green-100 text-green-700"
                          : "bg-red-100 text-red-700"
                      }`}
                      role="status"
                    >
                      {hashChainValid ? "Hash Chain: Valid" : "Hash Chain: BROKEN — Hubungi IT Security"}
                    </div>
                  )}

                  {auditEvents.length === 0 ? (
                    <p className="text-sm text-gray-500">Tidak ada event audit.</p>
                  ) : (
                    <div>
                      {auditEvents.map((event) => (
                        <AuditEventRow
                          key={event.eventId}
                          event={event}
                          showJsonb={true}
                        />
                      ))}
                    </div>
                  )}
                </CardContent>
              </Card>
            </TabsContent>
          </Tabs>
        </div>

        {/* Action panel (right sidebar) */}
        <div className="w-56 shrink-0">
          <Card>
            <CardContent className="pt-4 pb-4 space-y-2">
              <p className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-3">
                Aksi Tersedia
              </p>

              {isSealed && (
                <div className="text-xs text-gray-500 text-center py-4">
                  <Lock className="mx-auto h-5 w-5 text-gray-400 mb-1" aria-hidden="true" />
                  Penempatan ini terkunci.
                </div>
              )}

              {/* DRAFT actions */}
              {ws === "DRAFT" && isMaker && (
                <>
                  <Button asChild size="sm" variant="outline" className="w-full">
                    <Link href={`/transaksi/penempatan/${id}/edit`}>
                      <Edit className="mr-1.5 h-4 w-4" aria-hidden="true" />
                      Edit
                    </Link>
                  </Button>
                  <Button
                    size="sm"
                    className="w-full"
                    onClick={() => setShowSubmit(true)}
                    disabled={actionLoading}
                  >
                    Submit
                  </Button>
                  <Separator />
                  <Button
                    size="sm"
                    variant="destructive"
                    className="w-full"
                    onClick={() => setShowWithdraw(true)}
                    disabled={actionLoading}
                  >
                    Batalkan
                  </Button>
                </>
              )}

              {/* PENDING_REVIEW actions */}
              {ws === "PENDING_REVIEW" && (
                <>
                  {!canReview ? (
                    <SodBlockBanner
                      message="Anda tidak bisa mereview penempatan yang Anda buat sendiri."
                      className="text-xs"
                    />
                  ) : (
                    <>
                      <Button
                        size="sm"
                        className="w-full"
                        onClick={() => setShowReview(true)}
                        disabled={actionLoading}
                      >
                        Review
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
                        className="w-full"
                        onClick={() => setShowReject(true)}
                        disabled={actionLoading}
                      >
                        Tolak
                      </Button>
                    </>
                  )}
                </>
              )}

              {/* PENDING_APPROVAL actions */}
              {ws === "PENDING_APPROVAL" && (
                <>
                  {!canApprove ? (
                    <SodBlockBanner
                      message="Anda tidak bisa menyetujui penempatan yang Anda buat atau review sendiri."
                      className="text-xs"
                    />
                  ) : (
                    <>
                      <Button
                        size="sm"
                        className="w-full"
                        onClick={() => setShowApprove(true)}
                        disabled={actionLoading}
                      >
                        Approve (MFA)
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
                        className="w-full"
                        onClick={() => setShowReject(true)}
                        disabled={actionLoading}
                      >
                        Tolak
                      </Button>
                    </>
                  )}
                </>
              )}

              {/* APPROVED_ACTIVE actions */}
              {ws === "APPROVED_ACTIVE" && isMaker && (
                <Button
                  size="sm"
                  variant="outline"
                  className="w-full"
                  onClick={() => setShowTerminate(true)}
                  disabled={actionLoading}
                >
                  Ajukan Terminasi
                </Button>
              )}

              {/* TERMINATION_PENDING_REVIEW */}
              {ws === "TERMINATION_PENDING_REVIEW" && (
                <>
                  {!canTerminateReview ? (
                    <SodBlockBanner
                      message="Anda tidak bisa mereview terminasi yang Anda ajukan sendiri."
                      className="text-xs"
                    />
                  ) : (
                    <>
                      <Button
                        size="sm"
                        className="w-full"
                        onClick={() => setShowTerminateReview(true)}
                        disabled={actionLoading}
                      >
                        Review Terminasi
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
                        className="w-full"
                        onClick={() => setShowTerminateReject(true)}
                        disabled={actionLoading}
                      >
                        Tolak Terminasi
                      </Button>
                    </>
                  )}
                </>
              )}

              {/* TERMINATION_PENDING_APPROVAL */}
              {ws === "TERMINATION_PENDING_APPROVAL" && (
                <>
                  {!canTerminateApprove ? (
                    <SodBlockBanner
                      message="Anda tidak bisa menyetujui terminasi ini karena Anda terlibat di tahap sebelumnya."
                      className="text-xs"
                    />
                  ) : (
                    <>
                      <Button
                        size="sm"
                        className="w-full"
                        onClick={() => setShowTerminateApprove(true)}
                        disabled={actionLoading}
                      >
                        Approve Terminasi (MFA)
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
                        className="w-full"
                        onClick={() => setShowTerminateReject(true)}
                        disabled={actionLoading}
                      >
                        Tolak Terminasi
                      </Button>
                    </>
                  )}
                </>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Dialogs */}
      {penempatan && (
        <>
          <SubmitDialog
            open={showSubmit}
            onOpenChange={setShowSubmit}
            kodeTransaksi={penempatan.kodeTransaksi}
            onConfirm={handleSubmit}
          />
          <ReviewDialog
            open={showReview}
            onOpenChange={setShowReview}
            kodeTransaksi={penempatan.kodeTransaksi}
            sodBlocked={!canReview}
            onConfirm={handleReview}
          />
          <ApproveDialog
            open={showApprove}
            onOpenChange={setShowApprove}
            kodeTransaksi={penempatan.kodeTransaksi}
            sodBlocked={!canApprove}
            onConfirm={handleApprove}
          />
          <RejectDialog
            open={showReject}
            onOpenChange={setShowReject}
            kodeTransaksi={penempatan.kodeTransaksi}
            onConfirm={handleReject}
          />
          <WithdrawDialog
            open={showWithdraw}
            onOpenChange={setShowWithdraw}
            kodeTransaksi={penempatan.kodeTransaksi}
            onConfirm={handleWithdraw}
          />
          <TerminateActionDialog
            open={showTerminate}
            onOpenChange={setShowTerminate}
            kodeTransaksi={penempatan.kodeTransaksi}
            onConfirm={handleTerminate}
          />
          <ReviewDialog
            open={showTerminateReview}
            onOpenChange={setShowTerminateReview}
            kodeTransaksi={penempatan.kodeTransaksi}
            sodBlocked={!canTerminateReview}
            onConfirm={handleTerminateReview}
          />
          <TerminateApproveDialog
            open={showTerminateApprove}
            onOpenChange={setShowTerminateApprove}
            kodeTransaksi={penempatan.kodeTransaksi}
            sodBlocked={!canTerminateApprove}
            onConfirm={handleTerminateApprove}
          />
          <RejectDialog
            open={showTerminateReject}
            onOpenChange={setShowTerminateReject}
            kodeTransaksi={penempatan.kodeTransaksi}
            onConfirm={handleTerminateReject}
          />
        </>
      )}
    </div>
  );
}

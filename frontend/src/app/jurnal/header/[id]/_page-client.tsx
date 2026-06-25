"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { JurnalLinesTable } from "@/components/blips/jurnal/JurnalLinesTable";
import { jurnalQueryApi, manualPostApi } from "@/lib/api/jurnal.api";
import { usePermissions, useAuthStore } from "@/lib/stores/auth.store";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import { v4 as uuidv4 } from "uuid";
import type { JurnalLine } from "@/lib/schemas/jurnal.schema";

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

function fmtDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleDateString("id-ID", { timeZone: "Asia/Jakarta" });
}

export function JurnalHeaderDetailPageClient() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const qc = useQueryClient();
  const { can } = usePermissions();
  const user = useAuthStore((s) => s.user);

  const [approveComment, setApproveComment] = React.useState("");
  const [approveAttested, setApproveAttested] = React.useState(false);
  const [rejectReason, setRejectReason] = React.useState("");
  const [showReject, setShowReject] = React.useState(false);

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["jurnal-header-detail", id],
    queryFn: () => jurnalQueryApi.get(id),
    staleTime: 15_000,
  });

  const jurnal = data?.data;

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["jurnal-header-detail", id] });
    void qc.invalidateQueries({ queryKey: ["jurnal-header-list"] });
  };

  const submitMut = useMutation({
    mutationFn: () =>
      manualPostApi.submit(id, {}, uuidv4()),
    onSuccess: () => {
      notify.success(`Jurnal ${jurnal?.noJurnal ?? id} berhasil di-submit. Menunggu persetujuan.`);
      invalidate();
    },
    onError: (err) => {
      if (isApiError(err)) notify.error(err);
    },
  });

  const approveMut = useMutation({
    mutationFn: () =>
      manualPostApi.approve(id, {
        comment: approveComment,
        signatureMethod: "JWT_STEP_UP",
      }, uuidv4()),
    onSuccess: () => {
      notify.success(`Jurnal ${jurnal?.noJurnal ?? id} disetujui. Akan di-post ke GL pada jadwal berikutnya.`);
      setApproveComment("");
      setApproveAttested(false);
      invalidate();
    },
    onError: (err) => {
      if (isApiError(err)) {
        if (err.code === "SOD_VIOLATION") {
          notify.error({ ...err, message: "Anda tidak bisa menyetujui jurnal yang Anda buat sendiri." });
        } else {
          notify.error(err);
        }
      }
    },
  });

  const rejectMut = useMutation({
    mutationFn: () =>
      manualPostApi.reject(id, {
        rejectReason,
        signatureMethod: "JWT_STEP_UP",
      }, uuidv4()),
    onSuccess: () => {
      notify.success(`Jurnal ${jurnal?.noJurnal ?? id} ditolak. Maker akan dinotifikasi.`);
      setRejectReason("");
      setShowReject(false);
      invalidate();
    },
    onError: (err) => {
      if (isApiError(err)) notify.error(err);
    },
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-24 text-sm text-muted-foreground" aria-live="polite" aria-busy="true">
        <RefreshCw className="h-5 w-5 animate-spin mr-2" aria-hidden="true" />
        Memuat detail jurnal...
      </div>
    );
  }

  if (error || !jurnal) {
    return (
      <div className="flex flex-col items-center py-24 gap-4" aria-live="polite">
        <p className="text-sm text-muted-foreground">Jurnal tidak ditemukan atau Anda tidak memiliki akses.</p>
        <Button variant="outline" onClick={() => router.push("/jurnal/header")}>Kembali</Button>
      </div>
    );
  }

  const isDraft = jurnal.statusInternal === "DRAFT" || jurnal.statusInternal === "PENDING_APPROVAL";
  const isSubmitted = jurnal.statusInternal === "PENDING_APPROVAL";
  const isMaker = jurnal.createdBy === user?.id;
  const canSubmit = can("jurnal.submit") && jurnal.statusInternal === "DRAFT" && isMaker;
  const canApprove = can("jurnal.approve") && isSubmitted && !isMaker;
  const canReject = can("jurnal.reject") && isSubmitted && !isMaker;

  const lines: JurnalLine[] = (jurnal.lines ?? []).map((l) => ({
    urutan: l.urutan,
    posisi: l.posisi,
    akunId: l.akunId,
    akunKode: l.akunKode,
    akunNama: l.akunNama,
    amountIdr: l.amountIdr,
    narasi: l.narasi,
  }));

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-4 border-b px-6 py-4">
        <Button variant="ghost" size="icon" onClick={() => router.push("/jurnal/header")} aria-label="Kembali ke daftar jurnal">
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        </Button>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-3">
            <h2 className="text-xl font-semibold font-mono">{jurnal.noJurnal}</h2>
            <WorkflowStatusBadge status={jurnal.statusInternal} />
          </div>
          <p className="text-sm text-muted-foreground">{fmtDate(jurnal.tanggalPosting)}</p>
        </div>
        <Button variant="outline" size="icon" onClick={() => refetch()} aria-label="Refresh detail jurnal">
          <RefreshCw className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto p-6">
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Left: detail + lines */}
          <div className="lg:col-span-2 space-y-6">
            {/* Info card */}
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm">Informasi Jurnal</CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">Nomor Jurnal</dt>
                    <dd className="font-mono font-semibold">{jurnal.noJurnal}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">Tanggal Posting</dt>
                    <dd>{fmtDate(jurnal.tanggalPosting)}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">Kode Event</dt>
                    <dd className="font-mono text-xs">{jurnal.eventCode}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">Total Debit</dt>
                    <dd className="font-mono">{IDR.format(parseFloat(jurnal.totalDebit || "0"))}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">Total Kredit</dt>
                    <dd className="font-mono">{IDR.format(parseFloat(jurnal.totalKredit || "0"))}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">Dibuat Oleh</dt>
                    <dd className="font-mono text-xs">{jurnal.createdBy ?? "—"}</dd>
                  </div>
                </dl>
              </CardContent>
            </Card>

            {/* Lines table */}
            {lines.length > 0 && (
              <Card>
                <CardHeader className="pb-3">
                  <CardTitle className="text-sm">Baris Jurnal</CardTitle>
                </CardHeader>
                <CardContent className="p-0">
                  <JurnalLinesTable lines={lines} showSubtotal showBalanceBadge />
                </CardContent>
              </Card>
            )}
          </div>

          {/* Right: workflow panel */}
          <div className="space-y-4">
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm">Workflow Jurnal</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                {/* Stepper display */}
                <div className="space-y-2 text-sm">
                  <div className={`flex items-center gap-2 ${["DRAFT","PENDING_APPROVAL","POSTED","REVERSED"].includes(jurnal.statusInternal) ? "text-foreground" : "text-muted-foreground"}`}>
                    <span className="w-5 h-5 rounded-full border flex items-center justify-center text-xs font-bold bg-green-100 border-green-400">1</span>
                    <span>Maker</span>
                    {jurnal.createdBy && <span className="text-xs text-muted-foreground ml-auto">{jurnal.createdBy}</span>}
                  </div>
                  <div className={`flex items-center gap-2 ${["PENDING_APPROVAL","POSTED","REVERSED"].includes(jurnal.statusInternal) ? "text-foreground" : "text-muted-foreground"}`}>
                    <span className="w-5 h-5 rounded-full border flex items-center justify-center text-xs font-bold">2</span>
                    <span>Approver (AKUN-CTL)</span>
                    <span className="text-xs text-muted-foreground ml-auto">
                      {jurnal.statusInternal === "PENDING_APPROVAL" ? "Menunggu" : jurnal.statusInternal === "POSTED" ? "Disetujui" : ""}
                    </span>
                  </div>
                  <div className={`flex items-center gap-2 ${jurnal.statusInternal === "POSTED" ? "text-foreground" : "text-muted-foreground"}`}>
                    <span className="w-5 h-5 rounded-full border flex items-center justify-center text-xs font-bold">3</span>
                    <span>Posted ke GL</span>
                  </div>
                </div>

                <Separator />

                {/* Action area */}
                {canSubmit && (
                  <Button
                    className="w-full"
                    disabled={submitMut.isPending}
                    onClick={() => submitMut.mutate()}
                  >
                    {submitMut.isPending ? "Mengirim..." : "Submit ke Approver"}
                  </Button>
                )}

                {canApprove && (
                  <div className="space-y-3">
                    <div className="space-y-1.5">
                      <Label htmlFor="approve-comment">Komentar (opsional)</Label>
                      <Textarea
                        id="approve-comment"
                        value={approveComment}
                        onChange={(e) => setApproveComment(e.target.value)}
                        placeholder="Komentar approval..."
                        rows={2}
                      />
                    </div>
                    <div className="flex items-start gap-2">
                      <Checkbox
                        id="attest"
                        checked={approveAttested}
                        onCheckedChange={(v) => setApproveAttested(!!v)}
                      />
                      <Label htmlFor="attest" className="text-xs leading-relaxed cursor-pointer">
                        Saya menyatakan jurnal ini sesuai standar akuntansi yang berlaku.
                      </Label>
                    </div>
                    <Button
                      className="w-full"
                      disabled={!approveAttested || approveMut.isPending}
                      onClick={() => approveMut.mutate()}
                    >
                      {approveMut.isPending ? "Menyetujui..." : "Approve Jurnal"}
                    </Button>
                  </div>
                )}

                {canReject && (
                  <>
                    {!showReject ? (
                      <Button
                        variant="outline"
                        className="w-full"
                        onClick={() => setShowReject(true)}
                      >
                        Tolak
                      </Button>
                    ) : (
                      <div className="space-y-2">
                        <Label htmlFor="reject-reason">Alasan Penolakan <span className="text-destructive">*</span></Label>
                        <Textarea
                          id="reject-reason"
                          value={rejectReason}
                          onChange={(e) => setRejectReason(e.target.value)}
                          placeholder="Jelaskan alasan penolakan (min. 30 karakter)..."
                          rows={3}
                          minLength={30}
                        />
                        <div className="flex gap-2">
                          <Button variant="outline" size="sm" className="flex-1" onClick={() => setShowReject(false)}>
                            Batal
                          </Button>
                          <Button
                            variant="destructive"
                            size="sm"
                            className="flex-1"
                            disabled={rejectReason.length < 30 || rejectMut.isPending}
                            onClick={() => rejectMut.mutate()}
                          >
                            {rejectMut.isPending ? "Menolak..." : "Konfirmasi Tolak"}
                          </Button>
                        </div>
                      </div>
                    )}
                  </>
                )}

                {!canSubmit && !canApprove && !canReject && (
                  <p className="text-xs text-muted-foreground text-center">
                    {jurnal.statusInternal === "POSTED"
                      ? "Jurnal sudah terposting ke GL."
                      : "Tidak ada aksi yang tersedia untuk Anda pada jurnal ini."}
                  </p>
                )}
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </div>
  );
}

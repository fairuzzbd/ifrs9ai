"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, RefreshCw, Calendar, FileText, Clock } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { periodeStatusApi } from "@/lib/api/periode-close.api";
import { TIPE_PERIODE_LABELS, BULAN_LABELS } from "@/lib/schemas/periode-close.schema";
import { PeriodeStatusBadge } from "@/components/blips/periode-close/PeriodeStatusBadge";
import { ClosingChecklistPanel } from "@/components/blips/periode-close/ClosingChecklistPanel";
import { ClosingWorkflowActionBar } from "@/components/blips/periode-close/ClosingWorkflowActionBar";
import { MvRefreshStatusCard } from "@/components/blips/periode-close/MvRefreshStatusCard";
import { ChecklistSnapshotDetailDialog } from "@/components/blips/periode-close/ChecklistSnapshotDetailDialog";
import { usePermissions } from "@/lib/stores/auth.store";

// ---------------------------------------------------------------------------
// Date helper
// ---------------------------------------------------------------------------

function fmtDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("id-ID", {
    day: "2-digit",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "Asia/Jakarta",
  });
}

function fmtDateOnly(s: string | null | undefined): string {
  if (!s) return "—";
  return new Date(s).toLocaleDateString("id-ID", {
    day: "2-digit",
    month: "long",
    year: "numeric",
  });
}

// ---------------------------------------------------------------------------
// Page — S5-AC1, S5-AC2, S5-AC3, S5-AC4
// ---------------------------------------------------------------------------

export default function PeriodeBukuDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const qc = useQueryClient();
  const { can } = usePermissions();

  // Checklist "all passed" state — lifted up to gate action buttons
  const [allChecklistPassed, setAllChecklistPassed] = React.useState(false);
  const [mvRefreshJobId, setMvRefreshJobId] = React.useState<string | null>(null);
  const [snapshotDialogId, setSnapshotDialogId] = React.useState<string | null>(null);

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["periode-buku", "detail", id],
    queryFn: () => periodeStatusApi.get(id),
    staleTime: 15_000,
  });

  const periode = data?.data;

  const invalidate = React.useCallback(() => {
    void qc.invalidateQueries({ queryKey: ["periode-buku", "detail", id] });
    void qc.invalidateQueries({ queryKey: ["periode-buku", "checklist", id] });
    void qc.invalidateQueries({ queryKey: ["periode-buku", "list"] });
  }, [qc, id]);

  // Loading state
  if (isLoading) {
    return (
      <div
        className="flex items-center justify-center py-24 text-sm text-muted-foreground"
        aria-live="polite"
        aria-busy="true"
      >
        <RefreshCw className="h-5 w-5 animate-spin mr-2" aria-hidden="true" />
        Memuat detail periode...
      </div>
    );
  }

  // Error state
  if (error || !periode) {
    return (
      <div className="flex flex-col items-center py-24 gap-4" aria-live="polite">
        <p className="text-sm text-muted-foreground">
          Periode buku tidak ditemukan atau Anda tidak memiliki akses.
        </p>
        <Button variant="outline" onClick={() => router.back()}>
          Kembali
        </Button>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-4 border-b px-6 py-4">
        <Button
          variant="ghost"
          size="icon"
          onClick={() => router.push("/periode-buku")}
          aria-label="Kembali ke daftar periode buku"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        </Button>
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <Calendar className="h-5 w-5 text-muted-foreground shrink-0" aria-hidden="true" />
          <div className="min-w-0">
            <h1 className="text-xl font-semibold truncate">
              Periode Buku — {periode.periodeKode}
            </h1>
            <p className="text-sm text-muted-foreground">
              {BULAN_LABELS[periode.bulan] ?? periode.bulan} {periode.tahunBuku} ·{" "}
              {TIPE_PERIODE_LABELS[periode.tipePeriode]}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3 shrink-0">
          <PeriodeStatusBadge
            status={periode.statusPeriode}
            graceExpiresAt={periode.hardCloseGraceExpiresAt ?? undefined}
            size="md"
          />
          <Button
            variant="outline"
            size="icon"
            onClick={() => refetch()}
            aria-label="Refresh detail periode"
          >
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      </div>

      {/* Content grid — 2 columns on lg+ */}
      <div className="flex-1 overflow-auto p-6">
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Left column: detail + checklist */}
          <div className="lg:col-span-2 space-y-6">
            {/* Periode info card */}
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <FileText className="h-4 w-4" aria-hidden="true" />
                  Informasi Periode
                </CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">Kode</dt>
                    <dd className="font-mono font-semibold">{periode.periodeKode}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">Status</dt>
                    <dd>
                      <PeriodeStatusBadge
                        status={periode.statusPeriode}
                        graceExpiresAt={periode.hardCloseGraceExpiresAt ?? undefined}
                        size="sm"
                      />
                    </dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">
                      Tanggal Mulai
                    </dt>
                    <dd>{fmtDateOnly(periode.tanggalMulai)}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">
                      Tanggal Akhir
                    </dt>
                    <dd>{fmtDateOnly(periode.tanggalAkhir)}</dd>
                  </div>
                  {periode.tanggalSoftClose && (
                    <>
                      <div>
                        <dt className="text-muted-foreground text-xs uppercase tracking-wide">
                          Soft-Close
                        </dt>
                        <dd>{fmtDate(periode.tanggalSoftClose)}</dd>
                      </div>
                      <div>
                        <dt className="text-muted-foreground text-xs uppercase tracking-wide">
                          Oleh
                        </dt>
                        <dd className="font-mono text-xs">{periode.softCloseBy ?? "—"}</dd>
                      </div>
                    </>
                  )}
                  {periode.tanggalHardClose && (
                    <>
                      <div>
                        <dt className="text-muted-foreground text-xs uppercase tracking-wide">
                          Hard-Close
                        </dt>
                        <dd>{fmtDate(periode.tanggalHardClose)}</dd>
                      </div>
                      <div>
                        <dt className="text-muted-foreground text-xs uppercase tracking-wide">
                          Oleh
                        </dt>
                        <dd className="font-mono text-xs">{periode.hardCloseBy ?? "—"}</dd>
                      </div>
                    </>
                  )}
                  {periode.hardCloseGraceExpiresAt && (
                    <div className="col-span-2">
                      <dt className="text-muted-foreground text-xs uppercase tracking-wide flex items-center gap-1">
                        <Clock className="h-3 w-3" aria-hidden="true" />
                        Grace Window Expires
                      </dt>
                      <dd>{fmtDate(periode.hardCloseGraceExpiresAt)}</dd>
                    </div>
                  )}
                  {periode.reopenReason && (
                    <div className="col-span-2">
                      <dt className="text-muted-foreground text-xs uppercase tracking-wide">
                        Alasan Reopen Terakhir
                      </dt>
                      <dd className="text-sm italic">{periode.reopenReason}</dd>
                    </div>
                  )}
                  {periode.reopenedFlag && (
                    <div>
                      <dt className="text-muted-foreground text-xs uppercase tracking-wide">
                        Pernah Reopen
                      </dt>
                      <dd>
                        <Badge variant="outline" className="text-orange-700 border-orange-300">
                          Ya
                        </Badge>
                      </dd>
                    </div>
                  )}
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">
                      Jumlah Jurnal
                    </dt>
                    <dd>{periode.jumlahJurnal?.toLocaleString("id-ID") ?? "—"}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground text-xs uppercase tracking-wide">FX Lock</dt>
                    <dd>
                      {periode.fxLocked != null ? (
                        <Badge
                          variant={periode.fxLocked ? "secondary" : "outline"}
                          className={periode.fxLocked ? "bg-green-100 text-green-800" : ""}
                        >
                          {periode.fxLocked ? "Terkunci" : "Belum dikunci"}
                        </Badge>
                      ) : (
                        "—"
                      )}
                    </dd>
                  </div>
                </dl>
              </CardContent>
            </Card>

            {/* Closing Checklist */}
            {can("periode.read") && (
              <ClosingChecklistPanel
                periodeId={id}
                statusPeriode={periode.statusPeriode}
                onAllPassed={setAllChecklistPassed}
                onSnapshotClick={setSnapshotDialogId}
              />
            )}

            {/* MV Refresh status (post-hard-close) */}
            {periode.statusPeriode === "CLOSED" && (
              <MvRefreshStatusCard
                jobId={mvRefreshJobId}
                onRefreshComplete={() => {
                  void qc.invalidateQueries({ queryKey: ["periode-buku", "detail", id] });
                }}
              />
            )}
          </div>

          {/* Right column: workflow action bar */}
          <div className="space-y-4">
            <ClosingWorkflowActionBar
              periodeId={id}
              periodeKode={periode.periodeKode}
              statusPeriode={periode.statusPeriode}
              softCloseRequestedBy={periode.softCloseRequestedBy ?? null}
              rowVersion={periode.rowVersion}
              allChecklistPassed={allChecklistPassed}
              onTransitionSuccess={() => {
                invalidate();
                // If hard-close just completed, the response includes mvRefreshJobId
                // The parent would need to extract it from the response; for now we
                // rely on the invalidate to re-fetch and show MvRefreshStatusCard
              }}
            />

            {/* Periode stats side card */}
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Riwayat Workflow
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2 text-sm">
                {periode.softCloseRequestedAt && (
                  <div className="flex items-center justify-between">
                    <span className="text-muted-foreground text-xs">Request Soft-Close</span>
                    <span className="text-xs">{fmtDate(periode.softCloseRequestedAt)}</span>
                  </div>
                )}
                {periode.softCloseApprovedAt && (
                  <>
                    <Separator />
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground text-xs">Soft-Close Approved</span>
                      <span className="text-xs">{fmtDate(periode.softCloseApprovedAt)}</span>
                    </div>
                  </>
                )}
                {periode.hardCloseRequestedAt && (
                  <>
                    <Separator />
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground text-xs">Request Hard-Close</span>
                      <span className="text-xs">{fmtDate(periode.hardCloseRequestedAt)}</span>
                    </div>
                  </>
                )}
                {periode.hardCloseApprovedAt && (
                  <>
                    <Separator />
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground text-xs">Hard-Close Approved</span>
                      <span className="text-xs">{fmtDate(periode.hardCloseApprovedAt)}</span>
                    </div>
                  </>
                )}
                {periode.reopenedAt && (
                  <>
                    <Separator />
                    <div className="flex items-center justify-between">
                      <span className="text-muted-foreground text-xs">Reopen</span>
                      <span className="text-xs">{fmtDate(periode.reopenedAt)}</span>
                    </div>
                  </>
                )}
                {!periode.softCloseRequestedAt && !periode.hardCloseRequestedAt && (
                  <p className="text-xs text-muted-foreground">
                    Belum ada riwayat workflow untuk periode ini.
                  </p>
                )}
              </CardContent>
            </Card>
          </div>
        </div>
      </div>

      {/* Checklist snapshot detail dialog */}
      {snapshotDialogId && (
        <ChecklistSnapshotDetailDialog
          open={!!snapshotDialogId}
          onOpenChange={(o) => { if (!o) setSnapshotDialogId(null); }}
          periodeId={id}
          snapshotId={snapshotDialogId}
          periodeKode={periode.periodeKode}
        />
      )}
    </div>
  );
}

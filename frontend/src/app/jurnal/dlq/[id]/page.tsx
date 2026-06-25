"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { DLQActionPanel } from "@/components/blips/jurnal/DLQActionPanel";
import { JSONBTreeView } from "@/components/blips/JSONBTreeView";
import { MFAStepUpModal } from "@/components/blips/MFAStepUpModal";
import { dlqApi } from "@/lib/api/jurnal.api";
import type { DlqStatus } from "@/lib/schemas/jurnal.schema";
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";

const STATUS_LABELS: Record<DlqStatus, string> = {
  FAILED: "Gagal",
  REPLAYING: "Sedang Diulang",
  REPLAYED_OK: "Berhasil Diulang",
  ABANDONED: "Diabaikan",
};

const STATUS_VARIANT: Record<DlqStatus, string> = {
  FAILED: "destructive",
  REPLAYING: "secondary",
  REPLAYED_OK: "default",
  ABANDONED: "outline",
};

export default function DLQDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [mfaOpen, setMfaOpen] = React.useState(false);

  const { data, isLoading, error } = useQuery({
    queryKey: ["dlq-entry", id],
    queryFn: () => dlqApi.get(id),
  });

  const entry = data?.data;

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["dlq-entry", id] });

  const replayMutation = useMutation({
    mutationFn: (_stepUpToken: string) => dlqApi.replay(id),
    onSuccess: (res) => {
      notify.success(
        `DLQ-${id} berhasil di-replay ke GL Host. Status: DELIVERED.`,
        {
          action: {
            label: "Lihat status job",
            onClick: () => router.push(`/jobs/${res.data.jobId}`),
          },
        },
      );
      invalidate();
    },
    onError: (err) => {
      if (isApiError(err)) notify.error(err);
    },
  });

  const discardMutation = useMutation({
    mutationFn: (reason: string) => dlqApi.discard(id, { discardReason: reason }),
    onSuccess: () => {
      notify.success("Entri DLQ berhasil diabaikan (ABANDONED).");
      invalidate();
    },
    onError: (err) => {
      if (isApiError(err)) notify.error(err);
    },
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-24 text-sm text-muted-foreground">
        Memuat data DLQ...
      </div>
    );
  }

  if (error || !entry) {
    return (
      <div className="flex flex-col items-center py-24 gap-4">
        <p className="text-sm text-muted-foreground">Entri DLQ tidak ditemukan.</p>
        <Button variant="outline" onClick={() => router.back()}>Kembali</Button>
      </div>
    );
  }

  const canReplay = can("jurnal.dlq.replay");
  const canDiscard = can("jurnal.dlq.read");

  return (
    <div className="mx-auto max-w-4xl px-6 py-6 space-y-6">
      {/* Top nav */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.push("/jurnal/dlq")}
          aria-label="Kembali ke daftar DLQ"
        >
          <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
          Daftar DLQ
        </Button>
      </div>

      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-lg font-semibold font-mono">{entry.sourceEventType}</h1>
            <Badge
              variant={
                (STATUS_VARIANT[entry.status] as "default" | "secondary" | "destructive" | "outline") ?? "outline"
              }
            >
              {STATUS_LABELS[entry.status]}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground mt-0.5">
            Event: <span className="font-mono font-medium">{entry.eventCode}</span>
            {entry.instrumenId && (
              <> · Instrumen: <span className="font-mono">{entry.instrumenId}</span></>
            )}
          </p>
        </div>
        <div className="text-right text-xs text-muted-foreground">
          <p>Percobaan: {entry.attemptCount}&times;</p>
          <p>Terakhir: {new Date(entry.lastAttemptAt).toLocaleDateString("id-ID")}</p>
        </div>
      </div>

      <div className="grid grid-cols-[1fr_300px] gap-6">
        {/* Left — error info + payload */}
        <div className="space-y-4">
          {/* Error info */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm text-red-700">Informasi Kegagalan</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div>
                <p className="text-xs text-muted-foreground">Kode Error</p>
                <p className="font-mono text-sm font-medium text-red-700">{entry.errorCode}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Pesan Error</p>
                <p className="text-sm">{entry.errorMessage}</p>
              </div>
              {entry.replayedJurnalId && (
                <div>
                  <p className="text-xs text-muted-foreground">Jurnal Hasil Replay</p>
                  <Button
                    variant="link"
                    className="h-auto p-0 text-sm font-mono"
                    onClick={() => router.push(`/jurnal/header/${entry.replayedJurnalId}`)}
                  >
                    {entry.replayedJurnalId}
                  </Button>
                </div>
              )}
              {entry.abandonedReason && (
                <div>
                  <p className="text-xs text-muted-foreground">Alasan Discard</p>
                  <p className="text-sm">{entry.abandonedReason}</p>
                </div>
              )}
            </CardContent>
          </Card>

          {/* Payload */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm">Payload Asli</CardTitle>
            </CardHeader>
            <CardContent>
              <JSONBTreeView
                data={entry.payloadJsonb}
                maxDepth={4}
                initiallyExpanded={false}
              />
            </CardContent>
          </Card>

          {/* Metadata */}
          <div className="text-xs text-muted-foreground flex flex-wrap gap-4 pt-2">
            <span>Source ID: <span className="font-mono">{entry.sourceEventId}</span></span>
            <span>Periode: <span className="font-mono">{entry.periodeId ?? "-"}</span></span>
            <span>Dibuat: {new Date(entry.createdAt).toLocaleDateString("id-ID")}</span>
          </div>
        </div>

        {/* Right — action panel */}
        <div>
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Aksi DLQ</CardTitle>
            </CardHeader>
            <CardContent>
              <DLQActionPanel
                dlqId={entry.id}
                status={entry.status}
                retryCount={entry.attemptCount}
                maxRetry={3}
                canReplay={canReplay}
                canDiscard={canDiscard}
                onReplay={async () => {
                  // DEC-027: step-up MFA required for DLQ replay
                  setMfaOpen(true);
                }}
                onDiscard={async (reason) => {
                  await discardMutation.mutateAsync(reason);
                }}
              />
            </CardContent>
          </Card>
        </div>
      </div>

      {/* MFA step-up modal for DLQ replay (DEC-027) */}
      <MFAStepUpModal
        open={mfaOpen}
        onOpenChange={setMfaOpen}
        title="Verifikasi MFA untuk Replay DLQ ke GL"
        description="Replay DLQ adalah aksi sensitif (DEC-027). Verifikasi MFA step-up diperlukan."
        onVerified={(stepUpToken) => {
          setMfaOpen(false);
          replayMutation.mutate(stepUpToken);
        }}
        onCancel={() => setMfaOpen(false)}
      />
    </div>
  );
}

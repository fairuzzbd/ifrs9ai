"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { JSONBTreeView } from "@/components/blips/JSONBTreeView";
import { GlDlqActionPanel } from "@/components/blips/gl-delivery/GlDlqActionPanel";
import { GlStatusBadge } from "@/components/blips/gl-delivery/GlStatusBadge";
import { GlFailureCategoryBadge } from "@/components/blips/gl-delivery/GlFailureCategoryBadge";
import { glDeliveryDlqApi } from "@/lib/api/gl-delivery.api";
import { usePermissions } from "@/lib/stores/auth.store";
import { GL_HOST_STATUS_LABELS } from "@/lib/schemas/gl-delivery.schema";

// ---------------------------------------------------------------------------
// PII fields to redact in JSONBTreeView (S5-AC4)
// ---------------------------------------------------------------------------
const PII_FIELDS = new Set(["customer_name", "account_no", "npwp"]);

function fmt(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("id-ID", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "Asia/Jakarta",
  });
}

// ---------------------------------------------------------------------------
// Page (S5-AC1, S5-AC2, S5-AC3, S5-AC4)
// ---------------------------------------------------------------------------

export default function GlDlqDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const qc = useQueryClient();
  const { can, hasRole } = usePermissions();

  const canReplay = can("jurnal.gl_delivery.dlq.replay");
  // S5-AC4: discard button NOT in DOM for non-ROLE-IT-ADMIN
  const canDiscard = hasRole("ROLE-IT-ADMIN");
  const canViewRaw = hasRole("ROLE-IT-ADMIN");

  const { data, isLoading, error } = useQuery({
    queryKey: ["gl-dlq-entry", id],
    queryFn: () => glDeliveryDlqApi.get(id),
  });

  const entry = data?.data;

  const invalidate = () =>
    void qc.invalidateQueries({ queryKey: ["gl-dlq-entry", id] });

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
        <p className="text-sm text-muted-foreground">Entri DLQ GL tidak ditemukan.</p>
        <Button variant="outline" onClick={() => router.back()}>Kembali</Button>
      </div>
    );
  }

  // Sanitize PII from payload for non-IT-ADMIN
  const sanitizedPayload = !canViewRaw && entry.payloadSnapshotJsonb
    ? (sanitizePii(entry.payloadSnapshotJsonb) as Record<string, unknown>)
    : entry.payloadSnapshotJsonb;

  const sanitizedResponse = !canViewRaw && entry.glResponsePayloadJsonb
    ? (sanitizePii(entry.glResponsePayloadJsonb) as Record<string, unknown>)
    : entry.glResponsePayloadJsonb;

  return (
    <div className="mx-auto max-w-5xl px-6 py-6 space-y-6">
      {/* Top nav */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.push("/jrnl/gl-delivery-dlq")}
          aria-label="Kembali ke daftar DLQ GL"
        >
          <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
          DLQ GL
        </Button>
      </div>

      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-3 flex-wrap">
            <h1 className="text-xl font-semibold font-mono">{entry.noJurnal}</h1>
            <GlStatusBadge status={entry.glHostStatus} />
            {entry.failureCategory && (
              <GlFailureCategoryBadge category={entry.failureCategory} />
            )}
          </div>
          <p className="text-sm text-muted-foreground mt-0.5">
            GL Status:{" "}
            <span className="font-medium">
              {GL_HOST_STATUS_LABELS[entry.glHostStatus] ?? entry.glHostStatus}
            </span>{" "}
            · {entry.retryCount} retry
          </p>
        </div>
        <div className="text-right text-xs text-muted-foreground">
          <p>Masuk DLQ: {fmt(entry.createdAt)}</p>
          <p>Retry Terakhir: {fmt(entry.lastRetryAt)}</p>
        </div>
      </div>

      {/* 2-col layout: left = details + payload, right = action panel */}
      <div className="grid grid-cols-[1fr_320px] gap-6 items-start">
        {/* Left col */}
        <div className="space-y-4">
          {/* Error info */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm text-red-700">Informasi Kegagalan</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <dl className="grid grid-cols-[160px_1fr] gap-x-4 gap-y-2 text-sm">
                <dt className="text-muted-foreground">Error Code</dt>
                <dd className="font-mono font-medium text-red-700">{entry.errorCode}</dd>
                <dt className="text-muted-foreground">Pesan Error</dt>
                <dd className="text-sm">{entry.errorMessage}</dd>
                <dt className="text-muted-foreground">Jurnal Header ID</dt>
                <dd>
                  <Button
                    variant="link"
                    className="h-auto p-0 text-sm font-mono"
                    onClick={() =>
                      router.push(`/jrnl/journal-entries/${entry.jurnalHeaderId}`)
                    }
                  >
                    {entry.jurnalHeaderId}
                  </Button>
                </dd>
                {entry.discardInfo?.discardReason && (
                  <>
                    <dt className="text-muted-foreground">Alasan Discard</dt>
                    <dd className="text-sm">{entry.discardInfo.discardReason}</dd>
                  </>
                )}
              </dl>
            </CardContent>
          </Card>

          {/* GL Request Payload */}
          {entry.payloadSnapshotJsonb && (
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm">
                  Payload Dikirim ke GL Host
                  {!canViewRaw && (
                    <span className="ml-2 text-xs font-normal text-muted-foreground">
                      (PII disembunyikan)
                    </span>
                  )}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <JSONBTreeView
                  data={sanitizedPayload!}
                  maxDepth={4}
                  initiallyExpanded={false}
                />
              </CardContent>
            </Card>
          )}

          {/* GL Response Payload */}
          {entry.glResponsePayloadJsonb && (
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm">
                  Respons dari GL Host
                  {!canViewRaw && (
                    <span className="ml-2 text-xs font-normal text-muted-foreground">
                      (PII disembunyikan)
                    </span>
                  )}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <JSONBTreeView
                  data={sanitizedResponse!}
                  maxDepth={3}
                  initiallyExpanded={false}
                />
              </CardContent>
            </Card>
          )}
        </div>

        {/* Right col — action panel */}
        <div className="sticky top-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Aksi DLQ GL</CardTitle>
            </CardHeader>
            <CardContent>
              <GlDlqActionPanel
                dlqId={entry.dlqEntryId}
                jurnalNumber={entry.noJurnal}
                status={entry.glHostStatus}
                retryCount={entry.retryCount}
                maxRetry={5}
                canReplay={canReplay}
                canDiscard={canDiscard}
                onReplay={invalidate}
                onDiscard={invalidate}
              />
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// PII sanitizer — recursively replace PII_FIELDS with [REDACTED]
// ---------------------------------------------------------------------------

function sanitizePii(obj: unknown): unknown {
  if (obj === null || typeof obj !== "object") return obj;

  if (Array.isArray(obj)) {
    return obj.map(sanitizePii);
  }

  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    out[k] = PII_FIELDS.has(k) ? "[REDACTED]" : sanitizePii(v);
  }
  return out;
}

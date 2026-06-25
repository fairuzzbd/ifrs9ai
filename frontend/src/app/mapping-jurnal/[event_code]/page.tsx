/**
 * Route: /mapping-jurnal/[event_code]
 * Story: P5-M12-S1-AC2, S2 — detail + version history + 6-eyes action panel
 * Actors: ROLE-AKUN (submit, new version), ROLE-AKUN-CTL (review, approve-4-eyes),
 *         ROLE-RISK (approve-2 regulated), ROLE-AUDIT (read-only)
 */

"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { v4 as uuidv4 } from "uuid";
import { Pencil } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";

import { MappingStatusBadge } from "@/components/blips/mapping-jurnal/MappingStatusBadge";
import { MappingRegulatedBadge } from "@/components/blips/mapping-jurnal/MappingRegulatedBadge";
import { MappingDetailTable } from "@/components/blips/mapping-jurnal/MappingDetailTable";
import { MappingVersionTimeline } from "@/components/blips/mapping-jurnal/MappingVersionTimeline";
import { MappingSubmitDialog } from "@/components/blips/mapping-jurnal/MappingSubmitDialog";
import { MappingReviewDialog } from "@/components/blips/mapping-jurnal/MappingReviewDialog";
import { MappingApproveDialog } from "@/components/blips/mapping-jurnal/MappingApproveDialog";
import { MappingRejectDialog } from "@/components/blips/mapping-jurnal/MappingRejectDialog";
import { SodBlockBanner } from "@/components/blips/SodBlockBanner";
import { PeriodeLockBanner } from "@/components/blips/periode-close/PeriodeLockBanner";

import {
  mappingP12Api,
  mappingWorkflowApi,
  mappingP12QueryKeys,
} from "@/lib/api/mapping-jurnal-p12.api";
import { notify } from "@/lib/notify";
import { useCurrentUser } from "@/lib/stores/auth.store";

export default function MappingJurnalDetailPage() {
  const { event_code: eventCode } = useParams<{ event_code: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const currentUser = useCurrentUser();

  const { data, isLoading, isError } = useQuery({
    queryKey: mappingP12QueryKeys.detail(eventCode),
    queryFn: () => mappingP12Api.get(eventCode),
    enabled: !!eventCode,
  });

  const item = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: mappingP12QueryKeys.detail(eventCode) });
    void queryClient.invalidateQueries({ queryKey: mappingP12QueryKeys.all });
  };

  // Compute active version id (latest non-DRAFT, or latest DRAFT if no active)
  const versionId = item?.id ?? "";

  // SoD guards
  const isMaker = currentUser?.id === item?.makerId;
  const isReviewer = currentUser?.id === item?.reviewerId;
  const isApprover = currentUser?.id === item?.approverId;

  const permissions = currentUser?.permissions ?? [];

  const canSubmit =
    item?.workflowStatus === "DRAFT" && permissions.includes("jurnal.mapping.create") && isMaker;
  const canReview =
    item?.workflowStatus === "PENDING_REVIEW" &&
    permissions.includes("jurnal.mapping.review") &&
    !isMaker;
  const canApprove =
    item?.workflowStatus === "PENDING_APPROVAL" &&
    permissions.includes("jurnal.mapping.approve") &&
    !isMaker &&
    !isReviewer;
  const canApprove2 =
    item?.workflowStatus === "PENDING_APPROVAL_2" &&
    permissions.includes("jurnal.mapping.approve_2") &&
    !isMaker &&
    !isReviewer &&
    !isApprover;
  const canNewVersion =
    item?.workflowStatus === "APPROVED_ACTIVE" && permissions.includes("jurnal.mapping.create");

  const reviewSodBlock = item?.workflowStatus === "PENDING_REVIEW" && isMaker;
  const approveSodBlock = item?.workflowStatus === "PENDING_APPROVAL" && (isMaker || isReviewer);
  const approve2SodBlock =
    item?.workflowStatus === "PENDING_APPROVAL_2" && (isMaker || isReviewer || isApprover);

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-80" />
        <div className="grid grid-cols-1 lg:grid-cols-[3fr_2fr] gap-6">
          <Skeleton className="h-64" />
          <Skeleton className="h-64" />
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">Gagal memuat mapping jurnal.</p>
        <Button variant="outline" asChild>
          <Link href="/mapping-jurnal">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const periodeLocked = false; // runtime: would check from context/query

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/mapping-jurnal" className="hover:underline">Mapping Jurnal</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{eventCode}</span>
      </nav>

      {periodeLocked && (
        <PeriodeLockBanner message="Periode buku HARD_CLOSED — perubahan mapping tidak dapat diaktifkan saat ini." />
      )}

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3 flex-wrap">
          <h1 className="text-2xl font-semibold">{item.namaEvent}</h1>
          <code className="font-mono text-sm text-muted-foreground">{item.eventCode}</code>
          <MappingStatusBadge status={item.workflowStatus} />
          <MappingRegulatedBadge regulated={item.regulatedFlag} workflowPath={item.workflowPath} />
          {item.aktifFlag && (
            <Badge variant="outline" className="text-xs text-green-700 border-green-200 bg-green-50">
              Aktif
            </Badge>
          )}
        </div>
        {canNewVersion && (
          <Button
            size="sm"
            variant="outline"
            asChild
            aria-label={`Buat versi baru mapping ${eventCode}`}
          >
            <Link href={`/mapping-jurnal/${eventCode}?edit=true`}>
              <Pencil className="mr-1.5 h-4 w-4" aria-hidden="true" />
              Edit (Versi Baru)
            </Link>
          </Button>
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[3fr_2fr] gap-6">
        {/* Left: detail rows + version history */}
        <div className="space-y-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground">
                Detail Jurnal ({item.detail.length} baris)
              </CardTitle>
            </CardHeader>
            <CardContent>
              <MappingDetailTable rows={item.detail} />
            </CardContent>
          </Card>

          {item.rejectReason && (
            <div className="rounded-md border border-red-200 bg-red-50 p-3">
              <p className="text-xs font-medium text-red-700">Alasan Penolakan:</p>
              <p className="text-xs text-red-600 mt-1">{item.rejectReason}</p>
            </div>
          )}

          {item.versionHistory.length > 0 && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground">
                  Riwayat Versi ({item.versionHistory.length})
                </CardTitle>
              </CardHeader>
              <CardContent>
                <MappingVersionTimeline
                  versions={[item, ...item.versionHistory]}
                  currentVersionId={item.id}
                  eventCode={eventCode}
                />
              </CardContent>
            </Card>
          )}
        </div>

        {/* Right: workflow panel */}
        <div>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground">
                Proses Persetujuan ({item.workflowPath})
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {(reviewSodBlock || approveSodBlock || approve2SodBlock) && (
                <SodBlockBanner
                  message={
                    reviewSodBlock
                      ? "Anda tidak bisa mereview submission Anda sendiri (DEC-017)."
                      : approve2SodBlock
                        ? "Anda terlibat di step sebelumnya — tidak bisa menjadi approver-2 (DEC-017)."
                        : "Anda tidak bisa menyetujui data yang Anda buat atau review sendiri (DEC-017)."
                  }
                />
              )}

              {canSubmit && (
                <MappingSubmitDialog
                  eventCode={eventCode}
                  versionId={versionId}
                  onSubmit={async (comment) => {
                    await mappingWorkflowApi.submit(eventCode, versionId, { comment }, uuidv4());
                    notify.success(`Mapping ${eventCode} berhasil disubmit untuk review.`);
                    invalidate();
                  }}
                />
              )}

              {canReview && (
                <div className="flex gap-2">
                  <MappingReviewDialog
                    eventCode={eventCode}
                    isRegulated={item.regulatedFlag}
                    onReview={async (comment) => {
                      await mappingWorkflowApi.review(
                        eventCode,
                        versionId,
                        { comment, signatureMethod: "JWT_STEP_UP" },
                        uuidv4(),
                      );
                      const nextStatus = item.regulatedFlag ? "PENDING_APPROVAL_2 (ROLE-RISK)" : "PENDING_APPROVAL";
                      notify.success(`Mapping ${eventCode} berhasil di-review. Status: ${nextStatus}.`);
                      invalidate();
                    }}
                  />
                  <MappingRejectDialog
                    eventCode={eventCode}
                    onReject={async (reason) => {
                      await mappingWorkflowApi.reject(
                        eventCode,
                        versionId,
                        { reason },
                        uuidv4(),
                      );
                      notify.warning(`Mapping ${eventCode} ditolak dan dikembalikan ke Maker.`);
                      invalidate();
                    }}
                  />
                </div>
              )}

              {canApprove && (
                <div className="flex gap-2">
                  <MappingApproveDialog
                    eventCode={eventCode}
                    variant="4-eyes"
                    onApprove={async (comment) => {
                      await mappingWorkflowApi.approve(
                        eventCode,
                        versionId,
                        { comment, signatureMethod: "JWT_STEP_UP" },
                        uuidv4(),
                      );
                      notify.success(`Mapping ${eventCode} disetujui dan aktif. Resolver dapat menggunakan template ini.`);
                      invalidate();
                    }}
                  />
                  <MappingRejectDialog
                    eventCode={eventCode}
                    onReject={async (reason) => {
                      await mappingWorkflowApi.reject(eventCode, versionId, { reason }, uuidv4());
                      notify.warning(`Mapping ${eventCode} ditolak dan dikembalikan ke Maker.`);
                      invalidate();
                    }}
                  />
                </div>
              )}

              {canApprove2 && (
                <div className="flex gap-2">
                  <MappingApproveDialog
                    eventCode={eventCode}
                    variant="6-eyes"
                    onApprove={async (comment, stepUpToken) => {
                      if (!stepUpToken) throw new Error("Step-up token diperlukan");
                      await mappingWorkflowApi.approve2(
                        eventCode,
                        versionId,
                        { comment, signatureMethod: "JWT_STEP_UP" },
                        stepUpToken,
                        uuidv4(),
                      );
                      notify.success(`Mapping ${eventCode} disetujui dan aktif. Resolver dapat menggunakan template ini.`);
                      invalidate();
                      router.refresh();
                    }}
                  />
                  <MappingRejectDialog
                    eventCode={eventCode}
                    onReject={async (reason) => {
                      await mappingWorkflowApi.reject(eventCode, versionId, { reason }, uuidv4());
                      notify.warning(`Mapping ${eventCode} ditolak oleh ROLE-RISK.`);
                      invalidate();
                    }}
                  />
                </div>
              )}

              {item.workflowStatus === "APPROVED_ACTIVE" && (
                <div className="flex items-center gap-2 rounded-md border border-green-200 bg-green-50 p-3 text-xs text-green-700">
                  <span className="font-medium">Mapping aktif — Siap digunakan oleh resolver jurnal.</span>
                </div>
              )}

              {/* Signature timestamps */}
              <div className="space-y-1 text-xs text-muted-foreground border-t pt-3">
                {item.reviewerSignedAt && (
                  <p>Reviewer sign: {new Date(item.reviewerSignedAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}</p>
                )}
                {item.approverSignedAt && (
                  <p>Approver sign: {new Date(item.approverSignedAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}</p>
                )}
                {item.approver2SignedAt && (
                  <p>Approver-2 (RISK) sign: {new Date(item.approver2SignedAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}</p>
                )}
              </div>
            </CardContent>
          </Card>

          <div className="mt-3">
            <Link
              href={`/reports/mapping-history?filter[event_code]=${eventCode}`}
              className="text-xs text-primary hover:underline"
            >
              Lihat riwayat audit RPT-21 &rarr;
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}

"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Pencil, ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { WorkflowPathBadge } from "@/components/blips/jurnal/WorkflowPathBadge";
import { SixEyesWorkflowPanel } from "@/components/blips/jurnal/SixEyesWorkflowPanel";
import { MappingDetailRowsBuilder } from "@/components/blips/jurnal/MappingDetailRowsBuilder";
import { BalancePreviewCard } from "@/components/blips/jurnal/BalancePreviewCard";
import { mappingApi } from "@/lib/api/jurnal.api";
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import type { MappingDetailRow, KlasifikasiPsak71 } from "@/lib/schemas/jurnal.schema";
import { isRegulatedCode } from "@/lib/schemas/jurnal.schema";

// minimal session hook shim — replace with real useSession if NextAuth is wired
function useCurrentUser() {
  return {
    id: "current-user-placeholder",
    permissions: [
      "jurnal_mapping.create",
      "jurnal_mapping.review",
      "jurnal_mapping.approve",
      "jurnal_mapping.approve_2",
    ],
  };
}

export default function MappingJurnalDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const qc = useQueryClient();
  const currentUser = useCurrentUser();

  const { data, isLoading, error } = useQuery({
    queryKey: ["mapping-jurnal", id],
    queryFn: () => mappingApi.get(id),
  });

  const mapping = data?.data;

  const invalidate = () => qc.invalidateQueries({ queryKey: ["mapping-jurnal", id] });

  const submitMutation = useMutation({
    mutationFn: (comment: string) =>
      mappingApi.submit(id, { comment }),
    onSuccess: () => {
      notify.success("Mapping berhasil di-submit. Menunggu review.");
      void invalidate();
    },
    onError: (err) => isApiError(err) && notify.error(err),
  });

  const reviewMutation = useMutation({
    mutationFn: (comment: string) =>
      mappingApi.review(id, { comment, signatureMethod: "JWT_STANDARD" }),
    onSuccess: () => {
      notify.success("Review berhasil. Mapping menunggu persetujuan.");
      void invalidate();
    },
    onError: (err) => isApiError(err) && notify.error(err),
  });

  const approveMutation = useMutation({
    mutationFn: ({ comment, mfa }: { comment: string; mfa?: string }) =>
      mappingApi.approve(
        id,
        { comment, signatureMethod: mfa ? "JWT_STEP_UP" : "JWT_STANDARD" },
        mfa,
      ),
    onSuccess: () => {
      notify.success("Mapping berhasil disetujui.");
      void invalidate();
    },
    onError: (err) => isApiError(err) && notify.error(err),
  });

  const approve2Mutation = useMutation({
    mutationFn: ({ comment, mfa }: { comment: string; mfa: string }) =>
      mappingApi.approve2(id, { comment, signatureMethod: "JWT_STEP_UP" }, mfa),
    onSuccess: () => {
      notify.success("Persetujuan kedua berhasil. Mapping sekarang AKTIF.");
      void invalidate();
    },
    onError: (err) => isApiError(err) && notify.error(err),
  });

  const rejectMutation = useMutation({
    mutationFn: (reason: string) =>
      mappingApi.reject(id, { rejectReason: reason, signatureMethod: "JWT_STANDARD" }),
    onSuccess: () => {
      notify.destructive("Mapping ditolak dan dikembalikan ke Maker.");
      void invalidate();
    },
    onError: (err) => isApiError(err) && notify.error(err),
  });

  const withdrawMutation = useMutation({
    mutationFn: () => mappingApi.withdraw(id),
    onSuccess: () => {
      notify.info("Mapping berhasil ditarik ke Draft.");
      void invalidate();
    },
    onError: (err) => isApiError(err) && notify.error(err),
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-24 text-sm text-muted-foreground">
        Memuat data mapping...
      </div>
    );
  }

  if (error || !mapping) {
    return (
      <div className="flex flex-col items-center py-24 gap-4">
        <p className="text-sm text-muted-foreground">Mapping tidak ditemukan.</p>
        <Button variant="outline" onClick={() => router.back()}>
          Kembali
        </Button>
      </div>
    );
  }

  const isRegulated = isRegulatedCode(mapping.eventCode);

  const detailRowsForBuilder: MappingDetailRow[] = mapping.detailRows.map((r) => ({
    id: r.id,
    _clientKey: r.id,
    urutan: r.urutan,
    dkIndicator: r.dkIndicator,
    kodeAkunId: r.kodeAkunId,
    kodeAkunDisplay: r.kodeAkunDisplay,
    namaAkun: r.namaAkun,
    sumberAmount: r.sumberAmount,
    multiplier: r.multiplier,
    klasifikasiFilter: r.klasifikasiFilter as KlasifikasiPsak71 | null,
    catatan: r.catatan ?? "",
  }));

  const canEdit = mapping.workflowStatus === "DRAFT";

  return (
    <div className="mx-auto max-w-6xl px-6 py-6 space-y-6">
      {/* Top nav */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.push("/jrnl/mapping")}
          aria-label="Kembali ke daftar mapping"
        >
          <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
          Daftar Mapping
        </Button>
        <div className="flex-1" />
        {canEdit && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => router.push(`/jrnl/mapping/${id}/edit`)}
          >
            <Pencil className="h-4 w-4 mr-1.5" aria-hidden="true" />
            Edit
          </Button>
        )}
      </div>

      {/* Title row */}
      <div className="flex items-start gap-4">
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-semibold font-mono">{mapping.eventCode}</h1>
            <WorkflowPathBadge path={mapping.workflowPath} size="md" showTooltip />
          </div>
          <p className="text-muted-foreground mt-0.5">{mapping.namaEvent}</p>
          <p className="text-xs text-muted-foreground mt-1">
            {mapping.kategoriEvent} · Trigger: {mapping.triggerSource}
          </p>
        </div>
        <div className="flex flex-col items-end gap-1">
          <Badge
            variant={mapping.aktifFlag ? "default" : "secondary"}
            className="text-xs"
          >
            {mapping.aktifFlag ? "AKTIF" : "NONAKTIF"}
          </Badge>
          {mapping.klasifikasiBerlaku && mapping.klasifikasiBerlaku.length > 0 && (
            <div className="flex gap-1 flex-wrap justify-end">
              {mapping.klasifikasiBerlaku.map((k) => (
                <Badge key={k} variant="outline" className="text-xs">
                  {k}
                </Badge>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="grid grid-cols-[1fr_320px] gap-6">
        {/* Left: detail rows */}
        <div className="space-y-4">
          {mapping.deskripsi && (
            <p className="text-sm text-muted-foreground border rounded p-3 bg-muted/20">
              {mapping.deskripsi}
            </p>
          )}

          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Template Baris Jurnal ({mapping.detailRows.length} baris)</CardTitle>
            </CardHeader>
            <CardContent>
              <MappingDetailRowsBuilder
                value={detailRowsForBuilder}
                onChange={() => {}}
                disabled
              />
              <div className="mt-4">
                <BalancePreviewCard rows={detailRowsForBuilder} />
              </div>
            </CardContent>
          </Card>

          {/* Audit info */}
          <div className="text-xs text-muted-foreground flex gap-6 pt-2">
            <span>Dibuat: {new Date(mapping.createdAt).toLocaleDateString("id-ID")}</span>
            <span>Diperbarui: {new Date(mapping.updatedAt).toLocaleDateString("id-ID")}</span>
            <span>Versi: {mapping.rowVersion}</span>
          </div>
        </div>

        {/* Right: workflow panel */}
        <div>
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Workflow {isRegulated ? "6-Eyes" : "4-Eyes"}</CardTitle>
            </CardHeader>
            <CardContent>
              <SixEyesWorkflowPanel
                workflowPath={mapping.workflowPath}
                currentStatus={mapping.workflowStatus}
                maker={
                  mapping.makerId
                    ? { id: mapping.makerId, nama: mapping.makerNama ?? mapping.makerId, role: "ROLE-AKUN", signedAt: null }
                    : null
                }
                reviewer={
                  mapping.reviewerId
                    ? { id: mapping.reviewerId, nama: mapping.reviewerNama ?? mapping.reviewerId, role: "ROLE-AKUN-CTL", signedAt: mapping.reviewerSignedAt ?? null }
                    : null
                }
                approver={
                  mapping.approverId
                    ? { id: mapping.approverId, nama: mapping.approverNama ?? mapping.approverId, role: "ROLE-AKUN-CTL", signedAt: mapping.approverSignedAt ?? null }
                    : null
                }
                approver2={
                  mapping.approver2Id
                    ? { id: mapping.approver2Id, nama: mapping.approver2Nama ?? mapping.approver2Id, role: "ROLE-RISK", signedAt: mapping.approver2SignedAt ?? null }
                    : null
                }
                rejectReason={mapping.rejectReason}
                currentUserId={currentUser.id}
                makerId={mapping.makerId}
                reviewerId={mapping.reviewerId}
                approverId={mapping.approverId}
                approver2Id={mapping.approver2Id}
                currentUserPermissions={currentUser.permissions}
                isRegulated={isRegulated}
                onSubmit={async (comment) => { await submitMutation.mutateAsync(comment); }}
                onReview={async (comment) => { await reviewMutation.mutateAsync(comment); }}
                onApprove={async (comment, mfa) => { await approveMutation.mutateAsync({ comment, mfa }); }}
                onApprove2={async (comment, mfa) => { await approve2Mutation.mutateAsync({ comment, mfa }); }}
                onReject={async (reason) => { await rejectMutation.mutateAsync(reason); }}
                onWithdraw={async () => { await withdrawMutation.mutateAsync(); }}
              />
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}

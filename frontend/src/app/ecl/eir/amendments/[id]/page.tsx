"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";

import { eirApi } from "@/lib/api/eir.api";
import {
  amendmentRejectFormSchema,
  type AmendmentRejectForm,
} from "@/lib/schemas/eir.schema";
import { CatchUpAdjustmentCard } from "@/components/blips/CatchUpAdjustmentCard";
import { RoutingPathBadge } from "@/components/blips/RoutingPathBadge";
import type { TriggerSource } from "@/components/blips/RoutingPathBadge";
import { MFAStepUpModal, getStepUpToken } from "@/components/blips/MFAStepUpModal";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Textarea } from "@/components/ui/textarea";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function fmtEir(s: string | null | undefined): string {
  if (!s) return "—";
  const n = parseFloat(s);
  if (isNaN(n)) return s;
  return `${(n * 100).toFixed(6)}%`;
}

const STATUS_LABELS: Record<string, { label: string; variant: "default" | "secondary" | "destructive" | "outline" }> = {
  DRAFT: { label: "Draft", variant: "secondary" },
  PENDING_REVIEW: { label: "Menunggu Review", variant: "default" },
  PENDING_APPROVAL: { label: "Menunggu Approval ALCO", variant: "default" },
  APPROVED: { label: "Disetujui", variant: "default" },
  REJECTED: { label: "Ditolak", variant: "destructive" },
};

// ---------------------------------------------------------------------------
// Reject dialog
// ---------------------------------------------------------------------------

function RejectDialog({
  onReject,
  isPending,
  title,
}: {
  onReject: (comment: string) => void;
  isPending: boolean;
  title: string;
}) {
  const [open, setOpen] = React.useState(false);
  const form = useForm<AmendmentRejectForm>({
    resolver: zodResolver(amendmentRejectFormSchema),
    defaultValues: { comment: "", signatureMethod: "JWT_STANDARD" },
  });

  const onSubmit = (data: AmendmentRejectForm) => {
    onReject(data.comment);
    setOpen(false);
    form.reset();
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="destructive" size="sm" disabled={isPending}>
          Tolak
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>Berikan alasan penolakan (minimal 20 karakter).</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Alasan Penolakan</FormLabel>
                  <FormControl>
                    <Textarea rows={3} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="flex gap-2 justify-end">
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                Batal
              </Button>
              <Button type="submit" variant="destructive" disabled={isPending}>
                Konfirmasi Tolak
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function EIRAmendmentDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const router = useRouter();
  const queryClient = useQueryClient();
  const { can, userId } = usePermissions();

  const [mfaOpen, setMfaOpen] = React.useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["eir-amendment", id],
    queryFn: () => eirApi.getAmendment(id),
    enabled: !!id,
  });

  const amendment = data?.data;

  const reviewMutation = useMutation({
    mutationFn: ({ comment }: { comment?: string }) =>
      eirApi.reviewAmendment(
        id,
        { action: "APPROVE", comment: comment ?? "", signatureMethod: "JWT_STANDARD" },
        uuidv4(),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["eir-amendment", id] });
      notify.success("Amandemen EIR berhasil di-review. Menunggu approval ALCO.");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const approveMutation = useMutation({
    mutationFn: ({ comment, stepUpToken }: { comment?: string; stepUpToken: string }) =>
      eirApi.approveAmendment(
        id,
        { action: "APPROVE", comment: comment ?? "", signatureMethod: "JWT_STEP_UP" },
        stepUpToken,
        uuidv4(),
      ),
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: ["eir-amendment", id] });
      const adj = res.data.catchUpAdjustment;
      notify.success(
        `Amandemen EIR disetujui. EIR baru: ${fmtEir(res.data.eirSesudah)}.${adj ? ` Catch-up adjustment: aktif.` : ""}`,
      );
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const rejectMutation = useMutation({
    mutationFn: ({ comment }: { comment: string }) =>
      eirApi.rejectAmendment(
        id,
        { comment, signatureMethod: "JWT_STANDARD" },
        uuidv4(),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["eir-amendment", id] });
      notify.success("Amandemen EIR ditolak.");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const cancelMutation = useMutation({
    mutationFn: ({ comment }: { comment: string }) =>
      eirApi.cancelAmendment(
        id,
        { comment, signatureMethod: "JWT_STANDARD" },
        uuidv4(),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["eir-amendment", id] });
      notify.success("Amandemen EIR dibatalkan.");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const handleApprove = () => {
    const token = getStepUpToken();
    if (token) {
      approveMutation.mutate({ stepUpToken: token });
    } else {
      setMfaOpen(true);
    }
  };

  const handleMFAVerified = (token: string) => {
    setMfaOpen(false);
    approveMutation.mutate({ stepUpToken: token });
  };

  const isMaker = amendment?.makerId === userId;
  const isReviewer = amendment?.reviewerId === userId;

  const isPending =
    reviewMutation.isPending ||
    approveMutation.isPending ||
    rejectMutation.isPending ||
    cancelMutation.isPending;

  if (isLoading) {
    return (
      <div className="p-6">
        <div className="h-40 rounded-lg bg-muted animate-pulse" />
      </div>
    );
  }

  if (!amendment) {
    return (
      <div className="p-6">
        <p>Amandemen tidak ditemukan.</p>
        <Button variant="link" onClick={() => router.push("/ecl/eir/amendments/queue")}>
          Kembali ke antrian
        </Button>
      </div>
    );
  }

  const cfg = STATUS_LABELS[amendment.workflowStatus] ?? { label: amendment.workflowStatus, variant: "secondary" as const };

  return (
    <div className="p-6 space-y-4 max-w-2xl">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex gap-1">
          <li>
            <button className="hover:underline" onClick={() => router.push("/ecl/eir/amendments/queue")}>
              Amandemen EIR
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">{id.slice(0, 8)}…</li>
        </ol>
      </nav>

      <div className="flex items-center gap-3 flex-wrap">
        <h1 className="text-xl font-semibold">Detail Amandemen EIR</h1>
        <Badge variant={cfg.variant}>{cfg.label}</Badge>
        <RoutingPathBadge triggerSource={amendment.triggerSource as TriggerSource} />
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Informasi Amandemen</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div>
              <p className="text-xs text-muted-foreground">Instrumen</p>
              <p className="font-medium">
                {amendment.kodeInstrumen ?? amendment.instrumenId.slice(0, 8)}
              </p>
              {amendment.namaInstrumen && (
                <p className="text-xs text-muted-foreground">{amendment.namaInstrumen}</p>
              )}
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Tanggal Amandemen</p>
              <p>{amendment.amendmentDate ?? "—"}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">EIR Sebelum</p>
              <p className="font-mono">{fmtEir(amendment.eirSebelum)}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">EIR Sesudah</p>
              <p className="font-mono text-primary">{fmtEir(amendment.eirSesudah)}</p>
            </div>
          </div>

          {amendment.alasan && (
            <div>
              <p className="text-xs text-muted-foreground">Alasan</p>
              <p className="text-sm whitespace-pre-wrap mt-0.5">{amendment.alasan}</p>
            </div>
          )}

          <div className="grid grid-cols-2 gap-3 text-sm">
            <div>
              <p className="text-xs text-muted-foreground">Dibuat</p>
              <p>{new Date(amendment.createdAt).toLocaleDateString("id-ID")}</p>
            </div>
            {amendment.approvedAt && (
              <div>
                <p className="text-xs text-muted-foreground">Disetujui</p>
                <p>{new Date(amendment.approvedAt).toLocaleDateString("id-ID")}</p>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Catch-up adjustment */}
      <CatchUpAdjustmentCard adjustment={amendment.catchUpAdjustment ?? null} />

      {/* Action bar */}
      <div className="flex flex-wrap gap-2">
        {/* Review — AKUN / RISK, not maker */}
        {amendment.workflowStatus === "PENDING_REVIEW" &&
          can("ecl_eir.amendment.review") &&
          !isMaker && (
            <Button size="sm" onClick={() => reviewMutation.mutate({})} disabled={isPending}>
              Setuju (Review)
            </Button>
          )}

        {/* Approve ALCO — step-up MFA required */}
        {amendment.workflowStatus === "PENDING_APPROVAL" &&
          can("ecl_eir.amendment.approve") &&
          !isMaker &&
          !isReviewer && (
            <Button size="sm" onClick={handleApprove} disabled={isPending}>
              Setuju (ALCO)
            </Button>
          )}

        {/* Reject */}
        {(amendment.workflowStatus === "PENDING_REVIEW" ||
          amendment.workflowStatus === "PENDING_APPROVAL") &&
          (can("ecl_eir.amendment.review") || can("ecl_eir.amendment.approve")) && (
            <RejectDialog
              title="Tolak Amandemen EIR?"
              onReject={(comment) => rejectMutation.mutate({ comment })}
              isPending={isPending}
            />
          )}

        {/* Cancel — maker only, if draft/pending */}
        {isMaker &&
          (amendment.workflowStatus === "DRAFT" ||
            amendment.workflowStatus === "PENDING_REVIEW") && (
            <RejectDialog
              title="Batalkan Amandemen EIR?"
              onReject={(comment) => cancelMutation.mutate({ comment })}
              isPending={isPending}
            />
          )}
      </div>

      {/* MFA modal */}
      <MFAStepUpModal
        open={mfaOpen}
        onOpenChange={setMfaOpen}
        title="Konfirmasi MFA — Approval ALCO"
        description="Masukkan kode OTP untuk menyelesaikan approval amandemen EIR."
        onVerified={handleMFAVerified}
        onCancel={() => setMfaOpen(false)}
      />
    </div>
  );
}

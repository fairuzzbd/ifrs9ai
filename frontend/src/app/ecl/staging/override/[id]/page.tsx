"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";

import { stagingApi } from "@/lib/api/staging.api";
import {
  workflowRejectFormSchema,
  type WorkflowRejectForm,
} from "@/lib/schemas/staging.schema";
import { StageBadge } from "@/components/blips/StageBadge";
import { MFAStepUpModal, getStepUpToken } from "@/components/blips/MFAStepUpModal";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
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

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

const STATUS_LABELS: Record<string, string> = {
  PENDING_REVIEW: "Menunggu Review",
  PENDING_APPROVAL: "Menunggu Approval",
  APPROVED_ALCO: "Disetujui ALCO",
  ACTIVE: "Aktif",
  EXPIRED: "Kadaluarsa",
  REJECTED: "Ditolak",
};

function statusVariant(
  status: string,
): "default" | "secondary" | "destructive" | "outline" {
  if (status === "ACTIVE" || status === "APPROVED_ALCO") return "default";
  if (status === "REJECTED" || status === "EXPIRED") return "destructive";
  if (status === "PENDING_REVIEW" || status === "PENDING_APPROVAL")
    return "secondary";
  return "outline";
}

// ---------------------------------------------------------------------------
// Reject dialog
// ---------------------------------------------------------------------------

interface RejectDialogProps {
  onReject: (comment: string) => void;
  isPending: boolean;
}

function RejectDialog({ onReject, isPending }: RejectDialogProps) {
  const [open, setOpen] = React.useState(false);
  const form = useForm<WorkflowRejectForm>({
    resolver: zodResolver(workflowRejectFormSchema),
    defaultValues: { comment: "", signatureMethod: "JWT_STANDARD" },
  });

  const onSubmit = (data: WorkflowRejectForm) => {
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
          <DialogTitle>Tolak Proposal Override?</DialogTitle>
          <DialogDescription>
            Berikan alasan penolakan (minimal 20 karakter).
          </DialogDescription>
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
              <Button
                type="button"
                variant="outline"
                onClick={() => setOpen(false)}
              >
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

export default function StagingOverrideDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const router = useRouter();
  const queryClient = useQueryClient();
  const { can, userId } = usePermissions();

  const [mfaOpen, setMfaOpen] = React.useState(false);
  const [pendingAction, setPendingAction] = React.useState<
    "approve" | "approve2" | null
  >(null);

  const { data, isLoading } = useQuery({
    queryKey: ["staging-override", id],
    queryFn: () => stagingApi.getOverride(id),
    enabled: !!id,
  });

  const override = data?.data;

  const reviewMutation = useMutation({
    mutationFn: ({ comment }: { comment?: string }) =>
      stagingApi.reviewOverride(
        id,
        { action: "APPROVE", comment: comment ?? "", signatureMethod: "JWT_STANDARD" },
        uuidv4(),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["staging-override", id] });
      notify.success("Proposal override berhasil di-review. Menunggu approval ALCO.");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const approveMutation = useMutation({
    mutationFn: ({ comment, stepUpToken }: { comment?: string; stepUpToken: string }) =>
      stagingApi.approveOverride(
        id,
        { action: "APPROVE", comment: comment ?? "", signatureMethod: "JWT_STEP_UP" },
        stepUpToken,
        uuidv4(),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["staging-override", id] });
      notify.success("Override staging disetujui ALCO.");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const approve2Mutation = useMutation({
    mutationFn: ({ comment, stepUpToken }: { comment?: string; stepUpToken: string }) =>
      stagingApi.approveOverride2(
        id,
        { action: "APPROVE", comment: comment ?? "", signatureMethod: "JWT_STEP_UP" },
        stepUpToken,
        uuidv4(),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["staging-override", id] });
      notify.success("Override staging disetujui KOMITE. Override aktif.");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const rejectMutation = useMutation({
    mutationFn: ({ comment }: { comment: string }) =>
      stagingApi.rejectOverride(
        id,
        { comment, signatureMethod: "JWT_STANDARD" },
        uuidv4(),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["staging-override", id] });
      notify.success("Proposal override ditolak.");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const handleApprove = (action: "approve" | "approve2") => {
    const token = getStepUpToken();
    if (token) {
      if (action === "approve") {
        approveMutation.mutate({ stepUpToken: token });
      } else {
        approve2Mutation.mutate({ stepUpToken: token });
      }
    } else {
      setPendingAction(action);
      setMfaOpen(true);
    }
  };

  const handleMFAVerified = (token: string) => {
    setMfaOpen(false);
    if (pendingAction === "approve") {
      approveMutation.mutate({ stepUpToken: token });
    } else if (pendingAction === "approve2") {
      approve2Mutation.mutate({ stepUpToken: token });
    }
    setPendingAction(null);
  };

  const isMakerSelf = userId === override?.makerId;
  const isReviewer = override?.reviewerId === userId;

  if (isLoading) {
    return (
      <div className="p-6">
        <div className="h-40 rounded-lg bg-muted animate-pulse" />
      </div>
    );
  }

  if (!override) {
    return (
      <div className="p-6">
        <p>Override tidak ditemukan.</p>
        <Button
          variant="link"
          onClick={() => router.push("/ecl/staging/override")}
        >
          Kembali ke antrian
        </Button>
      </div>
    );
  }

  const stageFromNum = parseInt(
    override.stageFrom.replace("STAGE_", ""),
    10,
  ) as 1 | 2 | 3;
  const stageToNum = parseInt(
    override.stageTo.replace("STAGE_", ""),
    10,
  ) as 1 | 2 | 3;

  const isPending =
    reviewMutation.isPending ||
    approveMutation.isPending ||
    approve2Mutation.isPending ||
    rejectMutation.isPending;

  return (
    <div className="p-6 space-y-4 max-w-2xl">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex gap-1">
          <li>
            <button
              className="hover:underline"
              onClick={() => router.push("/ecl/staging/override")}
            >
              Override Staging
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">{id.slice(0, 8)}…</li>
        </ol>
      </nav>

      <div className="flex items-center gap-3">
        <h1 className="text-xl font-semibold">Proposal Override Staging</h1>
        <Badge variant={statusVariant(override.status)}>
          {STATUS_LABELS[override.status] ?? override.status}
        </Badge>
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Detail Proposal</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div>
              <p className="text-muted-foreground text-xs">Instrumen</p>
              <p className="font-medium">
                {override.kodeInstrumen ?? override.instrumenId}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">Periode Berakhir</p>
              <p>{override.periodeAkhir}</p>
            </div>
          </div>

          <div className="flex items-center gap-3">
            <div>
              <p className="text-muted-foreground text-xs mb-1">Stage Asal</p>
              <StageBadge stage={stageFromNum} size="sm" />
            </div>
            <span aria-hidden className="text-muted-foreground">
              →
            </span>
            <div>
              <p className="text-muted-foreground text-xs mb-1">Stage Target</p>
              <StageBadge stage={stageToNum} size="sm" />
            </div>
          </div>

          <div>
            <p className="text-muted-foreground text-xs">Alasan Override</p>
            <p className="text-sm mt-0.5 whitespace-pre-wrap">{override.alasan}</p>
          </div>

          <div className="grid grid-cols-2 gap-3 text-sm">
            <div>
              <p className="text-muted-foreground text-xs">Dibuat</p>
              <p>{new Date(override.createdAt).toLocaleDateString("id-ID")}</p>
            </div>
            <div>
              <p className="text-muted-foreground text-xs">Diajukan Oleh</p>
              <p className="font-mono text-xs">{override.makerId.slice(0, 8)}</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Action bar based on status + role */}
      <div className="flex flex-wrap gap-2">
        {/* Review — RISK, not maker */}
        {override.status === "PENDING_REVIEW" &&
          can("ecl_staging.override.review") &&
          !isMakerSelf && (
            <Button
              size="sm"
              onClick={() => reviewMutation.mutate({})}
              disabled={isPending}
            >
              Setuju (Review)
            </Button>
          )}

        {/* Approve ALCO — requires MFA step-up */}
        {override.status === "PENDING_APPROVAL" &&
          can("ecl_staging.override.approve") &&
          !isMakerSelf &&
          !isReviewer && (
            <Button
              size="sm"
              onClick={() => handleApprove("approve")}
              disabled={isPending}
            >
              Setuju (ALCO)
            </Button>
          )}

        {/* Approve 2 KOMITE — only when Stage 3 target or special flow */}
        {override.status === "APPROVED_ALCO" &&
          can("ecl_staging.override.approve2") &&
          !isMakerSelf &&
          !isReviewer && (
            <Button
              size="sm"
              onClick={() => handleApprove("approve2")}
              disabled={isPending}
            >
              Setuju (KOMITE)
            </Button>
          )}

        {/* Reject */}
        {(override.status === "PENDING_REVIEW" ||
          override.status === "PENDING_APPROVAL" ||
          override.status === "APPROVED_ALCO") &&
          (can("ecl_staging.override.review") ||
            can("ecl_staging.override.approve")) && (
            <RejectDialog
              onReject={(comment) => rejectMutation.mutate({ comment })}
              isPending={isPending}
            />
          )}
      </div>

      {/* MFA Modal */}
      <MFAStepUpModal
        open={mfaOpen}
        onOpenChange={setMfaOpen}
        title={
          pendingAction === "approve2"
            ? "Konfirmasi MFA — Approval KOMITE"
            : "Konfirmasi MFA — Approval ALCO"
        }
        description="Masukkan kode OTP untuk menyelesaikan approval."
        onVerified={handleMFAVerified}
        onCancel={() => {
          setMfaOpen(false);
          setPendingAction(null);
        }}
      />
    </div>
  );
}

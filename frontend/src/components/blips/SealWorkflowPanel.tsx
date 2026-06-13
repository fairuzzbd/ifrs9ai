"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { Loader2 } from "lucide-react";

import {
  requestSealSchema,
  approveSealSchema,
  rejectSealSchema,
  type RequestSealForm,
  type ApproveSealForm,
  type RejectSealForm,
} from "@/lib/schemas/calc-run.schema";
import { calcRunApi } from "@/lib/api/calc-run.api";
import { notify } from "@/lib/notify";
import { MFAStepUpModal } from "@/components/blips/MFAStepUpModal";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { useCalcRunStore, type SealModalState } from "@/lib/stores/calc-run.store";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface SealWorkflowPanelProps {
  calcRunId: string;
  calcRunLabel?: string;
  onSealed?: () => void;
  onSealRequested?: () => void;
  onSealRejected?: () => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function SealWorkflowPanel({
  calcRunId,
  calcRunLabel,
  onSealed,
  onSealRequested,
  onSealRejected,
}: SealWorkflowPanelProps) {
  const queryClient = useQueryClient();
  const { sealModalState, setSealModalState, pendingApproveComment, setPendingApproveComment } =
    useCalcRunStore();

  const label = calcRunLabel ?? calcRunId;

  // -------------------------------------------------------------------------
  // Forms
  // -------------------------------------------------------------------------

  const requestForm = useForm<RequestSealForm>({
    resolver: zodResolver(requestSealSchema),
    defaultValues: { comment: "" },
  });

  const approveForm = useForm<ApproveSealForm>({
    resolver: zodResolver(approveSealSchema),
    defaultValues: { comment: "" },
  });

  const rejectForm = useForm<RejectSealForm>({
    resolver: zodResolver(rejectSealSchema),
    defaultValues: { rejectReason: "" },
  });

  // -------------------------------------------------------------------------
  // Mutations
  // -------------------------------------------------------------------------

  const requestMutation = useMutation({
    mutationFn: (data: RequestSealForm) =>
      calcRunApi.seal(calcRunId, { action: "REQUEST", comment: data.comment }, uuidv4()),
    onSuccess: () => {
      notify.success(`Request segel ${label} dikirim. Menunggu persetujuan ALCO.`);
      setSealModalState("closed");
      requestForm.reset();
      void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] });
      onSealRequested?.();
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const approveMutation = useMutation({
    mutationFn: ({ comment, stepUpToken }: { comment: string; stepUpToken: string }) =>
      calcRunApi.seal(
        calcRunId,
        { action: "APPROVE", comment },
        uuidv4(),
        stepUpToken,
      ),
    onSuccess: () => {
      notify.success(
        `Calc run ${label} berhasil di-segel. Hasil ECL final dan immutable.`,
      );
      setSealModalState("closed");
      approveForm.reset();
      setPendingApproveComment("");
      void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] });
      onSealed?.();
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  const rejectMutation = useMutation({
    mutationFn: (data: RejectSealForm) =>
      calcRunApi.seal(
        calcRunId,
        { action: "REJECT", rejectReason: data.rejectReason },
        uuidv4(),
      ),
    onSuccess: () => {
      notify.destructive(`Segel ${label} berhasil ditolak.`);
      setSealModalState("closed");
      rejectForm.reset();
      void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] });
      onSealRejected?.();
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  // -------------------------------------------------------------------------
  // Handlers
  // -------------------------------------------------------------------------

  const handleApprovePreMFASubmit = (data: ApproveSealForm) => {
    setPendingApproveComment(data.comment);
    // Anti-stack: close approve-confirm before opening MFA
    setSealModalState("approve-mfa");
  };

  const handleMFAVerified = (stepUpToken: string) => {
    approveMutation.mutate({ comment: pendingApproveComment, stepUpToken });
  };

  // -------------------------------------------------------------------------
  // Helpers
  // -------------------------------------------------------------------------

  const close = (modal: SealModalState) => {
    if (sealModalState === modal) setSealModalState("closed");
  };

  const commentWatch = requestForm.watch("comment");
  const approveCommentWatch = approveForm.watch("comment");

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <>
      {/* Modal 1: Request Seal */}
      <Dialog
        open={sealModalState === "request"}
        onOpenChange={(open) => { if (!open) close("request"); }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Request Seal — {label}</DialogTitle>
            <DialogDescription>
              Seal akan mengunci hasil ECL ini secara permanen. Setelah di-seal,
              tidak ada modifikasi yang diizinkan (DEC-018).
            </DialogDescription>
          </DialogHeader>
          <Form {...requestForm}>
            <form
              onSubmit={requestForm.handleSubmit((d) => requestMutation.mutate(d))}
              className="space-y-4"
            >
              <FormField
                control={requestForm.control}
                name="comment"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Catatan Request *</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={4}
                        placeholder="Tulis catatan permintaan seal di sini (minimal 20 karakter)..."
                        aria-describedby="request-comment-error"
                        {...field}
                      />
                    </FormControl>
                    <div className="flex justify-between text-xs text-muted-foreground">
                      <FormMessage id="request-comment-error" />
                      <span
                        className={
                          (commentWatch?.length ?? 0) < 20
                            ? "text-destructive"
                            : "text-muted-foreground"
                        }
                      >
                        {commentWatch?.length ?? 0} / 20 min
                      </span>
                    </div>
                  </FormItem>
                )}
              />
              <div className="flex justify-end gap-2 pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => close("request")}
                  disabled={requestMutation.isPending}
                >
                  Batal
                </Button>
                <Button type="submit" disabled={requestMutation.isPending}>
                  {requestMutation.isPending && (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                  )}
                  Kirim Request
                </Button>
              </div>
            </form>
          </Form>
        </DialogContent>
      </Dialog>

      {/* Modal 2a: Approve Confirm (pre-MFA) */}
      <Dialog
        open={sealModalState === "approve-confirm"}
        onOpenChange={(open) => { if (!open) close("approve-confirm"); }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Approve Seal — {label}</DialogTitle>
            <DialogDescription>
              Anda akan menyetujui final seal hasil ECL ini. Tindakan ini tidak
              dapat dibalik.
            </DialogDescription>
          </DialogHeader>
          <Form {...approveForm}>
            <form
              onSubmit={approveForm.handleSubmit(handleApprovePreMFASubmit)}
              className="space-y-4"
            >
              <FormField
                control={approveForm.control}
                name="comment"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Catatan Approval *</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={4}
                        placeholder="Tulis catatan persetujuan seal (minimal 20 karakter)..."
                        aria-describedby="approve-comment-error"
                        {...field}
                      />
                    </FormControl>
                    <div className="flex justify-between text-xs text-muted-foreground">
                      <FormMessage id="approve-comment-error" />
                      <span
                        className={
                          (approveCommentWatch?.length ?? 0) < 20
                            ? "text-destructive"
                            : "text-muted-foreground"
                        }
                      >
                        {approveCommentWatch?.length ?? 0} / 20 min
                      </span>
                    </div>
                  </FormItem>
                )}
              />
              <div className="flex justify-end gap-2 pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => close("approve-confirm")}
                >
                  Batal
                </Button>
                <Button type="submit">
                  Lanjutkan ke Verifikasi MFA
                </Button>
              </div>
            </form>
          </Form>
        </DialogContent>
      </Dialog>

      {/* Modal 2b: MFA Step-Up */}
      <MFAStepUpModal
        open={sealModalState === "approve-mfa"}
        onOpenChange={(open) => { if (!open) setSealModalState("closed"); }}
        title="Step-up MFA diperlukan untuk tindakan sensitif ini (DEC-027). Verifikasi identitas Anda untuk melanjutkan."
        description="Verifikasi Identitas — MFA Step-Up"
        onVerified={handleMFAVerified}
        onCancel={() => setSealModalState("closed")}
      />

      {/* Modal 3: Reject Seal */}
      <AlertDialog
        open={sealModalState === "reject"}
        onOpenChange={(open) => { if (!open) close("reject"); }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Tolak Seal Calc Run?</AlertDialogTitle>
            <AlertDialogDescription>
              Penolakan akan mengembalikan calc run ke status SELESAI. ROLE-RISK
              dapat mengajukan ulang setelah perbaikan.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <Form {...rejectForm}>
            <form
              onSubmit={rejectForm.handleSubmit((d) => rejectMutation.mutate(d))}
              className="space-y-4 py-2"
            >
              <FormField
                control={rejectForm.control}
                name="rejectReason"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Alasan Penolakan *</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={4}
                        placeholder="Tulis alasan penolakan (minimal 30 karakter)..."
                        aria-describedby="reject-reason-error"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage id="reject-reason-error" />
                  </FormItem>
                )}
              />
              <AlertDialogFooter>
                <AlertDialogCancel onClick={() => close("reject")}>
                  Batal
                </AlertDialogCancel>
                <AlertDialogAction
                  type="submit"
                  className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                  disabled={rejectMutation.isPending}
                  onClick={(e) => {
                    // Prevent default close — form submit handles close
                    e.preventDefault();
                    void rejectForm.handleSubmit((d) => rejectMutation.mutate(d))();
                  }}
                >
                  {rejectMutation.isPending && (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                  )}
                  Tolak Seal
                </AlertDialogAction>
              </AlertDialogFooter>
            </form>
          </Form>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

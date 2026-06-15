"use client";

import * as React from "react";
import { Check, Clock, Circle, Loader2, Shield } from "lucide-react";
import { format } from "date-fns";
import { cn } from "@/lib/utils";
import { SodBlockBanner } from "@/components/blips/SodBlockBanner";
import {
  MFAStepUpModal,
  getStepUpToken,
  setStepUpToken,
} from "@/components/blips/MFAStepUpModal";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import type { MappingWorkflowStatus, WorkflowActor } from "@/lib/schemas/jurnal.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type StepState = "done" | "current" | "pending";

interface WorkflowStep {
  number: number;
  label: string;
  state: StepState;
  actor?: WorkflowActor | null;
  requiredRole?: string;
}

export interface SixEyesWorkflowPanelProps {
  workflowPath: "4-eyes" | "6-eyes";
  currentStatus: MappingWorkflowStatus;
  maker?: WorkflowActor | null;
  reviewer?: WorkflowActor | null;
  approver?: WorkflowActor | null;
  approver2?: WorkflowActor | null;
  rejectReason?: string | null;
  currentUserId: string;
  makerId?: string | null;
  reviewerId?: string | null;
  approverId?: string | null;
  approver2Id?: string | null;
  currentUserPermissions: string[];
  isRegulated?: boolean;
  onReview?: (comment: string) => Promise<void>;
  onApprove?: (comment: string, mfaToken?: string) => Promise<void>;
  onApprove2?: (comment: string, mfaToken: string) => Promise<void>;
  onReject?: (reason: string) => Promise<void>;
  onSubmit?: (comment: string) => Promise<void>;
  onWithdraw?: () => Promise<void>;
}

// ---------------------------------------------------------------------------
// Step icon
// ---------------------------------------------------------------------------

function StepIcon({ state }: { state: StepState }) {
  if (state === "done") return <Check className="h-4 w-4" aria-hidden="true" />;
  if (state === "current") return <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />;
  return <Circle className="h-4 w-4" aria-hidden="true" />;
}

function fmtDate(s?: string | null) {
  if (!s) return "-";
  try { return format(new Date(s), "d MMM yyyy HH:mm"); } catch { return s; }
}

// ---------------------------------------------------------------------------
// Action panel (sub-component)
// ---------------------------------------------------------------------------

interface ActionPanelProps {
  actionType: "review" | "approve" | "approve2" | "submit" | "withdraw";
  requireMfa?: boolean;
  submitting: boolean;
  onAction: (comment: string, mfaToken?: string) => Promise<void>;
  onReject?: (reason: string) => Promise<void>;
  minRejectChars?: number;
}

function ActionPanel({
  actionType,
  requireMfa = false,
  submitting,
  onAction,
  onReject,
  minRejectChars = 30,
}: ActionPanelProps) {
  const [comment, setComment] = React.useState("");
  const [attested, setAttested] = React.useState(false);
  const [showReject, setShowReject] = React.useState(false);
  const [rejectReason, setRejectReason] = React.useState("");
  const [mfaOpen, setMfaOpen] = React.useState(false);

  const handleApprove = async () => {
    if (requireMfa) {
      const existing = getStepUpToken();
      if (existing) {
        await onAction(comment, existing);
      } else {
        setMfaOpen(true);
      }
    } else {
      await onAction(comment);
    }
  };

  const handleMfaVerified = async (token: string) => {
    setStepUpToken(token);
    await onAction(comment, token);
  };

  const handleReject = async () => {
    if (rejectReason.length < minRejectChars) return;
    await onReject?.(rejectReason);
  };

  const actionLabel = {
    review: "Review & Lanjutkan",
    approve: requireMfa ? "Approve (MFA)" : "Approve",
    approve2: "Approve Kedua (MFA)",
    submit: "Submit ke Review",
    withdraw: "Tarik",
  }[actionType];

  if (showReject) {
    return (
      <div className="space-y-3">
        <h4 className="text-sm font-semibold text-destructive">Tolak & Kembalikan ke Maker</h4>
        <div className="space-y-1">
          <Label htmlFor="reject-reason" className="text-xs">
            Alasan Penolakan <span className="text-destructive">*</span>
            <span className="ml-1 text-muted-foreground">(min {minRejectChars} karakter)</span>
          </Label>
          <Textarea
            id="reject-reason"
            rows={3}
            value={rejectReason}
            onChange={(e) => setRejectReason(e.target.value)}
            placeholder="Jelaskan alasan penolakan..."
            aria-required="true"
            aria-describedby="reject-counter"
          />
          <div
            id="reject-counter"
            className={cn(
              "text-right text-xs",
              rejectReason.length < minRejectChars ? "text-destructive" : "text-muted-foreground",
            )}
          >
            {rejectReason.length} / {minRejectChars} karakter minimum
          </div>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => setShowReject(false)} disabled={submitting}>
            Batal
          </Button>
          <Button
            variant="destructive"
            size="sm"
            disabled={submitting || rejectReason.length < minRejectChars}
            onClick={handleReject}
          >
            {submitting ? "Memproses..." : "Tolak & Kembalikan"}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {requireMfa && (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800"
        >
          <Shield className="h-3.5 w-3.5 mt-0.5 shrink-0" aria-hidden="true" />
          <span>
            Langkah ini memerlukan <strong>MFA step-up</strong> (DEC-027). Verifikasi MFA akan diminta saat Approve.
          </span>
        </div>
      )}

      {(actionType === "review" || actionType === "approve" || actionType === "approve2") && (
        <>
          <div className="space-y-1">
            <Label htmlFor={`comment-${actionType}`} className="text-xs">
              Komentar <span className="text-muted-foreground">(opsional)</span>
            </Label>
            <Textarea
              id={`comment-${actionType}`}
              rows={2}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder="Tambahkan komentar..."
            />
          </div>

          <div className="flex items-start gap-3 rounded-md border p-3">
            <Checkbox
              id={`attest-${actionType}`}
              checked={attested}
              onCheckedChange={(v) => setAttested(v === true)}
              aria-describedby={`attest-label-${actionType}`}
            />
            <Label
              id={`attest-label-${actionType}`}
              htmlFor={`attest-${actionType}`}
              className="cursor-pointer text-xs leading-relaxed"
            >
              Saya menyatakan bahwa data ini telah saya periksa dan sesuai dengan standar yang berlaku.
            </Label>
          </div>
        </>
      )}

      <div className="flex gap-2">
        {onReject && (actionType === "review" || actionType === "approve" || actionType === "approve2") && (
          <Button variant="outline" size="sm" disabled={submitting} onClick={() => setShowReject(true)}>
            Tolak
          </Button>
        )}
        <Button
          size="sm"
          disabled={
            submitting ||
            ((actionType === "review" || actionType === "approve" || actionType === "approve2") && !attested)
          }
          onClick={handleApprove}
        >
          {submitting
            ? "Memproses..."
            : actionType === "withdraw"
              ? "Tarik Mapping"
              : actionLabel}
        </Button>
      </div>

      <MFAStepUpModal
        open={mfaOpen}
        onOpenChange={setMfaOpen}
        title={`Verifikasi MFA untuk ${actionLabel}`}
        description="Step-up MFA diperlukan untuk langkah ini (DEC-027)"
        onVerified={handleMfaVerified}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main Panel
// ---------------------------------------------------------------------------

export function SixEyesWorkflowPanel({
  workflowPath,
  currentStatus,
  maker,
  reviewer,
  approver,
  approver2,
  rejectReason,
  currentUserId,
  makerId,
  reviewerId,
  approverId,
  approver2Id,
  currentUserPermissions,
  isRegulated = false,
  onReview,
  onApprove,
  onApprove2,
  onReject,
  onSubmit,
  onWithdraw,
}: SixEyesWorkflowPanelProps) {
  const [submitting, setSubmitting] = React.useState(false);

  const is6Eyes = workflowPath === "6-eyes";

  // Compute step states
  const makerDone = currentStatus !== "DRAFT";
  const reviewerDone = ["PENDING_APPROVAL", "PENDING_APPROVAL_2", "APPROVED_ACTIVE"].includes(currentStatus);
  const approverDone = ["PENDING_APPROVAL_2", "APPROVED_ACTIVE"].includes(currentStatus);
  const approver2Done = currentStatus === "APPROVED_ACTIVE";

  const steps: WorkflowStep[] = [
    {
      number: 1,
      label: "Maker",
      state: makerDone ? "done" : "current",
      actor: maker,
    },
    {
      number: 2,
      label: "Reviewer",
      state:
        currentStatus === "PENDING_REVIEW"
          ? "current"
          : reviewerDone
            ? "done"
            : "pending",
      actor: reviewer,
      requiredRole: "ROLE-AKUN-CTL",
    },
    {
      number: 3,
      label: is6Eyes ? "Approver 1" : "Approver",
      state:
        currentStatus === "PENDING_APPROVAL"
          ? "current"
          : approverDone
            ? "done"
            : "pending",
      actor: approver,
      requiredRole: "ROLE-AKUN-CTL",
    },
    ...(is6Eyes
      ? [
          {
            number: 4,
            label: "Approver 2 (ROLE-RISK)",
            state:
              currentStatus === "PENDING_APPROVAL_2"
                ? "current"
                : approver2Done
                  ? "done"
                  : "pending",
            actor: approver2,
            requiredRole: "ROLE-RISK",
          } as WorkflowStep,
        ]
      : []),
  ];

  // Determine current actor's available action
  const isMaker = currentUserId === makerId;
  const isReviewer = currentUserId === reviewerId;
  const isApprover = currentUserId === approverId;

  const canReview =
    currentStatus === "PENDING_REVIEW" &&
    !isMaker &&
    currentUserPermissions.includes("jurnal_mapping.review");

  const canApprove =
    currentStatus === "PENDING_APPROVAL" &&
    !isMaker &&
    !isReviewer &&
    currentUserPermissions.includes("jurnal_mapping.approve");

  const canApprove2 =
    currentStatus === "PENDING_APPROVAL_2" &&
    !isMaker &&
    !isReviewer &&
    !isApprover &&
    currentUserPermissions.includes("jurnal_mapping.approve_2");

  const canSubmit =
    currentStatus === "DRAFT" &&
    isMaker &&
    currentUserPermissions.includes("jurnal_mapping.create");

  const canWithdraw =
    currentStatus === "DRAFT" &&
    isMaker &&
    currentUserPermissions.includes("jurnal_mapping.create");

  const wrap = async (fn: () => Promise<void>) => {
    setSubmitting(true);
    try {
      await fn();
    } finally {
      setSubmitting(false);
    }
  };

  // SoD block conditions
  const reviewSodBlock = currentStatus === "PENDING_REVIEW" && isMaker;
  const approveSodBlock =
    currentStatus === "PENDING_APPROVAL" && (isMaker || isReviewer);
  const approve2SodBlock =
    currentStatus === "PENDING_APPROVAL_2" && (isMaker || isReviewer || isApprover);

  return (
    <div className="space-y-4">
      {/* Steps timeline */}
      <div>
        <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-3">
          Status Workflow
        </h3>
        <div className="space-y-0">
          {steps.map((step, i) => {
            const iconBg =
              step.state === "done"
                ? "bg-green-100 text-green-700"
                : step.state === "current"
                  ? "bg-amber-100 text-amber-700"
                  : "bg-gray-100 text-gray-400";

            return (
              <div key={step.number} className="flex gap-3">
                <div className="flex flex-col items-center">
                  <div
                    className={cn(
                      "flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold",
                      iconBg,
                    )}
                  >
                    <StepIcon state={step.state} />
                  </div>
                  {i < steps.length - 1 && (
                    <div className="mt-1 flex-1 w-px bg-gray-200" />
                  )}
                </div>
                <div className="pb-5 min-w-0 flex-1">
                  <p
                    className={cn(
                      "text-sm font-medium",
                      step.state === "current"
                        ? "text-amber-700"
                        : step.state === "done"
                          ? "text-gray-800"
                          : "text-gray-400",
                    )}
                  >
                    {step.number}. {step.label}{" "}
                    {step.state === "current" && (
                      <span className="text-xs font-normal">(SEKARANG)</span>
                    )}
                  </p>
                  {step.actor && step.state !== "pending" ? (
                    <div className="mt-0.5 space-y-0.5">
                      <p className="text-xs text-gray-500">
                        {step.actor.nama}{" "}
                        {step.actor.signedAt && `· ${fmtDate(step.actor.signedAt)}`}
                      </p>
                      {step.actor.comment && (
                        <p className="text-xs text-gray-600 bg-gray-50 rounded p-1.5">
                          &ldquo;{step.actor.comment}&rdquo;
                        </p>
                      )}
                    </div>
                  ) : step.state !== "done" ? (
                    <p className="text-xs text-gray-400 mt-0.5">
                      {step.state === "current" && step.requiredRole
                        ? `Diperlukan: ${step.requiredRole}`
                        : "Belum dimulai"}
                    </p>
                  ) : null}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Reject reason */}
      {rejectReason && (
        <div className="rounded-md border border-red-200 bg-red-50 p-3">
          <p className="text-xs font-medium text-red-700">Alasan Penolakan:</p>
          <p className="text-xs text-red-600 mt-1">{rejectReason}</p>
        </div>
      )}

      {/* Action area */}
      {(reviewSodBlock || approveSodBlock || approve2SodBlock) && (
        <SodBlockBanner
          message={
            reviewSodBlock
              ? "Anda tidak bisa mereview submission Anda sendiri (DEC-017)."
              : approve2SodBlock
                ? "Anda tidak bisa menjadi approver kedua setelah terlibat di step sebelumnya (DEC-017)."
                : "Anda tidak bisa menyetujui data yang Anda buat atau review sendiri (DEC-017)."
          }
        />
      )}

      {canSubmit && !canWithdraw && onSubmit && (
        <ActionPanel
          actionType="submit"
          submitting={submitting}
          onAction={async (comment) => wrap(() => onSubmit(comment))}
        />
      )}

      {canSubmit && canWithdraw && onSubmit && (
        <div className="space-y-2">
          <ActionPanel
            actionType="submit"
            submitting={submitting}
            onAction={async (comment) => wrap(() => onSubmit(comment))}
          />
          {onWithdraw && (
            <Button
              variant="ghost"
              size="sm"
              className="text-muted-foreground hover:text-destructive w-full"
              onClick={() => wrap(onWithdraw)}
              disabled={submitting}
            >
              Tarik Mapping
            </Button>
          )}
        </div>
      )}

      {canReview && onReview && (
        <ActionPanel
          actionType="review"
          submitting={submitting}
          onAction={async (comment) => wrap(() => onReview(comment))}
          onReject={onReject ? async (r) => wrap(() => onReject(r)) : undefined}
        />
      )}

      {canApprove && onApprove && (
        <ActionPanel
          actionType="approve"
          requireMfa={isRegulated}
          submitting={submitting}
          onAction={async (comment, mfa) => wrap(() => onApprove(comment, mfa))}
          onReject={onReject ? async (r) => wrap(() => onReject(r)) : undefined}
        />
      )}

      {canApprove2 && onApprove2 && (
        <ActionPanel
          actionType="approve2"
          requireMfa={true}
          submitting={submitting}
          onAction={async (comment, mfa) => mfa ? wrap(() => onApprove2(comment, mfa)) : Promise.resolve()}
          onReject={onReject ? async (r) => wrap(() => onReject(r)) : undefined}
        />
      )}

      {currentStatus === "APPROVED_ACTIVE" && (
        <div className="flex items-center gap-2 rounded-md border border-green-200 bg-green-50 p-3 text-xs text-green-700">
          <Check className="h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="font-medium">Mapping aktif — Siap digunakan oleh resolver</span>
        </div>
      )}
    </div>
  );
}

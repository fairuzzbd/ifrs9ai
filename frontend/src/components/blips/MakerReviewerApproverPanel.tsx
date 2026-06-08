"use client";

import * as React from "react";
import { Check, Circle, CornerDownLeft } from "lucide-react";
import { format, parseISO } from "date-fns";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { ApprovalWithSignature } from "@/components/blips/ApprovalWithSignature";
import { cn } from "@/lib/utils";
import type { WorkflowStatus, WorkflowHistoryEntry, MasterWorkflowState } from "@/lib/schemas/mata-uang.schema";

export type { WorkflowStatus, WorkflowHistoryEntry, MasterWorkflowState };

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function isSoDViolation(
  currentUserId: string | null,
  workflow: WorkflowStatus,
  action: "review" | "approve" | "approve2",
): boolean {
  if (!currentUserId) return false;
  if (action === "review") {
    return currentUserId === workflow.makerId;
  }
  if (action === "approve") {
    return (
      currentUserId === workflow.makerId ||
      currentUserId === workflow.reviewerId
    );
  }
  if (action === "approve2") {
    return (
      currentUserId === workflow.makerId ||
      currentUserId === workflow.reviewerId ||
      currentUserId === workflow.approverId
    );
  }
  return false;
}

function formatDateTime(iso: string): string {
  try {
    return format(parseISO(iso), "dd MMM yyyy, HH:mm 'WIB'");
  } catch {
    return iso;
  }
}

// ---------------------------------------------------------------------------
// Step icon
// ---------------------------------------------------------------------------

type StepState = "done" | "active" | "returned" | "pending";

function StepIcon({ state }: { state: StepState }) {
  if (state === "done") {
    return (
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-green-100 text-green-700">
        <Check className="h-4 w-4" aria-hidden />
      </span>
    );
  }
  if (state === "returned") {
    return (
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-orange-100 text-orange-700">
        <CornerDownLeft className="h-4 w-4" aria-hidden />
      </span>
    );
  }
  if (state === "active") {
    return (
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border-2 border-primary bg-primary/10">
        <Circle className="h-3.5 w-3.5 fill-primary text-primary" aria-hidden />
      </span>
    );
  }
  return (
    <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border-2 border-muted bg-muted/30">
      <Circle className="h-3.5 w-3.5 text-muted-foreground" aria-hidden />
    </span>
  );
}

// ---------------------------------------------------------------------------
// Completed step summary
// ---------------------------------------------------------------------------

function CompletedStep({ entry }: { entry: WorkflowHistoryEntry }) {
  const [open, setOpen] = React.useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className="text-sm text-muted-foreground">
        <span className="font-medium text-foreground">{entry.username}</span>{" "}
        &mdash; {entry.role}
      </div>
      <div className="text-xs text-muted-foreground">
        {formatDateTime(entry.signedAt)}
      </div>
      {entry.comment && (
        <div className="mt-1 text-sm italic text-muted-foreground">
          &ldquo;{entry.comment.length > 100 ? `${entry.comment.slice(0, 100)}...` : entry.comment}&rdquo;
        </div>
      )}
      <CollapsibleTrigger asChild>
        <Button variant="link" size="sm" className="h-auto px-0 text-xs">
          {open ? "Sembunyikan detail" : "Lihat detail"}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-2 space-y-1 rounded-md border bg-muted/30 p-3 text-xs">
          <div>
            <span className="font-medium">Tanda tangan:</span>{" "}
            <code className="font-mono text-muted-foreground">
              {entry.signatureHash}
            </code>
          </div>
          {entry.comment && entry.comment.length > 100 && (
            <div>
              <span className="font-medium">Komentar lengkap:</span>
              <p className="mt-0.5 italic">{entry.comment}</p>
            </div>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export interface MakerReviewerApproverPanelProps {
  workflowData: WorkflowStatus;
  currentUserId: string | null;
  entityStatus: MasterWorkflowState;
  submitting?: boolean;
  onSubmit?: (comment?: string) => void;
  onReview?: (comment: string | undefined) => void;
  onApprove?: (comment: string | undefined) => void;
  /** 6-eyes: called when second approver acts. Requires signatureMethod JWT_STEP_UP. */
  onApprove2?: (comment: string | undefined) => void;
  onReject?: (comment: string) => void;
  className?: string;
}

export function MakerReviewerApproverPanel({
  workflowData,
  currentUserId,
  entityStatus,
  submitting = false,
  onReview,
  onApprove,
  onApprove2,
  onReject,
  className,
}: MakerReviewerApproverPanelProps) {
  const history = workflowData.history ?? [];
  const is6Eyes = !!onApprove2;

  // Find history entries by action type
  const submitEntry = history.find((h) => h.action === "SUBMIT");
  const reviewEntry = history.find((h) => h.action === "REVIEW");
  const approveEntry = history.find((h) => h.action === "APPROVE");
  const approve2Entry = history.find((h) => h.action === "APPROVE_2");
  const rejectEntry = [...history].reverse().find((h) => h.action === "REJECT");

  // Determine step states
  const isReturned = entityStatus === "RETURNED";
  const isPendingReview = entityStatus === "PENDING_REVIEW";
  const isPendingApproval = entityStatus === "PENDING_APPROVAL";
  const isPendingApproval2 = entityStatus === "PENDING_APPROVAL_2";
  const isApproved = entityStatus === "APPROVED";

  const step1State: StepState = submitEntry ? "done" : "pending";

  const step2State: StepState = reviewEntry
    ? "done"
    : isReturned && !reviewEntry
      ? "returned"
      : isPendingReview
        ? "active"
        : "pending";

  const step3State: StepState = approveEntry
    ? "done"
    : isPendingApproval
      ? "active"
      : "pending";

  // 6-eyes only
  const step4State: StepState = approve2Entry
    ? "done"
    : isPendingApproval2
      ? "active"
      : "pending";

  const sodReview = isSoDViolation(currentUserId, workflowData, "review");
  const sodApprove = isSoDViolation(currentUserId, workflowData, "approve");
  const sodApprove2 = isSoDViolation(currentUserId, workflowData, "approve2");

  // Determine the final approval entry for the approved banner
  const finalApproveEntry = is6Eyes ? (approve2Entry ?? approveEntry) : approveEntry;

  return (
    <div className={cn("space-y-1", className)}>
      <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
        Proses Persetujuan ({is6Eyes ? "6-eyes" : "4-eyes"})
      </h3>
      <Separator className="mb-4" />

      {/* Step 1: Maker */}
      <div className="flex gap-4 pb-4">
        <div className="flex flex-col items-center">
          <StepIcon state={step1State} />
          <div className="mt-1 w-0.5 flex-1 bg-border" aria-hidden />
        </div>
        <div className="flex-1 pb-4">
          <p className="text-sm font-medium">Step 1: Pembuat (Maker)</p>
          {submitEntry ? (
            <CompletedStep entry={submitEntry} />
          ) : (
            <p className="text-sm text-muted-foreground">Belum di-submit</p>
          )}
        </div>
      </div>

      {/* Step 2: Reviewer */}
      <div className="flex gap-4 pb-4">
        <div className="flex flex-col items-center">
          <StepIcon state={step2State} />
          <div className="mt-1 w-0.5 flex-1 bg-border" aria-hidden />
        </div>
        <div className="flex-1 pb-4">
          <p className={cn("text-sm font-medium", step2State === "active" && "text-primary")}>
            Step 2: Pemeriksa (Reviewer)
            {isReturned && (
              <span className="ml-2 text-xs text-orange-600">&mdash; Dikembalikan</span>
            )}
          </p>

          {reviewEntry ? (
            <CompletedStep entry={reviewEntry} />
          ) : isReturned && rejectEntry ? (
            <div className="space-y-1">
              <div className="text-sm text-muted-foreground">
                <span className="font-medium text-foreground">{rejectEntry.username}</span>{" "}
                &mdash; {rejectEntry.role}
              </div>
              <div className="text-xs text-muted-foreground">
                {formatDateTime(rejectEntry.signedAt)}
              </div>
              {rejectEntry.comment && (
                <div className="mt-2 space-y-1">
                  <p className="text-xs font-medium text-orange-700">Alasan Penolakan:</p>
                  <blockquote className="border-l-2 border-orange-400 pl-3 text-sm italic text-orange-800">
                    &ldquo;{rejectEntry.comment}&rdquo;
                  </blockquote>
                </div>
              )}
            </div>
          ) : isPendingReview ? (
            <div className="mt-2">
              <p className="mb-3 text-sm text-muted-foreground">
                Menunggu review dari Risk Officer
              </p>
              <ApprovalWithSignature
                actionType="review"
                sodBlocked={sodReview}
                submitting={submitting}
                onApprove={(comment) => onReview?.(comment)}
                onReject={(comment) => onReject?.(comment)}
              />
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">Menunggu step 1 selesai</p>
          )}
        </div>
      </div>

      {/* Step 3: Approver 1 */}
      <div className={cn("flex gap-4 pb-4", !is6Eyes && "")}>
        <div className="flex flex-col items-center">
          <StepIcon state={step3State} />
          {is6Eyes && <div className="mt-1 w-0.5 flex-1 bg-border" aria-hidden />}
        </div>
        <div className="flex-1 pb-4">
          <p className={cn("text-sm font-medium", step3State === "active" && "text-primary")}>
            Step 3: Pemberi Persetujuan 1{is6Eyes ? " (ALCO)" : " (Approver)"}
          </p>

          {approveEntry ? (
            <CompletedStep entry={approveEntry} />
          ) : isPendingApproval ? (
            <div className="mt-2">
              <p className="mb-3 text-sm text-muted-foreground">
                Menunggu persetujuan{is6Eyes ? " ALCO (step-up MFA wajib)" : " akhir"}
              </p>
              <ApprovalWithSignature
                actionType="approve"
                sodBlocked={sodApprove}
                submitting={submitting}
                requireStepUpMfa={is6Eyes}
                onApprove={(comment) => onApprove?.(comment)}
                onReject={(comment) => onReject?.(comment)}
              />
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">Menunggu step 2 selesai</p>
          )}
        </div>
      </div>

      {/* Step 4: Approver 2 — only for 6-eyes */}
      {is6Eyes && (
        <div className="flex gap-4">
          <div className="flex flex-col items-center">
            <StepIcon state={step4State} />
          </div>
          <div className="flex-1">
            <p className={cn("text-sm font-medium", step4State === "active" && "text-primary")}>
              Step 4: Pemberi Persetujuan 2 (CFO / Komite)
            </p>

            {approve2Entry ? (
              <CompletedStep entry={approve2Entry} />
            ) : isPendingApproval2 ? (
              <div className="mt-2">
                <p className="mb-3 text-sm text-muted-foreground">
                  Menunggu persetujuan akhir CFO/Komite (step-up MFA wajib)
                </p>
                <ApprovalWithSignature
                  actionType="approve"
                  sodBlocked={sodApprove2}
                  submitting={submitting}
                  requireStepUpMfa
                  onApprove={(comment) => onApprove2?.(comment)}
                  onReject={(comment) => onReject?.(comment)}
                />
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">Menunggu step 3 selesai</p>
            )}
          </div>
        </div>
      )}

      {/* Approved summary */}
      {isApproved && finalApproveEntry && (
        <>
          <Separator className="my-4" />
          <div className="flex items-center gap-2 rounded-md bg-green-50 px-3 py-2">
            <Check className="h-4 w-4 text-green-700" aria-hidden />
            <p className="text-sm text-green-800">
              Disetujui pada {formatDateTime(finalApproveEntry.signedAt)}
            </p>
          </div>
        </>
      )}
    </div>
  );
}

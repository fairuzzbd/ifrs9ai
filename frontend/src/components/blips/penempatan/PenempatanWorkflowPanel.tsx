"use client";

import * as React from "react";
import { Check, Clock, Circle, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { format } from "date-fns";
import type { PenempatanDeposito, PenempatanWorkflowStatus } from "@/lib/schemas/penempatan.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type StepState = "done" | "current" | "pending";

interface WorkflowStep {
  label: string;
  state: StepState;
  actor?: string | null;
  signedAt?: string | null;
  comment?: string | null;
  signatureHash?: string | null;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const TERMINATION_STATUSES: PenempatanWorkflowStatus[] = [
  "TERMINATION_PENDING_REVIEW",
  "TERMINATION_PENDING_APPROVAL",
  "TERMINATED",
];

function fmtDate(s: string | null | undefined): string {
  if (!s) return "-";
  try {
    return format(new Date(s), "d MMM yyyy HH:mm");
  } catch {
    return s;
  }
}

function truncateHash(hash: string | null | undefined): string {
  if (!hash) return "";
  return `${hash.slice(0, 8)}...`;
}

// ---------------------------------------------------------------------------
// Step component
// ---------------------------------------------------------------------------

function WorkflowStepItem({ step }: { step: WorkflowStep }) {
  const [expanded, setExpanded] = React.useState(false);

  const iconBg =
    step.state === "done"
      ? "bg-green-100 text-green-700"
      : step.state === "current"
        ? "bg-amber-100 text-amber-700"
        : "bg-gray-100 text-gray-400";

  return (
    <div className="flex gap-3">
      {/* Icon column */}
      <div className="flex flex-col items-center">
        <div className={cn("flex h-8 w-8 shrink-0 items-center justify-center rounded-full", iconBg)}>
          {step.state === "done" ? (
            <Check className="h-4 w-4" aria-hidden="true" />
          ) : step.state === "current" ? (
            <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
          ) : (
            <Circle className="h-4 w-4" aria-hidden="true" />
          )}
        </div>
        <div className="mt-1 flex-1 w-px bg-gray-200" />
      </div>

      {/* Content column */}
      <div className="pb-6 min-w-0 flex-1">
        <p
          className={cn(
            "text-sm font-medium",
            step.state === "current" ? "text-amber-700" : step.state === "done" ? "text-gray-800" : "text-gray-400",
          )}
        >
          {step.label}
        </p>

        {step.state === "done" && (
          <div className="mt-1 space-y-0.5">
            {step.actor && (
              <p className="text-xs text-gray-500">{step.actor} · {fmtDate(step.signedAt)}</p>
            )}
            {step.comment && (
              <button
                type="button"
                className="text-xs text-blue-600 hover:underline"
                onClick={() => setExpanded((v) => !v)}
                aria-expanded={expanded}
              >
                {expanded ? "Sembunyikan komentar" : "Lihat komentar"}
              </button>
            )}
            {expanded && step.comment && (
              <p className="text-xs text-gray-600 mt-1 bg-gray-50 rounded p-2">{step.comment}</p>
            )}
            {step.signatureHash && (
              <div className="flex items-center gap-1 mt-1">
                <p className="text-xs text-gray-400 font-mono">
                  Sig: {truncateHash(step.signatureHash)}
                </p>
                <button
                  type="button"
                  className="text-xs text-blue-500 hover:underline"
                  onClick={() => void navigator.clipboard.writeText(step.signatureHash!)}
                  aria-label="Salin signature hash"
                >
                  Salin
                </button>
              </div>
            )}
          </div>
        )}

        {step.state === "current" && (
          <p className="text-xs text-amber-600 mt-1">Menunggu tindakan...</p>
        )}

        {step.state === "pending" && (
          <p className="text-xs text-gray-400 mt-1">Belum dimulai</p>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

interface PenempatanWorkflowPanelProps {
  penempatan: PenempatanDeposito;
}

export function PenempatanWorkflowPanel({ penempatan }: PenempatanWorkflowPanelProps) {
  const ws = penempatan.workflowStatus;

  // ── Create workflow steps ──────────────────────────────────────────────────

  const createStepMaker: WorkflowStep = {
    label: "Maker",
    state: "done",
    actor: penempatan.makerNama ?? penempatan.makerId ?? "-",
    signedAt: penempatan.createdAt,
    comment: null,
    signatureHash: null,
  };

  const reviewerDone =
    ws === "PENDING_APPROVAL" ||
    ws === "APPROVED_ACTIVE" ||
    TERMINATION_STATUSES.includes(ws) ||
    ws === "MATURED" ||
    ws === "TERMINATED";

  const createStepReviewer: WorkflowStep = {
    label: "Reviewer",
    state: ws === "PENDING_REVIEW" ? "current" : reviewerDone ? "done" : "pending",
    actor: penempatan.reviewerNama ?? penempatan.reviewerId ?? undefined,
    signedAt: penempatan.reviewerSignedAt,
    comment: undefined,
    signatureHash: penempatan.reviewerSignatureHash,
  };

  const approverDone =
    ws === "APPROVED_ACTIVE" ||
    TERMINATION_STATUSES.includes(ws) ||
    ws === "MATURED" ||
    ws === "TERMINATED";

  const createStepApprover: WorkflowStep = {
    label: "Approver",
    state: ws === "PENDING_APPROVAL" ? "current" : approverDone ? "done" : "pending",
    actor: penempatan.approverNama ?? penempatan.approverId ?? undefined,
    signedAt: penempatan.approverSignedAt,
    comment: undefined,
    signatureHash: penempatan.approverSignatureHash,
  };

  const createSteps: WorkflowStep[] = [createStepMaker, createStepReviewer, createStepApprover];

  // ── Terminate workflow steps ───────────────────────────────────────────────

  const showTerminateWorkflow =
    TERMINATION_STATUSES.includes(ws) ||
    ws === "TERMINATED" ||
    !!penempatan.terminateReason;

  const terminateMakerDone = !!penempatan.terminateReason;
  const terminateReviewerDone =
    ws === "TERMINATION_PENDING_APPROVAL" || ws === "TERMINATED";
  const terminateApproverDone = ws === "TERMINATED";

  const terminateStepMaker: WorkflowStep = {
    label: "Maker Terminasi",
    state: terminateMakerDone ? "done" : "pending",
    actor: penempatan.makerNama ?? penempatan.makerId ?? undefined,
    comment: penempatan.terminateReason,
  };

  const terminateStepReviewer: WorkflowStep = {
    label: "Reviewer Terminasi",
    state:
      ws === "TERMINATION_PENDING_REVIEW"
        ? "current"
        : terminateReviewerDone
          ? "done"
          : "pending",
    actor: penempatan.terminateReviewerId ?? undefined,
  };

  const terminateStepApprover: WorkflowStep = {
    label: "Approver Terminasi",
    state:
      ws === "TERMINATION_PENDING_APPROVAL"
        ? "current"
        : terminateApproverDone
          ? "done"
          : "pending",
    actor: penempatan.terminateApproverId ?? undefined,
    signedAt: penempatan.terminatedAt,
  };

  const terminateSteps: WorkflowStep[] = [
    terminateStepMaker,
    terminateStepReviewer,
    terminateStepApprover,
  ];

  return (
    <div className="space-y-6">
      {/* Create workflow */}
      <div>
        <h3 className="mb-4 text-sm font-semibold text-gray-700 uppercase tracking-wide">
          Workflow Penempatan
        </h3>
        <div className="space-y-0">
          {createSteps.map((step) => (
            <WorkflowStepItem key={step.label} step={step} />
          ))}
        </div>
      </div>

      {/* Terminate workflow — conditional */}
      {showTerminateWorkflow && (
        <div>
          <div className="border-t border-gray-200 mb-4" />
          <h3 className="mb-4 text-sm font-semibold text-purple-700 uppercase tracking-wide">
            Workflow Terminasi
          </h3>
          <div className="space-y-0">
            {terminateSteps.map((step) => (
              <WorkflowStepItem key={step.label} step={step} />
            ))}
          </div>
        </div>
      )}

      {/* Reject reason banner */}
      {penempatan.rejectReason && (
        <div className="rounded-md border border-red-200 bg-red-50 p-3">
          <p className="text-xs font-medium text-red-700">Alasan Penolakan:</p>
          <p className="text-xs text-red-600 mt-1">{penempatan.rejectReason}</p>
        </div>
      )}
    </div>
  );
}

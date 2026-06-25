"use client";

/**
 * ClosingWorkflowActionBar
 *
 * Master action panel for the Periode Buku Close Workflow (P5-M4).
 * Buttons appear only when persona + state authorize the action (never disabled-visible).
 * MFA flow: destructive dialog → MFA dialog → confirm dialog (no modal stacking).
 *
 * State machine (simplified):
 *   OPEN → [soft-close-request, AKUN-CTL] → OPEN (pending)
 *   OPEN (pending) → [soft-close-approve, different AKUN-CTL, SoD] → SOFT_CLOSED
 *   SOFT_CLOSED → [hard-close-request, AKUN-CTL] → HARD_CLOSE_PENDING
 *   HARD_CLOSE_PENDING → [hard-close-approve, CFO + MFA] → CLOSED
 *   HARD_CLOSE_PENDING → [hard-close-reject, CFO] → SOFT_CLOSED
 *   CLOSED → [reopen-request, CFO] → CLOSED (pending reopen)
 *   CLOSED (pending) → [reopen-approve, CFO + MFA] → SOFT_CLOSED
 *   SOFT_CLOSED (pending reopen) → [reopen-approve, AKUN-CTL] → OPEN
 */

import * as React from "react";
import { ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { usePermissions } from "@/lib/stores/auth.store";
import type { StatusPeriode } from "@/lib/schemas/periode-close.schema";

// Dialog imports
import { SoftCloseRequestDialog } from "./SoftCloseRequestDialog";
import { SoftCloseApproveDialog } from "./SoftCloseApproveDialog";
import { HardCloseRequestDialog } from "./HardCloseRequestDialog";
import { HardCloseApproveConfirmDialog } from "./HardCloseApproveConfirmDialog";
import { HardCloseRejectDialog } from "./HardCloseRejectDialog";
import { ReopenRequestDialog } from "./ReopenRequestDialog";
import { ReopenApproveConfirmDialog } from "./ReopenApproveConfirmDialog";
import { MFAStepUpDialog } from "./MFAStepUpDialog";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ClosingWorkflowActionBarProps {
  periodeId: string;
  periodeKode: string;
  statusPeriode: StatusPeriode;
  /** For SoD check: ID of user who submitted soft-close request */
  softCloseRequestedBy?: string | null;
  /** For SoD check: ID of user who submitted hard-close / reopen request */
  pendingActionRequestedBy?: string | null;
  /** Row version for optimistic concurrency */
  rowVersion: number;
  /** Whether all 4 checklist items pass (gates soft-close and hard-close request buttons) */
  allChecklistPassed: boolean;
  /** Whether there is a pending soft-close request (OPEN state only) */
  hasPendingSoftCloseRequest?: boolean;
  /** Whether there is a pending reopen request (CLOSED or SOFT_CLOSED) */
  hasPendingReopenRequest?: boolean;
  /** Target status for the pending reopen request */
  pendingReopenTargetStatus?: StatusPeriode;
  /** Called on any successful state transition — parent should invalidate queries */
  onTransitionSuccess: () => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Dialog state enum (no modal stacking)
// ---------------------------------------------------------------------------

type ActiveDialog =
  | "none"
  | "soft-close-request"
  | "soft-close-approve"
  | "hard-close-request"
  | "hard-close-mfa"       // MFA before approve confirm
  | "hard-close-approve"   // Confirm after MFA
  | "hard-close-reject"
  | "reopen-request"
  | "reopen-mfa"           // MFA for CLOSED→SOFT_CLOSED only
  | "reopen-approve";

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ClosingWorkflowActionBar({
  periodeId,
  periodeKode,
  statusPeriode,
  softCloseRequestedBy,
  pendingActionRequestedBy: _pendingActionRequestedBy,
  rowVersion,
  allChecklistPassed,
  hasPendingSoftCloseRequest,
  hasPendingReopenRequest,
  pendingReopenTargetStatus,
  onTransitionSuccess,
  className,
}: ClosingWorkflowActionBarProps) {
  const { userId, hasRole, can } = usePermissions();
  const [activeDialog, setActiveDialog] = React.useState<ActiveDialog>("none");
  const [stepUpToken, setStepUpToken] = React.useState<string>("");

  // Helpers
  const close = () => setActiveDialog("none");

  // -------------------------------------------------------------------------
  // Persona flags (persona absent from DOM when not authorized)
  // -------------------------------------------------------------------------

  const isAkunCtl = hasRole("ROLE-AKUN-CTL");
  const isCFO = hasRole("ROLE-CFO");
  const canPeriodeSoftClose = can("periode.softclose");
  const canPeriodeHardClose = can("periode.hardclose");
  const canPeriodeReopen = can("periode.reopen");

  // SoD: AKUN-CTL who requested soft-close cannot approve it
  const isSoftCloseRequester = !!softCloseRequestedBy && userId === softCloseRequestedBy;

  // -------------------------------------------------------------------------
  // Visible action buttons per status + persona
  // -------------------------------------------------------------------------

  const showSoftCloseRequest =
    statusPeriode === "OPEN" &&
    !hasPendingSoftCloseRequest &&
    isAkunCtl &&
    canPeriodeSoftClose;

  const showSoftCloseApprove =
    statusPeriode === "OPEN" &&
    hasPendingSoftCloseRequest &&
    isAkunCtl &&
    !isSoftCloseRequester && // SoD
    canPeriodeSoftClose;

  const showHardCloseRequest =
    statusPeriode === "SOFT_CLOSED" &&
    isAkunCtl &&
    canPeriodeHardClose;

  const showHardCloseApprove =
    statusPeriode === "HARD_CLOSE_PENDING" &&
    isCFO &&
    canPeriodeHardClose;

  const showHardCloseReject =
    statusPeriode === "HARD_CLOSE_PENDING" &&
    isCFO &&
    canPeriodeHardClose;

  const showReopenRequest =
    (statusPeriode === "CLOSED" || statusPeriode === "SOFT_CLOSED") &&
    isCFO &&
    canPeriodeReopen &&
    !hasPendingReopenRequest;

  const showReopenApprove =
    hasPendingReopenRequest &&
    ((statusPeriode === "CLOSED" && isCFO) || (statusPeriode === "SOFT_CLOSED" && isAkunCtl)) &&
    (canPeriodeReopen || canPeriodeSoftClose);

  // Nothing to render for this user
  const hasAnyAction =
    showSoftCloseRequest ||
    showSoftCloseApprove ||
    showHardCloseRequest ||
    showHardCloseApprove ||
    showHardCloseReject ||
    showReopenRequest ||
    showReopenApprove;

  if (!hasAnyAction) return null;

  // -------------------------------------------------------------------------
  // Checklist gate tooltip
  // -------------------------------------------------------------------------

  const checklistGateProps = !allChecklistPassed
    ? { disabled: true, title: "Closing checklist belum semua lulus" }
    : {};

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <>
      {/* Action bar */}
      <div
        className={cn(
          "flex flex-wrap items-center gap-3 p-4 rounded-lg border bg-card",
          className,
        )}
        aria-label="Workflow actions for periode buku close"
      >
        <ChevronRight
          className="h-4 w-4 text-muted-foreground shrink-0 hidden sm:block"
          aria-hidden="true"
        />

        {/* OPEN → SOFT_CLOSED (request) */}
        {showSoftCloseRequest && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setActiveDialog("soft-close-request")}
            {...checklistGateProps}
            aria-label={`Ajukan soft-close untuk periode ${periodeKode}`}
          >
            Ajukan Soft-Close
          </Button>
        )}

        {/* OPEN (pending) → SOFT_CLOSED (approve, SoD) */}
        {showSoftCloseApprove && (
          <Button
            size="sm"
            onClick={() => setActiveDialog("soft-close-approve")}
            aria-label={`Approve soft-close request periode ${periodeKode}`}
          >
            Approve Soft-Close
          </Button>
        )}

        {/* SOFT_CLOSED → HARD_CLOSE_PENDING */}
        {showHardCloseRequest && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setActiveDialog("hard-close-request")}
            {...checklistGateProps}
            aria-label={`Ajukan hard-close untuk periode ${periodeKode}`}
          >
            Ajukan Hard-Close
          </Button>
        )}

        {/* HARD_CLOSE_PENDING → CLOSED (approve, CFO + MFA) */}
        {showHardCloseApprove && (
          <Button
            size="sm"
            variant="destructive"
            onClick={() => setActiveDialog("hard-close-mfa")}
            aria-label={`Approve hard-close periode ${periodeKode} — memerlukan step-up MFA`}
          >
            Approve Hard-Close (MFA)
          </Button>
        )}

        {/* HARD_CLOSE_PENDING → SOFT_CLOSED (reject, CFO) */}
        {showHardCloseReject && (
          <Button
            size="sm"
            variant="outline"
            className="text-destructive border-destructive/50 hover:bg-destructive/10"
            onClick={() => setActiveDialog("hard-close-reject")}
            aria-label={`Tolak hard-close request periode ${periodeKode}`}
          >
            Tolak Hard-Close
          </Button>
        )}

        {/* CLOSED / SOFT_CLOSED → reopen request */}
        {showReopenRequest && (
          <Button
            size="sm"
            variant="outline"
            className="text-orange-700 border-orange-300 hover:bg-orange-50"
            onClick={() => setActiveDialog("reopen-request")}
            aria-label={`Ajukan reopen periode ${periodeKode}`}
          >
            Reopen Periode
          </Button>
        )}

        {/* Reopen approve */}
        {showReopenApprove && (
          <Button
            size="sm"
            variant={pendingReopenTargetStatus === "SOFT_CLOSED" ? "destructive" : "default"}
            onClick={() => {
              if (pendingReopenTargetStatus === "SOFT_CLOSED") {
                // CLOSED→SOFT_CLOSED path: needs MFA first
                setActiveDialog("reopen-mfa");
              } else {
                // SOFT_CLOSED→OPEN path: no MFA
                setActiveDialog("reopen-approve");
              }
            }}
            aria-label={`Approve reopen periode ${periodeKode}`}
          >
            Approve Reopen
            {pendingReopenTargetStatus === "SOFT_CLOSED" ? " (MFA)" : ""}
          </Button>
        )}
      </div>

      {/* ------------------------------------------------------------------ */}
      {/* Dialogs (open one at a time — no stacking)                         */}
      {/* ------------------------------------------------------------------ */}

      {/* S1: Soft-close request */}
      <SoftCloseRequestDialog
        open={activeDialog === "soft-close-request"}
        onOpenChange={(o) => !o && close()}
        periodeId={periodeId}
        periodeKode={periodeKode}
        rowVersion={rowVersion}
        onSuccess={() => { close(); onTransitionSuccess(); }}
      />

      {/* S2: Soft-close approve */}
      <SoftCloseApproveDialog
        open={activeDialog === "soft-close-approve"}
        onOpenChange={(o) => !o && close()}
        periodeId={periodeId}
        periodeKode={periodeKode}
        onSuccess={() => { close(); onTransitionSuccess(); }}
      />

      {/* S3 step 1: Hard-close request (AKUN-CTL) */}
      <HardCloseRequestDialog
        open={activeDialog === "hard-close-request"}
        onOpenChange={(o) => !o && close()}
        periodeId={periodeId}
        periodeKode={periodeKode}
        rowVersion={rowVersion}
        onSuccess={() => { close(); onTransitionSuccess(); }}
      />

      {/* S3 step 2a: MFA for hard-close (opens before approve confirm) */}
      <MFAStepUpDialog
        open={activeDialog === "hard-close-mfa"}
        onOpenChange={(o) => !o && close()}
        scope="hard_close"
        actionDescription={`Approve hard-close permanent untuk periode ${periodeKode}`}
        periodeKode={periodeKode}
        onTokenReceived={(token) => {
          setStepUpToken(token);
          setActiveDialog("hard-close-approve");
        }}
      />

      {/* S3 step 2b: Hard-close approve confirm (destructive, after MFA) */}
      <HardCloseApproveConfirmDialog
        open={activeDialog === "hard-close-approve"}
        onOpenChange={(o) => !o && close()}
        periodeId={periodeId}
        periodeKode={periodeKode}
        rowVersion={rowVersion}
        stepUpToken={stepUpToken}
        onSuccess={() => { close(); setStepUpToken(""); onTransitionSuccess(); }}
      />

      {/* S3 step 3 alt: Hard-close reject (CFO, no MFA) */}
      <HardCloseRejectDialog
        open={activeDialog === "hard-close-reject"}
        onOpenChange={(o) => !o && close()}
        periodeId={periodeId}
        periodeKode={periodeKode}
        onSuccess={() => { close(); onTransitionSuccess(); }}
      />

      {/* S4 step 1: Reopen request */}
      <ReopenRequestDialog
        open={activeDialog === "reopen-request"}
        onOpenChange={(o) => !o && close()}
        periodeId={periodeId}
        periodeKode={periodeKode}
        currentStatus={statusPeriode}
        rowVersion={rowVersion}
        onSuccess={() => { close(); onTransitionSuccess(); }}
      />

      {/* S4 step 2a: MFA for CLOSED→SOFT_CLOSED reopen */}
      <MFAStepUpDialog
        open={activeDialog === "reopen-mfa"}
        onOpenChange={(o) => !o && close()}
        scope="reopen_closed"
        actionDescription={`Approve reopen exceptional ${periodeKode} ke SOFT_CLOSED`}
        periodeKode={periodeKode}
        onTokenReceived={(token) => {
          setStepUpToken(token);
          setActiveDialog("reopen-approve");
        }}
      />

      {/* S4 step 2b / S4 step 3: Reopen approve confirm */}
      <ReopenApproveConfirmDialog
        open={activeDialog === "reopen-approve"}
        onOpenChange={(o) => !o && close()}
        periodeId={periodeId}
        periodeKode={periodeKode}
        targetStatus={pendingReopenTargetStatus ?? "OPEN"}
        stepUpToken={stepUpToken || undefined}
        onSuccess={() => { close(); setStepUpToken(""); onTransitionSuccess(); }}
      />
    </>
  );
}

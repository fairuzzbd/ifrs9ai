/**
 * MVRefreshButton — Trigger manual MV refresh.
 * Absent from DOM when user does NOT have ROLE-IT-ADMIN.
 * ROLE-IT-ADMIN only per S2-AC3 + personas.md.
 */

"use client";

import * as React from "react";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface MVRefreshButtonProps {
  mvName?: string | null;
  isITAdmin: boolean;
  loading?: boolean;
  onClick: (mvName?: string | null) => void;
  className?: string;
}

export function MVRefreshButton({
  mvName,
  isITAdmin,
  loading = false,
  onClick,
  className,
}: MVRefreshButtonProps) {
  // Absent from DOM when not IT-ADMIN (not just hidden — security pattern)
  if (!isITAdmin) return null;

  const label = mvName
    ? `Refresh ${mvName.replace("rpt.mv_", "").replace(/_/g, " ")}`
    : "Refresh Semua MV";

  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={() => onClick(mvName)}
      disabled={loading}
      aria-label={label}
      aria-busy={loading}
      className={cn("gap-1.5", className)}
    >
      <RefreshCw
        className={cn("h-4 w-4", loading && "animate-spin")}
        aria-hidden="true"
      />
      {label}
    </Button>
  );
}

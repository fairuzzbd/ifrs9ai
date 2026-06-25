/**
 * MappingRegulatedBadge — indicates regulated_flag with PSAK ref tooltip.
 * Regulated events require 6-eyes workflow (ROLE-RISK approve-2 + step-up MFA).
 */

import * as React from "react";
import { Shield, ShieldOff } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

interface MappingRegulatedBadgeProps {
  regulated: boolean;
  workflowPath?: "4-eyes" | "6-eyes";
  className?: string;
}

export function MappingRegulatedBadge({
  regulated,
  workflowPath,
  className,
}: MappingRegulatedBadgeProps) {
  if (!regulated) {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              className={cn(
                "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
                "bg-gray-50 text-gray-500 border-gray-200",
                className,
              )}
              aria-label="Event operasional — jalur 4-eyes"
            >
              <ShieldOff className="h-3 w-3" aria-hidden="true" />
              <span>4-eyes</span>
            </span>
          </TooltipTrigger>
          <TooltipContent>
            <p className="text-xs max-w-[200px]">
              Event operasional — jalur 4-eyes (ROLE-AKUN-CTL approver). Tidak memerlukan ROLE-RISK.
            </p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            className={cn(
              "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
              "bg-rose-50 text-rose-700 border-rose-200",
              className,
            )}
            aria-label="Event regulated — jalur 6-eyes, memerlukan ROLE-RISK"
          >
            <Shield className="h-3 w-3" aria-hidden="true" />
            <span>6-eyes (RISK)</span>
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <p className="text-xs max-w-[240px]">
            Event regulated PSAK 71 (ECL/EIR/MTM/REKLAS).{" "}
            {workflowPath === "6-eyes"
              ? "Memerlukan approver-2 ROLE-RISK + MFA step-up (DEC-027)."
              : "Akan dirutekan ke jalur 6-eyes saat submit."}
          </p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

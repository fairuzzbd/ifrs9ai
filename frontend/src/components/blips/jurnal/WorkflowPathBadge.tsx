"use client";

import * as React from "react";
import { Tooltip, TooltipContent, TooltipTrigger, TooltipProvider } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export interface WorkflowPathBadgeProps {
  path: "4-eyes" | "6-eyes";
  size?: "sm" | "md";
  showTooltip?: boolean;
  className?: string;
}

export function WorkflowPathBadge({
  path,
  size = "md",
  showTooltip = true,
  className,
}: WorkflowPathBadgeProps) {
  const is6Eyes = path === "6-eyes";
  const count = is6Eyes ? 6 : 4;
  const label = is6Eyes ? "6-Mata" : "4-Mata";
  const tooltip = is6Eyes
    ? "Workflow 6 Mata: Maker → Reviewer → Approver → Approver Kedua (ROLE-RISK)"
    : "Workflow 4 Mata: Maker → Reviewer → Approver";

  const diamondSize = size === "sm" ? "h-2 w-2" : "h-2.5 w-2.5";
  const colorClass = is6Eyes ? "text-purple-600" : "text-blue-600";
  const bgClass = is6Eyes
    ? "bg-purple-50 border-purple-200 text-purple-700"
    : "bg-blue-50 border-blue-200 text-blue-700";

  const badge = (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5",
        size === "sm" ? "text-xs" : "text-xs",
        bgClass,
        className,
      )}
      aria-label={`Workflow path: ${label}`}
    >
      <span className="flex items-center gap-0.5" aria-hidden="true">
        {Array.from({ length: count }).map((_, i) => (
          <svg
            key={i}
            className={cn(diamondSize, colorClass)}
            viewBox="0 0 10 10"
            fill="currentColor"
          >
            <path d="M5 0L10 5L5 10L0 5Z" />
          </svg>
        ))}
      </span>
      <span>{label}</span>
    </span>
  );

  if (!showTooltip) return badge;

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>{badge}</TooltipTrigger>
        <TooltipContent>
          <p className="text-xs max-w-xs">{tooltip}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

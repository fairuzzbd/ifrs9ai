"use client";

import * as React from "react";
import { Copy, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface SignatureHashBadgeProps {
  hash: string;
  label?: string;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function SignatureHashBadge({
  hash,
  label = "Tanda Tangan Digital",
  className,
}: SignatureHashBadgeProps) {
  const [copied, setCopied] = React.useState(false);

  const truncated = hash.slice(0, 16);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(hash);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      /* ignore */
    }
  };

  return (
    <div className={cn("flex items-center gap-2", className)}>
      <span className="text-sm text-muted-foreground">{label}:</span>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            className="font-mono text-xs bg-muted px-2 py-1 rounded cursor-default"
            aria-label={`Hash tanda tangan: ${truncated}...`}
          >
            {truncated}...
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <p className="font-mono text-xs break-all max-w-xs">{hash}</p>
          <p className="text-xs text-muted-foreground mt-1">
            SHA-256 signature. Klik salin untuk full hash.
          </p>
        </TooltipContent>
      </Tooltip>
      <Button
        variant="ghost"
        size="icon"
        className="h-7 w-7"
        onClick={() => void handleCopy()}
        aria-label="Salin signature hash"
      >
        {copied ? (
          <Check className="h-3.5 w-3.5 text-green-600" aria-hidden="true" />
        ) : (
          <Copy className="h-3.5 w-3.5" aria-hidden="true" />
        )}
      </Button>
    </div>
  );
}

import * as React from "react";
import { CornerDownLeft } from "lucide-react";
import { cn } from "@/lib/utils";
import { format, parseISO } from "date-fns";

interface ReturnedBannerProps {
  rejectedBy: string;
  rejectedAt: string;
  comment: string;
  className?: string;
}

export function ReturnedBanner({
  rejectedBy,
  rejectedAt,
  comment,
  className,
}: ReturnedBannerProps) {
  const formattedDate = React.useMemo(() => {
    try {
      return format(parseISO(rejectedAt), "dd MMM yyyy, HH:mm 'WIB'");
    } catch {
      return rejectedAt;
    }
  }, [rejectedAt]);

  return (
    <div
      role="alert"
      className={cn(
        "rounded-md border border-amber-300 bg-amber-50 p-4",
        className,
      )}
    >
      <div className="flex items-start gap-3">
        <CornerDownLeft
          className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
          aria-hidden
        />
        <div className="space-y-1">
          <p className="text-sm font-medium text-amber-800">
            Dikembalikan oleh: {rejectedBy}
          </p>
          <p className="text-xs text-amber-700">{formattedDate}</p>
          {comment && (
            <blockquote className="mt-1 border-l-2 border-amber-400 pl-3 text-sm italic text-amber-800">
              &ldquo;{comment}&rdquo;
            </blockquote>
          )}
        </div>
      </div>
    </div>
  );
}

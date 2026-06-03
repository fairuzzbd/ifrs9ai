import * as React from "react";
import { Info } from "lucide-react";
import { cn } from "@/lib/utils";

interface SodBlockBannerProps {
  message?: string;
  className?: string;
}

export function SodBlockBanner({
  message = "Anda tidak bisa mereview atau menyetujui data yang Anda buat sendiri (Segregation of Duties / 4-Eyes Policy). Hubungi Finance Controller untuk melanjutkan.",
  className,
}: SodBlockBannerProps) {
  return (
    <div
      role="status"
      className={cn(
        "flex items-start gap-3 rounded-md border border-blue-200 bg-blue-50 p-4",
        className,
      )}
    >
      <Info className="mt-0.5 h-4 w-4 shrink-0 text-blue-600" aria-hidden />
      <p className="text-sm text-blue-800">{message}</p>
    </div>
  );
}

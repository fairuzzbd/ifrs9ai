"use client";

import * as React from "react";
import { Suspense } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { RenewalPreviewPanel } from "@/components/blips/renewal/RenewalPreviewPanel";
import { renewalDetailApi, renewalQueryKeys } from "@/lib/api/renewal.api";

// ---------------------------------------------------------------------------
// Read-only preview page — server-recomputed kalkulasi (S4)
// Also usable as embedded iframe or standalone shareable link.
// ---------------------------------------------------------------------------

function RenewalPreviewContent() {
  const params = useParams<{ id: string }>();

  const { data: previewData, isLoading: previewLoading } = useQuery({
    queryKey: renewalQueryKeys.preview(params.id),
    queryFn: () => renewalDetailApi.preview(params.id),
    staleTime: 60_000,
  });

  const { data: detailData } = useQuery({
    queryKey: renewalQueryKeys.detail(params.id),
    queryFn: () => renewalDetailApi.get(params.id),
    staleTime: 60_000,
  });

  const preview = previewData?.data;
  const renewal = detailData?.data;

  return (
    <div className="container mx-auto py-6 space-y-4 max-w-2xl">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/transaksi/renewal" className="hover:underline">
          Renewal Deposito
        </Link>
        <span className="mx-1.5">/</span>
        <Link
          href={`/transaksi/renewal/${params.id}`}
          className="hover:underline"
        >
          {renewal?.instrumenLamaKode ?? params.id.slice(0, 8)}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Preview Kalkulasi</span>
      </nav>

      <div className="flex items-center gap-3">
        <Button variant="ghost" size="sm" asChild>
          <Link
            href={`/transaksi/renewal/${params.id}`}
            aria-label="Kembali ke detail renewal"
          >
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          </Link>
        </Button>
        <h1 className="text-xl font-semibold">Preview Kalkulasi Renewal</h1>
      </div>

      <p className="text-sm text-muted-foreground">
        Server recompute read-only. Nilai ini digunakan ROLE-APPR-TR untuk
        verifikasi sebelum approve. PPh 20% per PP No. 131/2000. EIR via
        Newton-Raphson (DEC-013).
      </p>

      {previewLoading && (
        <div className="space-y-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      )}

      {preview && (
        <RenewalPreviewPanel
          preview={preview}
          tanggalJatuhTempoBaru={renewal?.tanggalJatuhTempoBaru}
          showSchedule
        />
      )}

      {!previewLoading && !preview && (
        <div className="text-center text-muted-foreground py-10">
          Preview tidak tersedia untuk renewal ini.
        </div>
      )}
    </div>
  );
}

export default function RenewalPreviewPage() {
  return (
    <Suspense>
      <RenewalPreviewContent />
    </Suspense>
  );
}

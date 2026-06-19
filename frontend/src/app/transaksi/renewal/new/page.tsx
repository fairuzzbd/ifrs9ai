"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { RenewalNewForm } from "@/components/blips/renewal/RenewalNewForm";
import { usePermissions } from "@/lib/stores/auth.store";

// ---------------------------------------------------------------------------
// Content (inside Suspense so useSearchParams works with App Router)
// ---------------------------------------------------------------------------

function NewRenewalContent() {
  const perms = usePermissions();
  const searchParams = useSearchParams();

  // Support pre-populating instrumen from query param e.g. ?instrumenId=...&kode=DEP-0042
  const instrumenId = searchParams.get("instrumenId") ?? undefined;
  const instrumenKode = searchParams.get("kode") ?? undefined;

  if (!perms.canCreate("transaksi")) {
    return (
      <div className="container mx-auto py-10 text-center text-muted-foreground">
        <p>Anda tidak memiliki izin untuk membuat renewal deposito.</p>
        <p className="text-xs mt-1">Permission: transaksi.create (ROLE-MAKER-TR)</p>
      </div>
    );
  }

  return (
    <div className="container mx-auto py-6 space-y-6">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/transaksi/renewal" className="hover:underline">
          Renewal Deposito
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Buat Renewal Baru</span>
      </nav>

      <div className="flex items-center gap-3">
        <Button variant="ghost" size="sm" asChild>
          <Link href="/transaksi/renewal" aria-label="Kembali ke daftar renewal">
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-semibold">Buat Renewal Deposito</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            ROLE-MAKER-TR — renewal dikirim langsung ke status PENDING_APPROVAL.
          </p>
        </div>
      </div>

      <RenewalNewForm
        instrumenId={instrumenId}
        instrumenKode={instrumenKode}
      />
    </div>
  );
}

export default function NewRenewalPage() {
  return (
    <Suspense>
      <NewRenewalContent />
    </Suspense>
  );
}

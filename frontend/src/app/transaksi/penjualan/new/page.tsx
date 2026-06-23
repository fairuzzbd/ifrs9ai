"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PenjualanNewForm } from "@/components/blips/penjualan/PenjualanNewForm";
import { usePermissions } from "@/lib/stores/auth.store";

// ---------------------------------------------------------------------------
// Content (inside Suspense so useSearchParams works with App Router)
// ---------------------------------------------------------------------------

function NewPenjualanContent() {
  const perms = usePermissions();
  const searchParams = useSearchParams();

  const instrumenId = searchParams.get("instrumenId") ?? undefined;
  const instrumenKode = searchParams.get("kode") ?? undefined;

  if (!perms.canCreate?.("transaksi")) {
    return (
      <div className="container mx-auto py-10 text-center text-muted-foreground">
        <p>Anda tidak memiliki izin untuk membuat penjualan instrumen.</p>
        <p className="text-xs mt-1">Permission: penjualan.create (ROLE-MAKER-TR)</p>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-6">
      <div className="flex items-center gap-3">
        <Link href="/transaksi/penjualan">
          <Button variant="ghost" size="sm" aria-label="Kembali ke daftar penjualan">
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            Kembali
          </Button>
        </Link>
        <div>
          <h1 className="text-2xl font-bold">Buat Penjualan Instrumen</h1>
          <p className="text-sm text-muted-foreground">
            {instrumenKode ? `Instrumen: ${instrumenKode}` : "Pilih instrumen ACTIVE dengan klasifikasi terkunci"}
          </p>
        </div>
      </div>

      <PenjualanNewForm
        instrumenId={instrumenId}
        instrumenKode={instrumenKode}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function NewPenjualanPage() {
  return (
    <Suspense>
      <NewPenjualanContent />
    </Suspense>
  );
}

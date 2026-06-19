"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { MtmUploadDropzone } from "@/components/blips/mtm/MtmUploadDropzone";
import type { MtmUploadBatchResponse } from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Page — S2: Upload Manual MTM
// Persona: ROLE-AKUN (mtm.create) — middleware or server component guards route
// ---------------------------------------------------------------------------

function MtmUploadContent() {
  const [batchResult, setBatchResult] = React.useState<MtmUploadBatchResponse | null>(null);

  const handleUploadSuccess = (result: MtmUploadBatchResponse) => {
    setBatchResult(result);
  };

  return (
    <div className="container mx-auto py-6 max-w-2xl space-y-5">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="flex items-center gap-2 text-sm text-muted-foreground">
        <Button variant="ghost" size="sm" className="h-7 gap-1 px-2" asChild>
          <Link href="/mtm" aria-label="Kembali ke daftar MTM">
            <ArrowLeft className="h-3.5 w-3.5" aria-hidden />
            Kembali
          </Link>
        </Button>
        <span>/</span>
        <Link href="/mtm" className="hover:underline">MTM Harian</Link>
        <span>/</span>
        <span className="text-foreground font-medium">Upload Manual</span>
      </nav>

      <div>
        <h1 className="text-2xl font-semibold">Upload Harga MTM Manual</h1>
        <p className="text-muted-foreground text-sm mt-1">
          Upload file XLSX atau CSV dengan harga pasar instrumen non-AC. Batch akan langsung masuk antrian PENDING_REVIEW.
        </p>
      </div>

      <MtmUploadDropzone onSuccess={handleUploadSuccess} />

      {/* Post-upload preview inline */}
      {batchResult && (
        <section
          className="rounded-lg border p-5 space-y-4"
          aria-labelledby="batch-preview-heading"
        >
          <h2 id="batch-preview-heading" className="text-base font-semibold">
            Hasil Upload — Batch {batchResult.uploadBatchId.slice(0, 8)}...
          </h2>

          {/* Summary stats */}
          <div className="grid grid-cols-3 gap-4 text-center">
            <div className="rounded-md border p-3">
              <p className="text-2xl font-bold">{batchResult.rowsParsed}</p>
              <p className="text-xs text-muted-foreground mt-0.5">Baris Dibaca</p>
            </div>
            <div className="rounded-md border p-3">
              <p className="text-2xl font-bold text-green-700">{batchResult.rowsValid}</p>
              <p className="text-xs text-muted-foreground mt-0.5">Baris Valid</p>
            </div>
            <div className="rounded-md border p-3">
              <p className={`text-2xl font-bold ${batchResult.rowsInvalid > 0 ? "text-destructive" : "text-muted-foreground"}`}>
                {batchResult.rowsInvalid}
              </p>
              <p className="text-xs text-muted-foreground mt-0.5">Baris Invalid</p>
            </div>
          </div>

          {/* Deviation warnings */}
          {batchResult.deviationWarnings.length > 0 && (
            <div className="rounded-md border border-amber-200 bg-amber-50 p-3 space-y-1">
              <p className="text-sm font-semibold text-amber-800">
                {batchResult.deviationWarnings.length} Peringatan Deviasi
              </p>
              <ul className="space-y-0.5">
                {batchResult.deviationWarnings.map((w, i) => (
                  <li key={i} className="text-xs text-amber-700">
                    {w.instrumenKode}: delta {w.deltaPct >= 0 ? "+" : ""}{w.deltaPct.toFixed(2)}% (threshold {w.thresholdPct}%)
                  </li>
                ))}
              </ul>
            </div>
          )}

          {/* Stale price warnings */}
          {batchResult.stalePriceWarnings.length > 0 && (
            <div className="rounded-md border border-amber-200 bg-amber-50 p-3 space-y-1">
              <p className="text-sm font-semibold text-amber-800">
                {batchResult.stalePriceWarnings.length} Peringatan Harga Kedaluwarsa
              </p>
              <ul className="space-y-0.5">
                {batchResult.stalePriceWarnings.map((w, i) => (
                  <li key={i} className="text-xs text-amber-700">{w}</li>
                ))}
              </ul>
            </div>
          )}

          {/* Rows preview */}
          {batchResult.rowsCreated.length > 0 && (
            <div>
              <h3 className="text-sm font-medium mb-2">Baris Diproses (maks. 5)</h3>
              <div className="rounded-md border overflow-x-auto">
                <table className="w-full text-xs">
                  <thead className="bg-muted/40">
                    <tr>
                      <th className="px-3 py-2 text-left font-medium">Instrumen</th>
                      <th className="px-3 py-2 text-left font-medium">Tanggal</th>
                      <th className="px-3 py-2 text-right font-medium">Harga (IDR)</th>
                      <th className="px-3 py-2 text-right font-medium">Delta %</th>
                      <th className="px-3 py-2 text-left font-medium">Sumber</th>
                    </tr>
                  </thead>
                  <tbody>
                    {batchResult.rowsCreated.slice(0, 5).map((row, i) => (
                      <tr key={i} className="border-t">
                        <td className="px-3 py-2 font-mono">{row.instrumenKode}</td>
                        <td className="px-3 py-2">{row.tanggalMtm}</td>
                        <td className="px-3 py-2 text-right font-mono">
                          {new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", minimumFractionDigits: 0 }).format(row.hargaPasarIdr)}
                        </td>
                        <td className={`px-3 py-2 text-right font-mono ${row.deltaPct >= 0 ? "text-green-700" : "text-red-700"}`}>
                          {row.deltaPct >= 0 ? "+" : ""}{row.deltaPct.toFixed(2)}%
                        </td>
                        <td className="px-3 py-2">{row.hargaSumber}</td>
                      </tr>
                    ))}
                    {batchResult.rowsCreated.length > 5 && (
                      <tr className="border-t">
                        <td colSpan={5} className="px-3 py-2 text-center text-muted-foreground">
                          ... dan {batchResult.rowsCreated.length - 5} baris lainnya. <Link href={`/mtm/upload/batch/${batchResult.uploadBatchId}`} className="text-primary hover:underline">Lihat semua</Link>
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Next step */}
          <p className="text-xs text-muted-foreground bg-muted/40 rounded px-3 py-2">
            {batchResult.nextStep}
          </p>

          <div className="flex gap-2">
            <Button variant="default" size="sm" asChild>
              <Link href={`/mtm/upload/batch/${batchResult.uploadBatchId}`}>
                Lihat Detail Batch
              </Link>
            </Button>
            <Button variant="outline" size="sm" asChild>
              <Link href="/mtm">Kembali ke Daftar MTM</Link>
            </Button>
          </div>
        </section>
      )}
    </div>
  );
}

export default function MtmUploadPage() {
  return (
    <Suspense>
      <MtmUploadContent />
    </Suspense>
  );
}

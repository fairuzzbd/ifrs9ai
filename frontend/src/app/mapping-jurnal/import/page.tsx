/**
 * Route: /mapping-jurnal/import
 * Story: P5-M12-S3 — Import XLSX bulk mapping (re-uses M11 upload_batch pattern)
 * Actor: ROLE-AKUN
 */

"use client";

import * as React from "react";
import { Suspense } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { v4 as uuidv4 } from "uuid";
import { ArrowLeft } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

import { MappingImportDropzone } from "@/components/blips/mapping-jurnal/MappingImportDropzone";
import { mappingImportApi } from "@/lib/api/mapping-jurnal-p12.api";
import { notify } from "@/lib/notify";
import { ApiError } from "@/lib/api";
import type { BulkImportResult } from "@/lib/schemas/mapping-jurnal-p12.schema";

function MappingImportContent() {
  const router = useRouter();
  const [selectedFile, setSelectedFile] = React.useState<File | null>(null);
  const [uploading, setUploading] = React.useState(false);
  const [result, setResult] = React.useState<BulkImportResult | null>(null);
  const [idempotencyKey] = React.useState(() => uuidv4());

  async function handleImport() {
    if (!selectedFile) return;
    setUploading(true);
    try {
      const res = await mappingImportApi.import(selectedFile, idempotencyKey);
      const data = res.data;
      setResult(data);
      if (data.invalidRows > 0) {
        notify.warning(
          `Import mapping diparsing: ${data.validRows} baris valid (DRAFT dibuat), ${data.invalidRows} baris gagal. Lihat detail di bawah.`,
        );
      } else {
        notify.success(
          `Import mapping berhasil. ${data.validRows} baris valid — DRAFT dibuat. Batch: ${data.batchId}`,
          {
            action: {
              label: "Lihat daftar mapping",
              onClick: () => router.push("/mapping-jurnal"),
            },
          },
        );
      }
    } catch (err) {
      if (err instanceof ApiError) {
        notify.error(err);
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal upload file", traceId: "" });
      }
    } finally {
      setUploading(false);
    }
  }

  return (
    <div className="container mx-auto py-6 space-y-6 max-w-3xl">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/mapping-jurnal" className="hover:underline">Mapping Jurnal</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Import Bulk</span>
      </nav>

      <div>
        <h1 className="text-2xl font-semibold">Import Bulk Mapping Jurnal</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Upload XLSX untuk mengupdate banyak mapping sekaligus. Semua baris valid akan dibuat sebagai DRAFT baru
          yang harus melalui workflow approval.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Upload File XLSX</CardTitle>
          <CardDescription>
            File XLSX dengan kolom: event_code, akun_debit, akun_kredit, debit_kredit, jumlah_calc, urutan.
            Maks 20MB. Server memvalidasi akun terhadap Chart of Accounts.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <MappingImportDropzone
            onFileSelected={setSelectedFile}
            disabled={uploading}
          />
          <div className="flex gap-3">
            <Button variant="outline" asChild>
              <Link href="/mapping-jurnal">
                <ArrowLeft className="mr-1.5 h-4 w-4" aria-hidden="true" />
                Kembali
              </Link>
            </Button>
            <Button
              onClick={handleImport}
              disabled={!selectedFile || uploading}
              aria-busy={uploading}
            >
              {uploading ? "Mengupload..." : "Import Mapping"}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Result panel */}
      {result && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm">Hasil Parsing — Batch {result.batchId}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-3 gap-3">
              <div className="rounded-lg border p-3 text-center">
                <p className="text-xl font-bold">{result.totalRows}</p>
                <p className="text-xs text-muted-foreground">Total Baris</p>
              </div>
              <div className="rounded-lg border border-green-200 bg-green-50 p-3 text-center">
                <p className="text-xl font-bold text-green-700">{result.validRows}</p>
                <p className="text-xs text-green-600">Valid (DRAFT)</p>
              </div>
              <div className="rounded-lg border border-red-200 bg-red-50 p-3 text-center">
                <p className="text-xl font-bold text-red-700">{result.invalidRows}</p>
                <p className="text-xs text-red-600">Gagal</p>
              </div>
            </div>

            {result.errors.length > 0 && (
              <div className="space-y-2">
                <h3 className="text-sm font-semibold text-destructive">
                  Error Parsing ({result.errors.length} baris)
                </h3>
                <div className="rounded-lg border overflow-x-auto">
                  <table className="w-full text-xs" aria-label="Error baris import">
                    <thead className="border-b bg-muted/50">
                      <tr>
                        <th scope="col" className="px-3 py-2 text-left font-medium text-muted-foreground">Baris</th>
                        <th scope="col" className="px-3 py-2 text-left font-medium text-muted-foreground">Kolom</th>
                        <th scope="col" className="px-3 py-2 text-left font-medium text-muted-foreground">Kode Error</th>
                        <th scope="col" className="px-3 py-2 text-left font-medium text-muted-foreground">Detail</th>
                      </tr>
                    </thead>
                    <tbody>
                      {result.errors.map((err, i) => (
                        <tr key={i} className="border-b last:border-0">
                          <td className="px-3 py-2">{err.row}</td>
                          <td className="px-3 py-2 font-mono">{err.col}</td>
                          <td className="px-3 py-2">
                            <Badge variant="outline" className="text-xs text-destructive border-destructive/30">
                              {err.errorCode}
                            </Badge>
                          </td>
                          <td className="px-3 py-2 text-muted-foreground">{err.error}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {result.validRows > 0 && (
              <div className="flex gap-2">
                <Button size="sm" asChild>
                  <Link href="/mapping-jurnal?filter[workflow_status]=DRAFT">
                    Lihat DRAFT baru &rarr;
                  </Link>
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

export default function MappingImportPage() {
  return (
    <Suspense>
      <MappingImportContent />
    </Suspense>
  );
}

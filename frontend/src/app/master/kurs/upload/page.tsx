"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowLeft, FileUp, Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { usePermissions } from "@/lib/stores/auth.store";
import { KursUploadDropzone } from "@/components/blips/fx-rate/KursUploadDropzone";
import type { KursUploadResponse } from "@/lib/schemas/fx-rate.schema";

// ---------------------------------------------------------------------------
// Page content (S2-AC1..4)
// ---------------------------------------------------------------------------

function KursUploadContent() {
  const router = useRouter();
  const perms = usePermissions();

  // Absent from DOM when no permission (not just disabled)
  if (!perms.can("kurs.create")) {
    return (
      <div className="container mx-auto py-6">
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          Anda tidak memiliki permission <code>kurs.create</code> untuk mengupload kurs manual.
          Hanya ROLE-AKUN yang dapat melakukan upload kurs.
        </div>
      </div>
    );
  }

  const handleSuccess = (response: KursUploadResponse) => {
    // Redirect to list filtered by batch
    void router.push(
      `/master/kurs?filter[upload_batch_id]=${response.uploadBatchId}&filter[workflow_status]=PENDING_APPROVAL`,
    );
  };

  return (
    <div className="container mx-auto py-6 space-y-5 max-w-3xl">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/kurs" className="hover:text-foreground transition-colors">
          Master Data / Kurs
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Upload Kurs Manual</span>
      </nav>

      <div className="flex items-center gap-3">
        <Button variant="outline" size="sm" asChild>
          <Link href="/master/kurs" aria-label="Kembali ke daftar kurs">
            <ArrowLeft className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Kembali
          </Link>
        </Button>
        <h1 className="text-2xl font-semibold flex items-center gap-2">
          <FileUp className="h-6 w-6 text-primary" aria-hidden="true" />
          Upload Kurs Manual
        </h1>
      </div>

      {/* Info banner */}
      <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
        <Info
          className="mt-0.5 h-4 w-4 shrink-0 text-blue-600"
          aria-hidden="true"
        />
        <div className="space-y-1">
          <p>
            <strong>Upload kurs manual</strong> digunakan ketika BI JISDOR feed
            gagal atau untuk mata uang yang tidak tersedia di JISDOR (mis. CNY via
            BI Kurs Tengah). Kurs akan masuk ke status{" "}
            <strong>PENDING_APPROVAL</strong> dan memerlukan persetujuan Finance
            Controller (ROLE-AKUN-CTL).
          </p>
          <p className="text-xs text-blue-700">
            Workflow: Upload (ROLE-AKUN) → Approve/Reject (ROLE-AKUN-CTL) — 4-eyes SoD.
          </p>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Form Upload</CardTitle>
        </CardHeader>
        <CardContent>
          <KursUploadDropzone
            onSuccess={handleSuccess}
            onCancel={() => router.push("/master/kurs")}
          />
        </CardContent>
      </Card>

      {/* Template download hint */}
      <Card className="border-dashed">
        <CardContent className="pt-4">
          <p className="text-sm text-muted-foreground">
            <strong>Template kolom wajib:</strong>{" "}
            <code className="font-mono text-xs bg-muted px-1 py-0.5 rounded">
              kode_mata_uang (CHAR3) · tanggal_berlaku (YYYY-MM-DD) · kurs_tengah (NUMERIC)
            </code>
          </p>
          <p className="text-xs text-muted-foreground mt-1">
            Kolom opsional: <code className="font-mono">kurs_beli · kurs_jual · catatan</code>
            {" "}(catatan wajib jika deviasi &gt; 20% dari hari sebelumnya, min 20 karakter).
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

export default function KursUploadPage() {
  return (
    <Suspense>
      <KursUploadContent />
    </Suspense>
  );
}

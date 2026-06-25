"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import { ChevronRight, Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { SettlementBalanceHintCard } from "@/components/blips/penempatan/SettlementBalanceHintCard";
import { EIRPreviewSidePanel } from "@/components/blips/penempatan/EIRPreviewSidePanel";
import { notify } from "@/lib/notify";
import { penempatanApi } from "@/lib/api/penempatan.api";
import { PenempatanCreateSchema } from "@/lib/schemas/penempatan.schema";
import type { PenempatanCreateInput, SettlementBalanceHint, EirPreviewResult, KlasifikasiPsak71 } from "@/lib/schemas/penempatan.schema";
import { isFvtpl } from "@/lib/schemas/penempatan.schema";
import { isApiError } from "@/lib/api";
import { addMonths, format } from "date-fns";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function computeJatuhTempo(tanggal: string, tenorBulan: number): string {
  try {
    const date = new Date(tanggal);
    return format(addMonths(date, tenorBulan), "yyyy-MM-dd");
  } catch {
    return "";
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function PenempatanNewPage() {
  const router = useRouter();
  const [submitting, setSubmitting] = React.useState(false);
  const [savedId, setSavedId] = React.useState<string | null>(null);
  const [settlementHint, setSettlementHint] = React.useState<SettlementBalanceHint | null>(null);
  const [eirPreview, setEirPreview] = React.useState<EirPreviewResult | null>(null);
  const [eirPreviewLoading, setEirPreviewLoading] = React.useState(false);
  const [selectedKlasifikasi, setSelectedKlasifikasi] = React.useState<KlasifikasiPsak71 | null>(null);

  const form = useForm<PenempatanCreateInput>({
    resolver: zodResolver(PenempatanCreateSchema),
    defaultValues: {
      tanggalPenempatan: format(new Date(), "yyyy-MM-dd"),
      tenorBulan: 12,
      biayaTransaksiIdr: "0.0000",
      kuponPersen: "",
      mataUangId: "",
      instrumenId: "",
      counterpartyBankId: "",
      periodeId: "",
    },
  });

  const watchTanggal = form.watch("tanggalPenempatan");
  const watchTenor = form.watch("tenorBulan");
  const watchSettlement = form.watch("settlementAccount");
  const watchNominalIdr = form.watch("nominalIdr");

  const jatuhTempo = React.useMemo(
    () =>
      watchTanggal && watchTenor
        ? computeJatuhTempo(watchTanggal, Number(watchTenor))
        : "",
    [watchTanggal, watchTenor],
  );

  // Load EIR preview
  const handleEirPreview = async () => {
    if (!savedId) {
      notify.warning("Simpan penempatan dulu sebelum menghitung EIR Preview.");
      return;
    }
    setEirPreviewLoading(true);
    try {
      const res = await penempatanApi.eirPreview(savedId);
      setEirPreview(res.data);
    } catch (err) {
      if (isApiError(err)) notify.error(err);
    } finally {
      setEirPreviewLoading(false);
    }
  };

  // Submit form
  const onSubmit = async (data: PenempatanCreateInput) => {
    setSubmitting(true);
    const idempotencyKey = uuidv4();
    try {
      const res = await penempatanApi.create(data, idempotencyKey);
      const penempatan = res.data;
      setSavedId(penempatan.id);
      if (penempatan.settlementBalanceHint) {
        setSettlementHint(penempatan.settlementBalanceHint);
      }
      notify.success(
        `Penempatan ${penempatan.kodeTransaksi} berhasil dibuat. Status: Konsep. Lampirkan dokumen jika belum, lalu submit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/transaksi/penempatan/${penempatan.id}`),
          },
        },
      );
      router.push(`/transaksi/penempatan/${penempatan.id}`);
    } catch (err) {
      if (isApiError(err)) {
        // Set field errors if validation
        if (err.code === "VALIDATION_FAILED" && err.details.length > 0) {
          err.details.forEach((d) => {
            const fieldMap: Record<string, keyof PenempatanCreateInput> = {
              "body.instrumenId": "instrumenId",
              "body.tenorBulan": "tenorBulan",
              "body.kuponPersen": "kuponPersen",
              "body.nominalIdr": "nominalIdr",
            };
            const f = fieldMap[d.field];
            if (f) form.setError(f, { message: d.message });
          });
        }
        notify.error(err);
      } else {
        notify.error({ code: "INTERNAL", message: "Gagal membuat penempatan", traceId: "" });
      }
    } finally {
      setSubmitting(false);
    }
  };

  const showFvtplBanner = selectedKlasifikasi ? isFvtpl(selectedKlasifikasi) : false;

  return (
    <div className="p-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="flex items-center gap-1 text-sm text-gray-500">
        <Link href="/transaksi/penempatan" className="hover:underline">
          Penempatan Deposito
        </Link>
        <ChevronRight className="h-4 w-4" aria-hidden="true" />
        <span className="text-gray-900">Buat Baru</span>
      </nav>

      <h1 className="text-xl font-semibold">Buat Penempatan Deposito</h1>

      <div className="flex gap-6">
        {/* Main form */}
        <div className="flex-1 min-w-0 space-y-4">
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">

              {/* Seksi 1: Instrumen & Counterparty */}
              <Card>
                <CardHeader>
                  <CardTitle className="text-sm font-medium">1. Instrumen &amp; Counterparty</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <FormField
                    control={form.control}
                    name="instrumenId"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Instrumen <span aria-hidden="true" className="text-destructive">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            placeholder="UUID instrumen (cari di master instrumen)"
                            aria-required="true"
                            onChange={(e) => {
                              field.onChange(e);
                              // In real app: fetch instrumen detail to get klasifikasi
                              // setSelectedKlasifikasi(instrumen.klasifikasiPsak71)
                            }}
                          />
                        </FormControl>
                        <FormMessage />
                        {showFvtplBanner && (
                          <p className="text-xs text-blue-600 mt-1">
                            Instrumen FVTPL — EIR dan ECL staging tidak diterapkan.
                          </p>
                        )}
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="counterpartyBankId"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Bank Counterparty <span aria-hidden="true" className="text-destructive">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input {...field} placeholder="UUID bank counterparty" aria-required="true" />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="periodeId"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Periode Buku <span aria-hidden="true" className="text-destructive">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input {...field} placeholder="UUID periode buku (status OPEN)" aria-required="true" />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </CardContent>
              </Card>

              {/* Seksi 2: Nominal & Mata Uang */}
              <Card>
                <CardHeader>
                  <CardTitle className="text-sm font-medium">2. Nominal &amp; Mata Uang</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <FormField
                    control={form.control}
                    name="mataUangId"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Mata Uang <span aria-hidden="true" className="text-destructive">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input {...field} placeholder="UUID mata uang (IDR/USD/...)" aria-required="true" />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="nominalIdr"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Nominal IDR <span aria-hidden="true" className="text-destructive">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type="text"
                            inputMode="numeric"
                            placeholder="0,0000"
                            aria-required="true"
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="biayaTransaksiIdr"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Biaya Transaksi (IDR)</FormLabel>
                        <FormControl>
                          <Input {...field} type="text" inputMode="numeric" placeholder="0,0000" />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </CardContent>
              </Card>

              {/* Seksi 3: Tenor & Kupon */}
              <Card>
                <CardHeader>
                  <CardTitle className="text-sm font-medium">3. Tenor &amp; Kupon</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <FormField
                    control={form.control}
                    name="tanggalPenempatan"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Tanggal Penempatan <span aria-hidden="true" className="text-destructive">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input {...field} type="date" aria-required="true" />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="tenorBulan"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Tenor (Bulan) <span aria-hidden="true" className="text-destructive">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type="number"
                            min={1}
                            inputMode="numeric"
                            aria-required="true"
                          />
                        </FormControl>
                        {jatuhTempo && (
                          <p className="text-xs text-muted-foreground">
                            Jatuh Tempo:{" "}
                            <span className="font-medium text-gray-700">{jatuhTempo}</span>
                          </p>
                        )}
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="kuponPersen"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          Kupon (%) <span aria-hidden="true" className="text-destructive">*</span>
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type="text"
                            inputMode="decimal"
                            placeholder="5,25000000"
                            aria-required="true"
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="nomorReferensiBankIn"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Nomor Referensi Bank</FormLabel>
                        <FormControl>
                          <Input {...field} placeholder="BCA/DEP/2026/001" />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </CardContent>
              </Card>

              {/* Seksi 4: Settlement & Dokumen */}
              <Card>
                <CardHeader>
                  <CardTitle className="text-sm font-medium">4. Settlement &amp; Dokumen</CardTitle>
                </CardHeader>
                <CardContent className="space-y-4">
                  <FormField
                    control={form.control}
                    name="settlementAccount"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Rekening Settlement</FormLabel>
                        <FormControl>
                          <Input {...field} placeholder="1234567890" />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="catatan"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Catatan</FormLabel>
                        <FormControl>
                          <Textarea {...field} rows={3} maxLength={2000} placeholder="Catatan tambahan (maks 2000 karakter)" />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </CardContent>
              </Card>

              {/* FVTPL banner */}
              {showFvtplBanner && (
                <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 p-4">
                  <Info className="mt-0.5 h-5 w-5 shrink-0 text-blue-500" aria-hidden="true" />
                  <div>
                    <p className="text-sm font-medium text-blue-800">FVTPL</p>
                    <p className="text-sm text-blue-700 mt-1">
                      Instrumen ini tidak memerlukan ECL staging atau EIR computation (PSAK 71
                      §5.5.15). Fair value akan diproses oleh MTM engine (P5-M6).
                    </p>
                  </div>
                </div>
              )}

              {/* Form footer */}
              <div className="flex items-center gap-3 pt-2">
                <Button type="submit" disabled={submitting}>
                  {submitting ? "Menyimpan..." : "Simpan sebagai Konsep"}
                </Button>
                <Button type="button" variant="outline" asChild>
                  <Link href="/transaksi/penempatan">Batal</Link>
                </Button>
              </div>
            </form>
          </Form>
        </div>

        {/* Sidebar */}
        <div className="w-72 shrink-0 space-y-4">
          <SettlementBalanceHintCard
            hint={settlementHint}
            nominalIdrRaw={watchNominalIdr}
            settlementAccount={watchSettlement}
          />

          <EIRPreviewSidePanel
            workflowStatus="DRAFT"
            klasifikasiPsak71={selectedKlasifikasi}
            eirPreviewResult={eirPreview}
            eirPreviewLoading={eirPreviewLoading}
            onRequestPreview={handleEirPreview}
          />
        </div>
      </div>
    </div>
  );
}

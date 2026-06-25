"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import { addMonths, format as dateFnsFormat } from "date-fns";
import { ChevronRight, Lock } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { SettlementBalanceHintCard } from "@/components/blips/penempatan/SettlementBalanceHintCard";
import { PenempatanStatusBadge } from "@/components/blips/penempatan/PenempatanStatusBadge";
import { notify } from "@/lib/notify";
import { penempatanApi } from "@/lib/api/penempatan.api";
import { PenempatanUpdateSchema, type PenempatanUpdateInput } from "@/lib/schemas/penempatan.schema";
import type { PenempatanDeposito } from "@/lib/schemas/penempatan.schema";
import { isApiError, isValidationError } from "@/lib/api";
import { isFvtpl } from "@/lib/schemas/penempatan.schema";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatDate(s: string | null | undefined): string {
  if (!s) return "-";
  try {
    return dateFnsFormat(new Date(s), "d MMMM yyyy");
  } catch {
    return s;
  }
}

function computeJatuhTempo(tanggal: string, tenorBulan: number): string | null {
  try {
    const d = new Date(tanggal);
    if (isNaN(d.getTime())) return null;
    return dateFnsFormat(addMonths(d, tenorBulan), "d MMMM yyyy");
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

interface PageProps {
  params: { id: string };
}

export default function PenempatanEditPage({ params }: PageProps) {
  const router = useRouter();
  const { id } = params;

  const [penempatan, setPenempatan] = React.useState<PenempatanDeposito | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [submitting, setSubmitting] = React.useState(false);

  // ── Load DRAFT ─────────────────────────────────────────────────────────────

  React.useEffect(() => {
    async function load() {
      setLoading(true);
      try {
        const res = await penempatanApi.get(id);
        setPenempatan(res.data);
      } catch (err) {
        if (isApiError(err)) notify.error(err);
      } finally {
        setLoading(false);
      }
    }
    void load();
  }, [id]);

  // ── Form ──────────────────────────────────────────────────────────────────

  const form = useForm<PenempatanUpdateInput>({
    resolver: zodResolver(PenempatanUpdateSchema),
    defaultValues: {
      rowVersion: 1,
      tanggalPenempatan: "",
      nominalIdr: "",
      nominalFcy: "",
      tenorBulan: 1,
      kuponPersen: "",
      biayaTransaksiIdr: "",
      nomorReferensiBankIn: "",
      settlementAccount: "",
      catatan: "",
    },
  });

  // Populate form when penempatan loaded
  React.useEffect(() => {
    if (!penempatan) return;
    form.reset({
      rowVersion: penempatan.rowVersion,
      tanggalPenempatan: penempatan.tanggalPenempatan,
      nominalIdr: penempatan.nominalIdr?.toString() ?? "",
      nominalFcy: penempatan.nominalFcy?.toString() ?? "",
      tenorBulan: penempatan.tenorBulan,
      kuponPersen: penempatan.kuponPersen?.toString() ?? "",
      biayaTransaksiIdr: penempatan.biayaTransaksiIdr?.toString() ?? "0.0000",
      nomorReferensiBankIn: penempatan.nomorReferensiBankIn ?? "",
      settlementAccount: penempatan.settlementAccount ?? "",
      catatan: penempatan.catatan ?? "",
    });
  }, [penempatan, form]);

  const tanggalWatched = form.watch("tanggalPenempatan");
  const tenorWatched = form.watch("tenorBulan");

  const jatuhTempoPreview = React.useMemo(() => {
    if (!tanggalWatched || !tenorWatched) return null;
    return computeJatuhTempo(tanggalWatched, Number(tenorWatched));
  }, [tanggalWatched, tenorWatched]);

  // ── Guard: non-DRAFT ──────────────────────────────────────────────────────

  const isNotDraft = penempatan && penempatan.workflowStatus !== "DRAFT";

  // ── Submit ────────────────────────────────────────────────────────────────

  const onSubmit = async (data: PenempatanUpdateInput) => {
    if (isNotDraft) return;
    setSubmitting(true);
    const idempotencyKey = uuidv4();
    try {
      await penempatanApi.update(id, data, idempotencyKey);
      notify.success(
        `Penempatan ${penempatan!.kodeTransaksi} berhasil diperbarui.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/transaksi/penempatan/${id}`),
          },
        },
      );
      router.push(`/transaksi/penempatan/${id}`);
    } catch (err) {
      if (isValidationError(err)) {
        err.details.forEach((d) => {
          const fieldMap: Record<string, keyof PenempatanUpdateInput> = {
            "body.tanggalPenempatan": "tanggalPenempatan",
            "body.nominalIdr": "nominalIdr",
            "body.tenorBulan": "tenorBulan",
            "body.kuponPersen": "kuponPersen",
          };
          const fKey = fieldMap[d.field];
          if (fKey) form.setError(fKey, { message: d.message });
        });
        notify.error({
          ...err,
          message: `${err.details.length} field bermasalah — lihat form di bawah`,
        });
      } else if (isApiError(err)) {
        if (err.code === "CONFLICT") {
          notify.error({
            ...err,
            message:
              "Data berubah sejak terakhir dimuat. Muat ulang halaman dan coba lagi.",
          });
        } else {
          notify.error(err);
        }
      }
    } finally {
      setSubmitting(false);
    }
  };

  // ── Loading ───────────────────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-8 w-64 bg-gray-200 rounded" />
          <div className="h-64 bg-gray-200 rounded" />
        </div>
      </div>
    );
  }

  if (!penempatan) {
    return (
      <div className="p-6">
        <p className="text-gray-500">Penempatan tidak ditemukan.</p>
        <Button asChild className="mt-4">
          <Link href="/transaksi/penempatan">Kembali</Link>
        </Button>
      </div>
    );
  }

  // Locked guard
  if (isNotDraft) {
    return (
      <div className="p-6">
        <nav aria-label="Breadcrumb" className="flex items-center gap-1 text-sm text-gray-500 mb-6">
          <Link href="/transaksi/penempatan" className="hover:underline">Penempatan Deposito</Link>
          <ChevronRight className="h-4 w-4" aria-hidden="true" />
          <Link href={`/transaksi/penempatan/${id}`} className="hover:underline">{penempatan.kodeTransaksi}</Link>
          <ChevronRight className="h-4 w-4" aria-hidden="true" />
          <span className="text-gray-900">Edit</span>
        </nav>

        <Card>
          <CardContent className="py-10 text-center">
            <Lock className="mx-auto h-8 w-8 text-gray-400 mb-3" aria-hidden="true" />
            <p className="text-sm text-gray-600 mb-4">
              Penempatan <strong>{penempatan.kodeTransaksi}</strong> tidak dapat diedit karena status bukan DRAFT.
            </p>
            <PenempatanStatusBadge status={penempatan.workflowStatus} />
            <div className="mt-6">
              <Button asChild variant="outline">
                <Link href={`/transaksi/penempatan/${id}`}>Lihat Detail</Link>
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  const fvtpl = isFvtpl(penempatan.klasifikasiPsak71);

  return (
    <div className="p-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="flex items-center gap-1 text-sm text-gray-500">
        <Link href="/transaksi/penempatan" className="hover:underline">Penempatan Deposito</Link>
        <ChevronRight className="h-4 w-4" aria-hidden="true" />
        <Link href={`/transaksi/penempatan/${id}`} className="hover:underline">
          {penempatan.kodeTransaksi}
        </Link>
        <ChevronRight className="h-4 w-4" aria-hidden="true" />
        <span className="text-gray-900">Edit</span>
      </nav>

      {/* Page header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">
            Edit Penempatan Deposito — {penempatan.kodeTransaksi}
          </h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Bidang instrumen dan bank tidak dapat diubah setelah dibuat.
          </p>
        </div>
        <PenempatanStatusBadge status={penempatan.workflowStatus} />
      </div>

      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className="flex gap-6">
          <div className="flex-1 min-w-0 space-y-4">

            {/* Row 1: Locked fields */}
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-gray-700">
                  Informasi Tidak Dapat Diubah
                </CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="grid grid-cols-2 gap-4 text-sm">
                  <div>
                    <dt className="text-gray-500">Instrumen</dt>
                    <dd className="text-gray-900 font-medium mt-0.5">
                      {penempatan.instrumenKode ?? "-"} — {penempatan.instrumenNama ?? "-"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-gray-500">Bank Counterparty</dt>
                    <dd className="text-gray-900 font-medium mt-0.5">
                      {penempatan.counterpartyBankNama ?? "-"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-gray-500">Klasifikasi PSAK 71</dt>
                    <dd className="text-gray-900 font-medium mt-0.5">
                      {penempatan.klasifikasiPsak71 ?? "-"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-gray-500">Mata Uang</dt>
                    <dd className="text-gray-900 font-medium mt-0.5">
                      {penempatan.mataUangKode ?? "-"}
                    </dd>
                  </div>
                </dl>

                {/* Hidden row_version for optimistic locking */}
                <input type="hidden" {...form.register("rowVersion", { valueAsNumber: true })} />
              </CardContent>
            </Card>

            {/* FVTPL info banner */}
            {fvtpl && (
              <div className="rounded-md border border-blue-200 bg-blue-50 p-3 text-sm text-blue-700">
                Instrumen dengan klasifikasi <strong>{penempatan.klasifikasiPsak71}</strong> diukur
                pada Fair Value through P&amp;L. EIR dan ECL staging tidak diterapkan
                (PSAK 71 §5.5.15).
              </div>
            )}

            {/* Section 1: Placement date + tenor */}
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-gray-700">
                  Tanggal &amp; Tenor
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <FormField
                  control={form.control}
                  name="tanggalPenempatan"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Tanggal Penempatan <span aria-hidden="true">*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="date"
                          {...field}
                          aria-required="true"
                        />
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
                        Tenor (Bulan) <span aria-hidden="true">*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min={1}
                          {...field}
                          onChange={(e) => field.onChange(Number(e.target.value))}
                          aria-required="true"
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {jatuhTempoPreview && (
                  <div className="text-sm text-gray-600">
                    Tanggal Jatuh Tempo (preview):{" "}
                    <span className="font-semibold text-gray-900">{jatuhTempoPreview}</span>
                  </div>
                )}
              </CardContent>
            </Card>

            {/* Section 2: Financial terms */}
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-gray-700">
                  Nilai &amp; Kupon
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <FormField
                  control={form.control}
                  name="nominalIdr"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Nominal IDR</FormLabel>
                      <FormControl>
                        <Input
                          type="text"
                          inputMode="decimal"
                          placeholder="0.0000"
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {penempatan.mataUangKode && penempatan.mataUangKode !== "IDR" && (
                  <FormField
                    control={form.control}
                    name="nominalFcy"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Nominal {penempatan.mataUangKode}</FormLabel>
                        <FormControl>
                          <Input
                            type="text"
                            inputMode="decimal"
                            placeholder="0.0000"
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}

                <FormField
                  control={form.control}
                  name="kuponPersen"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Kupon (% per tahun) <span aria-hidden="true">*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="text"
                          inputMode="decimal"
                          placeholder="0.00000000"
                          {...field}
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
                        <Input
                          type="text"
                          inputMode="decimal"
                          placeholder="0.0000"
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </CardContent>
            </Card>

            {/* Section 3: References */}
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-gray-700">
                  Referensi &amp; Catatan
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <FormField
                  control={form.control}
                  name="settlementAccount"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Rekening Settlement</FormLabel>
                      <FormControl>
                        <Input {...field} placeholder="No. rekening settlement" maxLength={50} />
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
                      <FormLabel>No. Referensi Bank</FormLabel>
                      <FormControl>
                        <Input {...field} placeholder="Nomor referensi dari bank" maxLength={100} />
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
                        <Textarea
                          {...field}
                          placeholder="Catatan internal (opsional)"
                          rows={3}
                          maxLength={2000}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </CardContent>
            </Card>

            {/* Action buttons */}
            <div className="flex justify-end gap-3 pb-6">
              <Button
                type="button"
                variant="outline"
                asChild
                disabled={submitting}
              >
                <Link href={`/transaksi/penempatan/${id}`}>Batal</Link>
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting && (
                  <span className="mr-2 inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" aria-hidden="true" />
                )}
                Simpan Perubahan
              </Button>
            </div>
          </div>

          {/* Right sidebar */}
          <div className="w-72 shrink-0 space-y-4">
            <SettlementBalanceHintCard
              settlementAccount={
                form.watch("settlementAccount") ?? penempatan.settlementAccount ?? undefined
              }
              hint={penempatan.settlementBalanceHint}
            />

            {/* Meta info panel */}
            <Card>
              <CardContent className="pt-4">
                <p className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-3">
                  Informasi Record
                </p>
                <dl className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <dt className="text-gray-500">Dibuat oleh</dt>
                    <dd>{penempatan.makerNama ?? "-"}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="text-gray-500">Tanggal Buat</dt>
                    <dd>{formatDate(penempatan.createdAt)}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="text-gray-500">Terakhir Update</dt>
                    <dd>{formatDate(penempatan.updatedAt)}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="text-gray-500">Row Version</dt>
                    <dd className="font-mono">{penempatan.rowVersion}</dd>
                  </div>
                </dl>
              </CardContent>
            </Card>
          </div>
        </form>
      </Form>
    </div>
  );
}

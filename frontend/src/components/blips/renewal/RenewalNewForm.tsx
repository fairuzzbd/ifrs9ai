"use client";

import * as React from "react";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import { useRouter } from "next/navigation";
import { CalendarIcon } from "lucide-react";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import { renewalCreateApi, renewalQueryKeys } from "@/lib/api/renewal.api";
import {
  createRenewalSchema,
  type CreateRenewalInput,
  type CreateRenewalResponse,
} from "@/lib/schemas/renewal.schema";
import { RenewalPreviewPanel } from "./RenewalPreviewPanel";
import { useQueryClient } from "@tanstack/react-query";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RenewalNewFormProps {
  /** Pre-selected instrumen id (from query param or navigation) */
  instrumenId?: string;
  instrumenKode?: string;
  onSuccess?: (response: CreateRenewalResponse) => void;
}

// ---------------------------------------------------------------------------
// Component (S1 — Maker creates renewal; live preview after submit)
// ---------------------------------------------------------------------------

export function RenewalNewForm({
  instrumenId,
  instrumenKode,
  onSuccess,
}: RenewalNewFormProps) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const idempotencyKey = React.useRef(uuidv4());
  const [preview, setPreview] = React.useState<CreateRenewalResponse | null>(null);

  const form = useForm<CreateRenewalInput>({
    resolver: zodResolver(createRenewalSchema),
    defaultValues: {
      instrumenId: instrumenId ?? "",
      skema: "POKOK_SAJA",
      tenorBaruBulan: 12,
      rateBaruPersen: 0,
      tanggalEfektifBaru: "",
    },
  });

  const isSubmitting = form.formState.isSubmitting;

  const onSubmit = async (data: CreateRenewalInput) => {
    try {
      const result = await renewalCreateApi.create(data, idempotencyKey.current);
      const resp = result.data;
      setPreview(resp);

      notify.success(
        `Renewal ${instrumenKode ?? data.instrumenId} berhasil dibuat (${resp.renewalId.slice(0, 8)}...). Menunggu approval Treasury Approver.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/transaksi/renewal/${resp.renewalId}`),
          },
        },
      );

      await queryClient.invalidateQueries({ queryKey: renewalQueryKeys.lists() });
      onSuccess?.(resp);
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "VALIDATION_FAILED" && err.details.length > 0) {
          err.details.forEach((d) => {
            const fieldMap: Record<string, keyof CreateRenewalInput> = {
              instrumenId: "instrumenId",
              tenorBaruBulan: "tenorBaruBulan",
              rateBaruPersen: "rateBaruPersen",
              tanggalEfektifBaru: "tanggalEfektifBaru",
              skema: "skema",
            };
            const key = fieldMap[d.field];
            if (key) {
              form.setError(key, { message: d.message });
            }
          });
        }
        notify.error(err);
      } else {
        notify.error({
          code: "NETWORK_ERROR",
          message: "Gagal menghubungi server.",
          traceId: "",
        });
      }
      // Regenerate idempotency key for retry
      idempotencyKey.current = uuidv4();
    }
  };

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      {/* Form panel */}
      <div className="space-y-4">
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            {/* Instrumen ID */}
            <FormField
              control={form.control}
              name="instrumenId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    ID Instrumen Deposito <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder="UUID instrumen deposito ACTIVE"
                      aria-describedby="instrumen-hint"
                      disabled={!!instrumenId}
                    />
                  </FormControl>
                  <p id="instrumen-hint" className="text-xs text-muted-foreground">
                    Hanya instrumen DEPOSITO dengan status ACTIVE dan klasifikasi final.
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Skema */}
            <FormField
              control={form.control}
              name="skema"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Skema Renewal <span aria-hidden="true">*</span>
                  </FormLabel>
                  <Select
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger aria-label="Pilih skema renewal">
                        <SelectValue placeholder="Pilih skema" />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="POKOK_SAJA">
                        Pokok Saja — pokok baru = pokok lama
                      </SelectItem>
                      <SelectItem value="POKOK_PLUS_BUNGA">
                        Pokok + Bunga — pokok baru = pokok lama + bunga bersih
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Tenor */}
            <FormField
              control={form.control}
              name="tenorBaruBulan"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Tenor Baru (bulan) <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="number"
                      min={1}
                      max={60}
                      step={1}
                      placeholder="1–60"
                      onChange={(e) => field.onChange(parseInt(e.target.value, 10) || 0)}
                      aria-describedby="tenor-hint"
                    />
                  </FormControl>
                  <p id="tenor-hint" className="text-xs text-muted-foreground">
                    Range: 1–60 bulan (inklusif). Error RENEWAL_TENOR_OUT_OF_RANGE jika di luar.
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Rate */}
            <FormField
              control={form.control}
              name="rateBaruPersen"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Rate Baru (% p.a.) <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="number"
                      min={0}
                      max={30}
                      step={0.01}
                      placeholder="0–30"
                      onChange={(e) => field.onChange(parseFloat(e.target.value) || 0)}
                      aria-describedby="rate-hint"
                    />
                  </FormControl>
                  <p id="rate-hint" className="text-xs text-muted-foreground">
                    Range: 0%–30% p.a. (inklusif). Error RENEWAL_RATE_OUT_OF_RANGE jika di luar.
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Tanggal Efektif */}
            <FormField
              control={form.control}
              name="tanggalEfektifBaru"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Tanggal Efektif Baru <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <div className="relative">
                      <Input
                        {...field}
                        type="date"
                        aria-describedby="tanggal-hint"
                      />
                      <CalendarIcon
                        className="absolute right-3 top-2.5 h-4 w-4 text-muted-foreground pointer-events-none"
                        aria-hidden="true"
                      />
                    </div>
                  </FormControl>
                  <p id="tanggal-hint" className="text-xs text-muted-foreground">
                    Format YYYY-MM-DD. Periode buku untuk tanggal ini harus OPEN.
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="flex gap-2 pt-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => router.back()}
                disabled={isSubmitting}
              >
                Batal
              </Button>
              <Button type="submit" disabled={isSubmitting}>
                {isSubmitting ? "Memproses..." : "Buat Renewal"}
              </Button>
            </div>
          </form>
        </Form>
      </div>

      {/* Preview panel — shown after successful create */}
      <div>
        {preview ? (
          <RenewalPreviewPanel
            preview={preview.preview}
            showSchedule
          />
        ) : (
          <div className="rounded-lg border border-dashed bg-muted/20 p-8 text-center text-sm text-muted-foreground">
            <p>Preview kalkulasi akan ditampilkan setelah renewal berhasil dibuat oleh server.</p>
            <p className="mt-1 text-xs">
              Server menghitung: bunga_kotor, PPh 20%, bunga_bersih, pokok_baru, EIR (Newton-Raphson).
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

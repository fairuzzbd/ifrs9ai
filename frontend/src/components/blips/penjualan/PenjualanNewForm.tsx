"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import { useRouter } from "next/navigation";
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
import { penjualanCreateApi, penjualanQueryKeys } from "@/lib/api/penjualan.api";
import {
  createPenjualanSchema,
  type CreatePenjualanInput,
  type CreatePenjualanResponse,
  JENIS_DISPOSAL_LABELS,
} from "@/lib/schemas/penjualan.schema";
import { PenjualanPreviewPanel } from "./PenjualanPreviewPanel";
import { useQueryClient } from "@tanstack/react-query";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface PenjualanNewFormProps {
  instrumenId?: string;
  instrumenKode?: string;
  onSuccess?: (response: CreatePenjualanResponse) => void;
}

// ---------------------------------------------------------------------------
// Component (S1 — Maker creates penjualan; live preview after submit)
// ---------------------------------------------------------------------------

export function PenjualanNewForm({
  instrumenId,
  instrumenKode,
  onSuccess,
}: PenjualanNewFormProps) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const idempotencyKey = React.useRef(uuidv4());
  const [preview, setPreview] = React.useState<CreatePenjualanResponse | null>(null);

  const form = useForm<CreatePenjualanInput>({
    resolver: zodResolver(createPenjualanSchema),
    defaultValues: {
      instrumenId: instrumenId ?? "",
      jenisDisposal: "PARTIAL",
      qtyTerjual: "",
      hargaJualPerUnit: "",
      tanggalEksekusi: "",
    },
  });

  const isSubmitting = form.formState.isSubmitting;

  const onSubmit = async (data: CreatePenjualanInput) => {
    try {
      const result = await penjualanCreateApi.create(data, idempotencyKey.current);
      const resp = result.data;
      setPreview(resp);

      notify.success(
        `Penjualan ${instrumenKode ?? data.instrumenId.slice(0, 8)} (${data.qtyTerjual} unit) berhasil dibuat (${resp.penjualanId.slice(0, 8)}). Menunggu approval Treasury Approver.`,
        {
          action: {
            label: "Lihat Detail",
            onClick: () => router.push(`/transaksi/penjualan/${resp.penjualanId}`),
          },
        },
      );

      await queryClient.invalidateQueries({ queryKey: penjualanQueryKeys.lists() });

      // Rotate idempotency key after success
      idempotencyKey.current = uuidv4();
      onSuccess?.(resp);
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "VALIDATION_FAILED" && err.details.length > 0) {
          err.details.forEach((d) => {
            const fieldMap: Record<string, keyof CreatePenjualanInput> = {
              qtyTerjual: "qtyTerjual",
              hargaJualPerUnit: "hargaJualPerUnit",
              tanggalEksekusi: "tanggalEksekusi",
              instrumenId: "instrumenId",
            };
            const f = fieldMap[d.field];
            if (f) form.setError(f, { message: d.message });
          });
        }
        notify.error(err);
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal menghubungi server.", traceId: "" });
      }
    }
  };

  const jenisDisposalValue = form.watch("jenisDisposal");

  return (
    <div className="space-y-6">
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
          {/* Instrumen ID */}
          <FormField
            control={form.control}
            name="instrumenId"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Instrumen <span aria-hidden="true">*</span></FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    placeholder="UUID Instrumen (ACTIVE, klasifikasi locked)"
                    aria-describedby="instrumen-hint"
                    readOnly={!!instrumenId}
                    className={instrumenId ? "bg-muted" : ""}
                  />
                </FormControl>
                <p id="instrumen-hint" className="text-xs text-muted-foreground">
                  Hanya instrumen ACTIVE dengan klasifikasi PSAK 71 terkunci yang dapat dijual.
                </p>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Jenis Disposal */}
          <FormField
            control={form.control}
            name="jenisDisposal"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Jenis Disposal <span aria-hidden="true">*</span></FormLabel>
                <Select onValueChange={field.onChange} value={field.value}>
                  <FormControl>
                    <SelectTrigger aria-label="Pilih jenis disposal">
                      <SelectValue placeholder="Pilih jenis disposal" />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {Object.entries(JENIS_DISPOSAL_LABELS).map(([value, label]) => (
                      <SelectItem key={value} value={value}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  {jenisDisposalValue === "FULL"
                    ? "Seluruh qty holding dijual. Instrumen akan berstatus DISPOSED."
                    : "Sebagian qty holding dijual. Instrumen tetap ACTIVE."}
                </p>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Qty Terjual */}
          <FormField
            control={form.control}
            name="qtyTerjual"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Qty Terjual <span aria-hidden="true">*</span></FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type="text"
                    inputMode="decimal"
                    placeholder="0.00000000"
                    aria-describedby="qty-hint"
                  />
                </FormControl>
                <p id="qty-hint" className="text-xs text-muted-foreground">
                  Qty harus positif dan tidak melebihi qty holding saat ini.
                </p>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Harga Jual Per Unit */}
          <FormField
            control={form.control}
            name="hargaJualPerUnit"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Harga Jual Per Unit (IDR) <span aria-hidden="true">*</span></FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type="text"
                    inputMode="decimal"
                    placeholder="0.0000"
                    aria-describedby="harga-hint"
                  />
                </FormControl>
                <p id="harga-hint" className="text-xs text-muted-foreground">
                  NUMERIC(20,4). Harus lebih dari 0.
                </p>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Tanggal Eksekusi */}
          <FormField
            control={form.control}
            name="tanggalEksekusi"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Tanggal Eksekusi <span aria-hidden="true">*</span></FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type="date"
                    aria-describedby="tanggal-hint"
                  />
                </FormControl>
                <p id="tanggal-hint" className="text-xs text-muted-foreground">
                  Harus dalam periode buku yang OPEN.
                </p>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button type="submit" disabled={isSubmitting} className="w-full sm:w-auto">
            {isSubmitting ? "Memproses..." : "Buat Penjualan"}
          </Button>
        </form>
      </Form>

      {/* Preview panel shown after successful create */}
      {preview && (
        <PenjualanPreviewPanel
          preview={preview.preview}
          jenisDisposal={form.getValues("jenisDisposal")}
        />
      )}
    </div>
  );
}

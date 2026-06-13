"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { v4 as uuidv4 } from "uuid";
import { Loader2 } from "lucide-react";

import {
  createCalcRunSchema,
  type CreateCalcRunForm,
} from "@/lib/schemas/calc-run.schema";
import { calcRunApi } from "@/lib/api/calc-run.api";
import { notify } from "@/lib/notify";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { apiGet } from "@/lib/api";
import type { ListResponse } from "@/lib/api";

// ---------------------------------------------------------------------------
// Periode Buku type (minimal)
// ---------------------------------------------------------------------------

interface PeriodeBuku {
  id: string;
  label: string;
  status: string;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface CreateCalcRunModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function CreateCalcRunModal({ open, onOpenChange }: CreateCalcRunModalProps) {
  const router = useRouter();

  const today = new Date().toISOString().split("T")[0];

  const form = useForm<CreateCalcRunForm>({
    resolver: zodResolver(createCalcRunSchema),
    defaultValues: {
      periodeId: "",
      evaluationDate: today ?? "",
    },
  });

  // Fetch available periods (exclude HARD_CLOSED)
  const { data: periodeData } = useQuery({
    queryKey: ["periode-buku-available"],
    queryFn: () =>
      apiGet<ListResponse<PeriodeBuku>>(
        "/api/v1/master/periode-buku?filter[status][]=OPEN&filter[status][]=SOFT_CLOSED&limit=200",
      ),
    enabled: open,
  });

  const periodeList = periodeData?.data ?? [];

  const mutation = useMutation({
    mutationFn: (data: CreateCalcRunForm) =>
      calcRunApi.create({ periodeId: data.periodeId, evaluationDate: data.evaluationDate }, uuidv4()),
    onSuccess: (res) => {
      const run = res.data;
      notify.success(
        `Calc run untuk periode ${run.periodeLabel ?? run.periodeId} berhasil dibuat (${run.id}). Status: DRAFT.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/ecl/calc-runs/${run.id}`),
          },
        },
      );
      onOpenChange(false);
      form.reset({ periodeId: "", evaluationDate: today ?? "" });
      router.push(`/ecl/calc-runs/${run.id}`);
    },
    onError: (err) => {
      notify.error(err as Parameters<typeof notify.error>[0]);
      // Keep modal open on error — user can pick different periode
    },
  });

  const onSubmit = (data: CreateCalcRunForm) => mutation.mutate(data);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Buat Calc Run Baru</DialogTitle>
          <DialogDescription>
            Pilih periode buku dan tanggal evaluasi untuk memulai perhitungan ECL.
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            {/* Periode picker */}
            <FormField
              control={form.control}
              name="periodeId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Periode Buku *</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value}>
                    <FormControl>
                      <SelectTrigger aria-describedby="periode-error">
                        <SelectValue placeholder="Pilih periode..." />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {periodeList.map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.label} ({p.status})
                        </SelectItem>
                      ))}
                      {periodeList.length === 0 && (
                        <SelectItem value="_empty" disabled>
                          Tidak ada periode tersedia
                        </SelectItem>
                      )}
                    </SelectContent>
                  </Select>
                  <FormMessage id="periode-error" />
                </FormItem>
              )}
            />

            {/* Evaluation date */}
            <FormField
              control={form.control}
              name="evaluationDate"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Tanggal Evaluasi *</FormLabel>
                  <FormControl>
                    <Input
                      type="date"
                      aria-describedby="eval-date-error"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage id="eval-date-error" />
                </FormItem>
              )}
            />

            <div className="flex justify-end gap-2 pt-2 border-t">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={mutation.isPending}
              >
                Batal
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending && (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                )}
                Buat
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

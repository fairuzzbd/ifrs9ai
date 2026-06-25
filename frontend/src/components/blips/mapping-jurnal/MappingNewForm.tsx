/**
 * MappingNewForm — RHF+Zod form for creating new version of a mapping.
 * Used in: /mapping-jurnal/[event_code] (edit APPROVED_ACTIVE → new version)
 * Fields: reason + detail rows array (akun_debit, akun_kredit, D/K, jumlah_calc, urutan)
 */

"use client";

import * as React from "react";
import { useForm, useFieldArray } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Plus, Trash2 } from "lucide-react";
import { v4 as uuidv4 } from "uuid";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { newVersionFormSchema, type NewVersionFormInput } from "@/lib/schemas/mapping-jurnal-p12.schema";

interface MappingNewFormProps {
  eventCode: string;
  defaultDetail?: NewVersionFormInput["detail"];
  onSubmit: (values: NewVersionFormInput) => Promise<void>;
  submitting?: boolean;
  onCancel?: () => void;
}

export function MappingNewForm({
  eventCode,
  defaultDetail,
  onSubmit,
  submitting = false,
  onCancel,
}: MappingNewFormProps) {
  const form = useForm<NewVersionFormInput>({
    resolver: zodResolver(newVersionFormSchema),
    defaultValues: {
      reason: "",
      detail: defaultDetail ?? [
        { _clientKey: uuidv4(), akunDebit: "", akunKredit: "", debitKredit: "D", jumlahCalc: "", urutan: 1 },
        { _clientKey: uuidv4(), akunDebit: "", akunKredit: "", debitKredit: "K", jumlahCalc: "", urutan: 2 },
      ],
    },
  });

  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: "detail",
    keyName: "_clientKey",
  });

  const handleAddRow = () => {
    const nextUrutan = (fields[fields.length - 1]?.urutan ?? 0) + 1;
    append({
      _clientKey: uuidv4(),
      akunDebit: "",
      akunKredit: "",
      debitKredit: "D",
      jumlahCalc: "",
      urutan: nextUrutan,
    });
  };

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
        <FormField
          control={form.control}
          name="reason"
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                Alasan Perubahan <span className="text-destructive">*</span>
              </FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  rows={2}
                  placeholder="Alasan pembuatan versi baru mapping..."
                  disabled={submitting}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold">
              Baris Detail Mapping ({eventCode})
            </h3>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={handleAddRow}
              disabled={submitting}
              aria-label="Tambah baris detail"
            >
              <Plus className="mr-1 h-4 w-4" aria-hidden="true" />
              Tambah Baris
            </Button>
          </div>

          {form.formState.errors.detail?.root && (
            <p role="alert" className="text-xs text-destructive">
              {form.formState.errors.detail.root.message}
            </p>
          )}
          {form.formState.errors.detail?.message && (
            <p role="alert" className="text-xs text-destructive">
              {form.formState.errors.detail.message}
            </p>
          )}

          <div className="rounded-lg border overflow-x-auto">
            <table className="w-full text-sm" aria-label="Form baris detail mapping">
              <thead className="border-b bg-muted/50">
                <tr>
                  <th scope="col" className="px-2 py-2 text-left text-xs font-medium text-muted-foreground w-12">No.</th>
                  <th scope="col" className="px-2 py-2 text-left text-xs font-medium text-muted-foreground">Akun Debit</th>
                  <th scope="col" className="px-2 py-2 text-left text-xs font-medium text-muted-foreground">Akun Kredit</th>
                  <th scope="col" className="px-2 py-2 text-left text-xs font-medium text-muted-foreground w-24">D/K</th>
                  <th scope="col" className="px-2 py-2 text-left text-xs font-medium text-muted-foreground">Formula</th>
                  <th scope="col" className="px-2 py-2 w-10" aria-label="Aksi"></th>
                </tr>
              </thead>
              <tbody>
                {fields.map((field, idx) => (
                  <tr key={field._clientKey} className="border-b last:border-0">
                    <td className="px-2 py-1.5">
                      <FormField
                        control={form.control}
                        name={`detail.${idx}.urutan`}
                        render={({ field: f }) => (
                          <FormItem className="space-y-0">
                            <FormControl>
                              <Input
                                {...f}
                                type="number"
                                min={1}
                                className="h-7 w-12 text-xs"
                                disabled={submitting}
                                aria-label={`Urutan baris ${idx + 1}`}
                                onChange={(e) => f.onChange(Number(e.target.value))}
                              />
                            </FormControl>
                            <FormMessage className="text-xs" />
                          </FormItem>
                        )}
                      />
                    </td>
                    <td className="px-2 py-1.5">
                      <FormField
                        control={form.control}
                        name={`detail.${idx}.akunDebit`}
                        render={({ field: f }) => (
                          <FormItem className="space-y-0">
                            <FormControl>
                              <Input
                                {...f}
                                className="h-7 text-xs font-mono"
                                placeholder="110201"
                                disabled={submitting}
                                aria-label={`Akun debit baris ${idx + 1}`}
                              />
                            </FormControl>
                            <FormMessage className="text-xs" />
                          </FormItem>
                        )}
                      />
                    </td>
                    <td className="px-2 py-1.5">
                      <FormField
                        control={form.control}
                        name={`detail.${idx}.akunKredit`}
                        render={({ field: f }) => (
                          <FormItem className="space-y-0">
                            <FormControl>
                              <Input
                                {...f}
                                className="h-7 text-xs font-mono"
                                placeholder="440101"
                                disabled={submitting}
                                aria-label={`Akun kredit baris ${idx + 1}`}
                              />
                            </FormControl>
                            <FormMessage className="text-xs" />
                          </FormItem>
                        )}
                      />
                    </td>
                    <td className="px-2 py-1.5">
                      <FormField
                        control={form.control}
                        name={`detail.${idx}.debitKredit`}
                        render={({ field: f }) => (
                          <FormItem className="space-y-0">
                            <Select
                              value={f.value}
                              onValueChange={f.onChange}
                              disabled={submitting}
                            >
                              <FormControl>
                                <SelectTrigger
                                  className="h-7 text-xs w-20"
                                  aria-label={`D/K baris ${idx + 1}`}
                                >
                                  <SelectValue />
                                </SelectTrigger>
                              </FormControl>
                              <SelectContent>
                                <SelectItem value="D" className={cn("text-xs text-blue-700")}>D (Debit)</SelectItem>
                                <SelectItem value="K" className={cn("text-xs text-orange-700")}>K (Kredit)</SelectItem>
                              </SelectContent>
                            </Select>
                            <FormMessage className="text-xs" />
                          </FormItem>
                        )}
                      />
                    </td>
                    <td className="px-2 py-1.5">
                      <FormField
                        control={form.control}
                        name={`detail.${idx}.jumlahCalc`}
                        render={({ field: f }) => (
                          <FormItem className="space-y-0">
                            <FormControl>
                              <Input
                                {...f}
                                value={f.value ?? ""}
                                className="h-7 text-xs"
                                placeholder="ECL_weighted"
                                disabled={submitting}
                                aria-label={`Formula baris ${idx + 1}`}
                              />
                            </FormControl>
                            <FormMessage className="text-xs" />
                          </FormItem>
                        )}
                      />
                    </td>
                    <td className="px-2 py-1.5 text-center">
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 text-muted-foreground hover:text-destructive"
                        onClick={() => remove(idx)}
                        disabled={submitting || fields.length <= 1}
                        aria-label={`Hapus baris ${idx + 1}`}
                      >
                        <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="flex gap-3">
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel} disabled={submitting}>
              Batal
            </Button>
          )}
          <Button type="submit" disabled={submitting} aria-busy={submitting}>
            {submitting ? "Menyimpan..." : "Simpan Versi Baru"}
          </Button>
        </div>
      </form>
    </Form>
  );
}

/**
 * ScheduledEmailForm — RHF+Zod form for creating/editing scheduled email config.
 * ROLE-AKUN-CTL only (caller enforces persona gating — absent from DOM if not).
 * Fields: reportSlug, format, frequency, sendTime, recipients, active,
 *         subjectTemplate, bodyTemplate.
 */

"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  scheduledEmailFormSchema,
  type ScheduledEmailFormInput,
  REPORT_SLUG_LABELS,
  type ReportSlug,
} from "@/lib/schemas/reporting.schema";

interface ScheduledEmailFormProps {
  defaultValues?: Partial<ScheduledEmailFormInput>;
  onSubmit: (values: ScheduledEmailFormInput) => Promise<void>;
  submitting?: boolean;
  onCancel?: () => void;
}

const REPORT_SLUGS = Object.keys(REPORT_SLUG_LABELS) as ReportSlug[];

export function ScheduledEmailForm({
  defaultValues,
  onSubmit,
  submitting = false,
  onCancel,
}: ScheduledEmailFormProps) {
  const form = useForm<ScheduledEmailFormInput>({
    resolver: zodResolver(scheduledEmailFormSchema),
    defaultValues: {
      reportSlug: "mv-jurnal-summary",
      format: "xlsx",
      frequency: "daily",
      sendTime: "07:00+07:00",
      recipients: "",
      active: true,
      subjectTemplate: "Laporan {report_slug} BLIPS — {tanggal}",
      bodyTemplate:
        "Terlampir laporan {report_slug} BLIPS per {tanggal}.\n\nFile dapat diverifikasi dengan SHA-256: {file_hash}.\n\nUntuk opt-out, klik: {opt_out_link}",
      ...defaultValues,
    },
  });

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
        {/* Report slug */}
        <FormField
          control={form.control}
          name="reportSlug"
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                Laporan <span className="text-destructive">*</span>
              </FormLabel>
              <Select
                value={field.value}
                onValueChange={field.onChange}
                disabled={submitting}
              >
                <FormControl>
                  <SelectTrigger aria-label="Pilih laporan">
                    <SelectValue placeholder="Pilih laporan..." />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  {REPORT_SLUGS.map((slug) => (
                    <SelectItem key={slug} value={slug}>
                      {REPORT_SLUG_LABELS[slug]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />

        <div className="grid grid-cols-2 gap-4">
          {/* Format */}
          <FormField
            control={form.control}
            name="format"
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  Format <span className="text-destructive">*</span>
                </FormLabel>
                <Select value={field.value} onValueChange={field.onChange} disabled={submitting}>
                  <FormControl>
                    <SelectTrigger aria-label="Pilih format export">
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value="xlsx">XLSX (Excel)</SelectItem>
                    <SelectItem value="csv">CSV</SelectItem>
                    <SelectItem value="pdf">PDF</SelectItem>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Frequency */}
          <FormField
            control={form.control}
            name="frequency"
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  Frekuensi <span className="text-destructive">*</span>
                </FormLabel>
                <Select value={field.value} onValueChange={field.onChange} disabled={submitting}>
                  <FormControl>
                    <SelectTrigger aria-label="Pilih frekuensi pengiriman">
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value="daily">Harian</SelectItem>
                    <SelectItem value="weekly">Mingguan</SelectItem>
                    <SelectItem value="monthly">Bulanan</SelectItem>
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        {/* Send time */}
        <FormField
          control={form.control}
          name="sendTime"
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                Waktu Kirim (WIB) <span className="text-destructive">*</span>
              </FormLabel>
              <FormControl>
                <Input
                  {...field}
                  placeholder="07:00+07:00"
                  disabled={submitting}
                  aria-describedby="sendTime-desc"
                />
              </FormControl>
              <FormDescription id="sendTime-desc">
                Format: HH:MM+07:00 — contoh: 07:00+07:00 (pukul 07.00 WIB)
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Recipients */}
        <FormField
          control={form.control}
          name="recipients"
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                Penerima Email <span className="text-destructive">*</span>
              </FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  rows={3}
                  placeholder="cfo@tugu-re.com, risk@tugu-re.com, akun@tugu-re.com"
                  disabled={submitting}
                  aria-describedby="recipients-desc"
                />
              </FormControl>
              <FormDescription id="recipients-desc">
                Pisahkan beberapa alamat email dengan koma. Maks 50 penerima.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Active toggle */}
        <FormField
          control={form.control}
          name="active"
          render={({ field }) => (
            <FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
              <div className="space-y-0.5">
                <FormLabel>Aktif</FormLabel>
                <FormDescription>
                  Jika dimatikan, jadwal email tidak akan dikirim sampai diaktifkan kembali.
                </FormDescription>
              </div>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  disabled={submitting}
                  aria-label="Aktifkan jadwal email"
                />
              </FormControl>
            </FormItem>
          )}
        />

        {/* Subject template */}
        <FormField
          control={form.control}
          name="subjectTemplate"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Template Subjek Email</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  value={field.value ?? ""}
                  placeholder="Laporan {report_slug} BLIPS — {tanggal}"
                  disabled={submitting}
                />
              </FormControl>
              <FormDescription>
                Placeholder: {"{tanggal}"}, {"{report_slug}"}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Body template */}
        <FormField
          control={form.control}
          name="bodyTemplate"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Template Isi Email</FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  value={field.value ?? ""}
                  rows={4}
                  placeholder="Terlampir laporan..."
                  disabled={submitting}
                />
              </FormControl>
              <FormDescription>
                Placeholder: {"{tanggal}"}, {"{file_hash}"}, {"{report_slug}"},{" "}
                {"{opt_out_link}"}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <div className="flex gap-3 pt-1">
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel} disabled={submitting}>
              Batal
            </Button>
          )}
          <Button type="submit" disabled={submitting} aria-busy={submitting}>
            {submitting ? "Menyimpan..." : "Simpan Jadwal Email"}
          </Button>
        </div>
      </form>
    </Form>
  );
}

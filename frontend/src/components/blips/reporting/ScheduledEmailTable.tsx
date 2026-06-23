/**
 * ScheduledEmailTable — DataTable list of scheduled email configs.
 * Supports soft-delete (DestructiveActionDialog) + filter by active/inactive.
 * Persona gating: delete button absent from DOM for non-AKUN-CTL.
 */

"use client";

import * as React from "react";
import { Trash2, Mail } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ExportFormatBadge } from "./ExportFormatBadge";
import type { ScheduledEmailItem } from "@/lib/schemas/reporting.schema";
import { REPORT_SLUG_LABELS } from "@/lib/schemas/reporting.schema";
import { formatInTimeZone } from "date-fns-tz";

interface ScheduledEmailTableProps {
  items: ScheduledEmailItem[];
  isAkunCtl: boolean;
  onDelete: (id: string, reportSlug: string) => void;
  loading?: boolean;
}

function formatSentAt(iso: string | null): string {
  if (!iso) return "—";
  return formatInTimeZone(new Date(iso), "Asia/Jakarta", "dd MMM yyyy HH:mm");
}

export function ScheduledEmailTable({
  items,
  isAkunCtl,
  onDelete,
  loading = false,
}: ScheduledEmailTableProps) {
  if (loading) {
    return (
      <div className="rounded-lg border p-6 text-center text-sm text-muted-foreground">
        Memuat daftar jadwal email...
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="rounded-lg border p-8 text-center">
        <Mail className="mx-auto mb-2 h-8 w-8 text-muted-foreground/40" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">
          Belum ada jadwal email terkonfigurasi.
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-lg border overflow-x-auto">
      <Table aria-label="Daftar jadwal email terjadwal">
        <TableHeader className="bg-muted/40">
          <TableRow>
            <TableHead className="w-48">Laporan</TableHead>
            <TableHead className="w-20">Format</TableHead>
            <TableHead className="w-24">Frekuensi</TableHead>
            <TableHead className="w-28">Waktu Kirim</TableHead>
            <TableHead>Penerima</TableHead>
            <TableHead className="w-20">Status</TableHead>
            <TableHead className="w-36">Terakhir Kirim</TableHead>
            {isAkunCtl && <TableHead className="w-16 text-center" aria-label="Aksi" />}
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((item) => (
            <TableRow key={item.id}>
              <TableCell className="font-medium text-sm">
                {REPORT_SLUG_LABELS[item.reportSlug as keyof typeof REPORT_SLUG_LABELS] ??
                  item.reportSlug}
              </TableCell>
              <TableCell>
                <ExportFormatBadge format={item.format} />
              </TableCell>
              <TableCell className="text-sm capitalize">{item.frequency}</TableCell>
              <TableCell className="font-mono text-xs">{item.sendTime}</TableCell>
              <TableCell className="text-xs">
                <span>{item.recipients.slice(0, 2).join(", ")}</span>
                {item.recipients.length > 2 && (
                  <span className="text-muted-foreground">
                    {" "}+ {item.recipients.length - 2} lainnya
                  </span>
                )}
                {item.optOutCount > 0 && (
                  <span className="ml-1 text-amber-600 text-xs">
                    ({item.optOutCount} opt-out)
                  </span>
                )}
              </TableCell>
              <TableCell>
                <Badge variant={item.active ? "default" : "secondary"} className="text-xs">
                  {item.active ? "Aktif" : "Nonaktif"}
                </Badge>
              </TableCell>
              <TableCell className="text-xs text-muted-foreground">
                {formatSentAt(item.lastSentAt)}
                {item.lastStatus === "FAILED" && (
                  <span className="ml-1 text-red-600 font-medium">Gagal</span>
                )}
              </TableCell>
              {isAkunCtl && (
                <TableCell className="text-center">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 text-muted-foreground hover:text-destructive"
                    onClick={() => onDelete(item.id, item.reportSlug)}
                    aria-label={`Hapus jadwal email untuk ${item.reportSlug}`}
                  >
                    <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                  </Button>
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

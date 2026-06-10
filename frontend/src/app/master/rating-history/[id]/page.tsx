"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { AlertTriangle, MoreHorizontal, Pencil } from "lucide-react";
import { v4 as uuidv4 } from "uuid";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

import { ratingHistoryApi } from "@/lib/api/rating-history.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  ACTION_TYPE_LABELS,
  RATING_OUTLOOK_LABELS,
} from "@/lib/schemas/rating-history.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">{label}</span>
      <span className="text-sm font-medium">{value ?? "—"}</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// SICR / DEFAULT badge
// ---------------------------------------------------------------------------

function TriggerBadge({ type }: { type: "SICR" | "DEFAULT" }) {
  const isDefault = type === "DEFAULT";
  return (
    <span className={cn(
      "inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium border",
      isDefault
        ? "bg-orange-100 text-orange-700 border-orange-300"
        : "bg-red-100 text-red-700 border-red-300",
    )}>
      <AlertTriangle className="h-3 w-3" aria-hidden />
      {type}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function RatingHistoryDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const perms = usePermissions();

  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["rating-history", id],
    queryFn: () => ratingHistoryApi.get(id),
    enabled: !!id,
  });

  const item = data?.data;

  const handleDelete = async () => {
    if (!item) return;
    setDeleting(true);
    try {
      await ratingHistoryApi.delete(id, uuidv4());
      notify.destructive(`Rating ${item.ratingPefindo} untuk ${item.counterpartyNama} berhasil dihapus.`);
      router.push(`/master/counterparty/${item.counterpartyId}/rating-history`);
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally { setDeleting(false); setDeleteOpen(false); }
  };

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="space-y-4">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}</div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">Gagal memuat data rating history.</p>
        <Button variant="outline" asChild><Link href="/master/rating-history">Kembali ke Daftar</Link></Button>
      </div>
    );
  }

  const canEdit = perms.canUpdate("rating_history");
  const canDelete = perms.canDelete("rating_history");

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/rating-history" className="hover:underline">Rating History</Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/counterparty/${item.counterpartyId}`} className="hover:underline">{item.counterpartyKode}</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">{item.ratingPefindo} — {item.tanggalBerlaku}</span>
      </nav>

      {/* Title */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">
            Rating: <span className="font-mono">{item.ratingPefindo}</span>
          </h1>
          {item.sicrTriggered && <TriggerBadge type="SICR" />}
          {item.defaultTriggered && <TriggerBadge type="DEFAULT" />}
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/rating-history/${id}/edit`}>
                <Pencil className="mr-1.5 h-4 w-4" aria-hidden /> Edit
              </Link>
            </Button>
          )}
          {canDelete && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="icon" className="h-9 w-9" aria-label="Aksi lainnya">
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/rating-history/${id}/history`}>Riwayat Audit</Link>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem className="text-destructive" onClick={() => setDeleteOpen(true)}>
                  Hapus
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </div>

      {/* SICR alert */}
      {item.sicrTriggered && (
        <div className="flex items-start gap-3 rounded-lg border border-amber-400 bg-amber-50 px-4 py-3">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" aria-hidden />
          <p className="text-sm text-amber-800">
            <strong>SICR Terdeteksi</strong> — Rating ini memicu Significant Increase in Credit Risk.
            Counterparty <strong>{item.counterpartyNama}</strong> dipindah ke Stage 2 ECL
            per tanggal <strong>{item.tanggalBerlaku}</strong>.
          </p>
        </div>
      )}

      {/* Detail */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
            Detail Rating
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <DetailRow
              label="Counterparty"
              value={
                <Link href={`/master/counterparty/${item.counterpartyId}`} className="hover:underline text-primary">
                  {item.counterpartyNama}
                </Link>
              }
            />
            <DetailRow label="Rating Pefindo" value={<span className="font-mono font-bold text-base">{item.ratingPefindo}</span>} />
            <DetailRow label="Outlook" value={RATING_OUTLOOK_LABELS[item.ratingOutlook] ?? item.ratingOutlook} />
            <DetailRow
              label="Action Type"
              value={
                <span className={cn(
                  "font-medium",
                  item.actionType === "UPGRADE" ? "text-green-700" :
                  item.actionType === "DOWNGRADE" ? "text-red-700" : undefined,
                )}>
                  {ACTION_TYPE_LABELS[item.actionType] ?? item.actionType}
                </span>
              }
            />
            <DetailRow
              label="Notch Change"
              value={
                <span className={cn(
                  "font-mono",
                  item.notchChange > 0 ? "text-green-700" : item.notchChange < 0 ? "text-red-700" : "text-muted-foreground",
                )}>
                  {item.notchChange > 0 ? `+${item.notchChange}` : item.notchChange}
                </span>
              }
            />
            <DetailRow label="Tanggal Berlaku" value={item.tanggalBerlaku} />
            <DetailRow label="Tanggal Berakhir" value={item.tanggalBerakhir ?? "—"} />
            <DetailRow label="Sumber" value={item.sumber ?? "—"} />
            <DetailRow
              label="SICR Triggered"
              value={
                <span className={item.sicrTriggered ? "text-red-700 font-medium" : "text-muted-foreground"}>
                  {item.sicrTriggered ? "Ya" : "Tidak"}
                </span>
              }
            />
            <DetailRow
              label="Default Triggered"
              value={
                <span className={item.defaultTriggered ? "text-orange-700 font-medium" : "text-muted-foreground"}>
                  {item.defaultTriggered ? "Ya" : "Tidak"}
                </span>
              }
            />
          </div>
          {item.catatan && (
            <div className="mt-4 rounded-md border bg-muted/30 p-3">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-1">Catatan</p>
              <p className="text-sm">{item.catatan}</p>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Metadata */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">Metadata</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4">
            <DetailRow label="Dibuat oleh" value={item.createdBy} />
            <DetailRow label="Dibuat pada" value={item.createdAt} />
            <DetailRow label="Diperbarui oleh" value={item.updatedBy} />
            <DetailRow label="Diperbarui pada" value={item.updatedAt} />
            <DetailRow label="Versi" value={item.rowVersion} />
          </div>
        </CardContent>
      </Card>

      <div className="flex gap-4 text-sm">
        <Link href={`/master/counterparty/${item.counterpartyId}/rating-history`} className="text-primary hover:underline">
          Semua rating counterparty ini &rarr;
        </Link>
        <Link href={`/master/rating-history/${id}/history`} className="text-primary hover:underline">
          Riwayat Audit &rarr;
        </Link>
      </div>

      {/* Delete dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hapus Rating History?</DialogTitle>
            <DialogDescription>
              Rating <strong>{item.ratingPefindo}</strong> untuk <strong>{item.counterpartyNama}</strong>{" "}
              ({item.tanggalBerlaku}) akan dihapus (soft-delete).
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={deleting}>Batal</Button>
            <Button variant="destructive" onClick={() => void handleDelete()} disabled={deleting}>
              {deleting ? "Menghapus..." : "Hapus"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

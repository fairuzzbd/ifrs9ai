"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { format, parseISO } from "date-fns";
import {
  Pencil,
  MoreHorizontal,
  AlertTriangle,
  CheckCircle2,
  XCircle,
} from "lucide-react";
import { v4 as uuidv4 } from "uuid";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { MakerReviewerApproverPanel } from "@/components/blips/MakerReviewerApproverPanel";

import { bobotSkenarioApi } from "@/lib/api/bobot-skenario.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  SKENARIO_ECL_LABELS,
  bobotDecimalToPercent,
  groupIntoTrios,
  isSumValid,
  type BobotSkenarioDetail,
  type SkenarioEcl,
} from "@/lib/schemas/bobot-skenario.schema";

// ---------------------------------------------------------------------------
// Detail field row
// ---------------------------------------------------------------------------

function DetailRow({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
        {label}
      </span>
      <span className="text-sm font-medium">{value ?? "—"}</span>
    </div>
  );
}

function formatDt(iso: string | null | undefined): string {
  if (!iso) return "—";
  try {
    return format(parseISO(iso), "dd MMM yyyy, HH:mm 'WIB'");
  } catch {
    return iso;
  }
}

function fmtDate(val: string | null | undefined): string {
  if (!val) return "—";
  try {
    return format(parseISO(val), "dd MMM yyyy");
  } catch {
    return val;
  }
}

// ---------------------------------------------------------------------------
// Trio summary panel (shows sibling G/N/B for the same period)
// ---------------------------------------------------------------------------

interface TrioPanelProps {
  currentItem: BobotSkenarioDetail;
}

function TrioPanel({ currentItem }: TrioPanelProps) {
  const { data: listData } = useQuery({
    queryKey: ["bobot-skenario", { limit: 200, sort: "periode_berlaku_dari:desc" }],
    queryFn: () =>
      bobotSkenarioApi.list({
        limit: 200,
        sort: "periode_berlaku_dari:desc",
        "filter[periode_berlaku_dari]": currentItem.periodeBerlakuDari,
      }),
    staleTime: 30_000,
  });

  const trios = React.useMemo(() => {
    const items = listData?.data ?? [];
    // Include current item in case it's not yet in the list response
    const allItems = items.some((i) => i.id === currentItem.id)
      ? items
      : [currentItem, ...items];
    return groupIntoTrios(allItems);
  }, [listData, currentItem]);

  const trio = trios.find(
    (t) => t.periodeBerlakuDari === currentItem.periodeBerlakuDari,
  );

  if (!trio) return null;

  const sumPct = trio.sum
    ? (parseFloat(trio.sum) * 100).toFixed(2)
    : null;

  const skenarios: SkenarioEcl[] = ["GOOD", "NORMAL", "BAD"];
  const skenarioColors: Record<SkenarioEcl, string> = {
    GOOD: "bg-green-50 border-green-200 text-green-800",
    NORMAL: "bg-blue-50 border-blue-200 text-blue-800",
    BAD: "bg-red-50 border-red-200 text-red-800",
  };

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
          Ringkasan Trio Periode {currentItem.periodeBerlakuDari}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {/* Sum indicator */}
        <div className="flex items-center gap-2">
          {!trio.isComplete ? (
            <>
              <AlertTriangle className="h-4 w-4 text-amber-600 shrink-0" aria-hidden />
              <span className="text-sm text-amber-700 font-medium">
                Trio belum lengkap — {!trio.good && "Good "}
                {!trio.normal && "Normal "}
                {!trio.bad && "Bad "}
                belum ada
              </span>
            </>
          ) : trio.isValid ? (
            <>
              <CheckCircle2 className="h-4 w-4 text-green-600 shrink-0" aria-hidden />
              <span className="text-sm text-green-700 font-medium">
                Sum{" "}
                <span className="font-mono">{sumPct}%</span> — Valid (DEC-010)
              </span>
            </>
          ) : (
            <>
              <XCircle className="h-4 w-4 text-red-600 shrink-0" aria-hidden />
              <span className="text-sm text-red-700 font-medium">
                Sum <span className="font-mono">{sumPct}%</span> — INVALID (harus 100%)
              </span>
            </>
          )}
        </div>

        <Separator />

        {/* 3 skenario cards side by side */}
        <div className="grid grid-cols-3 gap-2">
          {skenarios.map((sk) => {
            const skItem = trio[sk.toLowerCase() as "good" | "normal" | "bad"];
            const isCurrentItem = skItem?.id === currentItem.id;
            return (
              <div
                key={sk}
                className={`rounded-md border p-2 ${skenarioColors[sk]} ${isCurrentItem ? "ring-2 ring-primary ring-offset-1" : ""}`}
                aria-current={isCurrentItem ? "page" : undefined}
              >
                <div className="text-xs font-semibold uppercase tracking-wide mb-1">
                  {sk}
                </div>
                {skItem ? (
                  <>
                    <div className="font-mono text-lg font-bold">
                      {bobotDecimalToPercent(skItem.bobot)}%
                    </div>
                    <div className="mt-1">
                      <WorkflowStatusBadge status={skItem.workflowStatus} />
                    </div>
                    {!isCurrentItem && (
                      <Link
                        href={`/master/bobot-skenario/${skItem.id}`}
                        className="mt-1 block text-xs underline hover:no-underline"
                      >
                        Lihat detail
                      </Link>
                    )}
                  </>
                ) : (
                  <div className="text-xs text-muted-foreground">—</div>
                )}
              </div>
            );
          })}
        </div>

        {/* Balance hint */}
        {!trio.isValid && trio.isComplete && (
          <p className="text-xs text-muted-foreground">
            Total bobot ketiga skenario harus tepat 1.0 (100%) untuk memenuhi
            constraint DEC-010. Edit salah satu bobot untuk menyesuaikan.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function BobotSkenarioDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const perms = usePermissions();

  const [workflowSubmitting, setWorkflowSubmitting] = React.useState(false);
  const [deleteOpen, setDeleteOpen] = React.useState(false);
  const [deleting, setDeleting] = React.useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ["bobot-skenario", id],
    queryFn: () => bobotSkenarioApi.get(id),
    enabled: !!id,
  });

  const item: BobotSkenarioDetail | undefined = data?.data;

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["bobot-skenario", id] });
    void queryClient.invalidateQueries({ queryKey: ["bobot-skenario"] });
  };

  // ---------------------------------------------------------------------------
  // Workflow actions
  // ---------------------------------------------------------------------------

  const handleSubmit = async () => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await bobotSkenarioApi.submit(id, { rowVersion: item.rowVersion }, uuidv4());
      notify.success(
        `Bobot skenario ${SKENARIO_ECL_LABELS[item.skenario]} berhasil disubmit untuk review. Menunggu Risk Officer.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleReview = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      const res = await bobotSkenarioApi.review(
        id,
        { comment, signatureMethod: "JWT_STANDARD", rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Bobot skenario ${SKENARIO_ECL_LABELS[item.skenario]} berhasil di-review. Status: ${res.data.currentState}.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleApprove = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      const res = await bobotSkenarioApi.approve(
        id,
        {
          comment,
          // DEC-027: approve step requires JWT_STEP_UP MFA
          signatureMethod: "JWT_STEP_UP",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.success(
        `Bobot skenario ${SKENARIO_ECL_LABELS[item.skenario]} disetujui (Approval 1 — ALCO). Status: ${res.data.currentState}.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleApprove2 = async (comment: string | undefined) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      const res = await bobotSkenarioApi.approve2(
        id,
        {
          comment,
          // DEC-027: approve2 also requires JWT_STEP_UP MFA
          signatureMethod: "JWT_STEP_UP",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.success(
        `Bobot skenario ${SKENARIO_ECL_LABELS[item.skenario]} disetujui final (Approval 2 — CFO/Komite). Parameter ECL sekarang aktif.`,
        {
          action: {
            label: "Lihat riwayat",
            onClick: () => router.push(`/master/bobot-skenario/${id}/history`),
          },
        },
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleReject = async (comment: string) => {
    if (!item) return;
    setWorkflowSubmitting(true);
    try {
      await bobotSkenarioApi.reject(
        id,
        {
          comment,
          signatureMethod: "JWT_STANDARD",
          rowVersion: item.rowVersion,
        },
        uuidv4(),
      );
      notify.warning(
        `Bobot skenario ${SKENARIO_ECL_LABELS[item.skenario]} dikembalikan ke maker.`,
      );
      invalidate();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setWorkflowSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!item) return;
    setDeleting(true);
    try {
      await bobotSkenarioApi.softDelete(id, uuidv4());
      notify.destructive(
        `Bobot skenario ${SKENARIO_ECL_LABELS[item.skenario]} berhasil dihapus.`,
      );
      router.push("/master/bobot-skenario");
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    } finally {
      setDeleting(false);
      setDeleteOpen(false);
    }
  };

  // ---------------------------------------------------------------------------
  // Loading / Error
  // ---------------------------------------------------------------------------

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
          <div className="space-y-4">
            {Array.from({ length: 7 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
          <Skeleton className="h-80 w-full" />
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <p className="text-sm text-destructive">
          Gagal memuat data bobot skenario {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/bobot-skenario">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const isDraft =
    item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
  const canEdit = perms.canUpdate("ecl_parameter") && isDraft;
  const canDelete = perms.canDelete("ecl_parameter") && isDraft;
  const canSubmit = perms.canSubmit("ecl_parameter") && isDraft;

  const skenarioLabel =
    SKENARIO_ECL_LABELS[item.skenario] ?? item.skenario;

  const bobotPct = bobotDecimalToPercent(item.bobot);

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/bobot-skenario" className="hover:underline">
          Bobot Skenario ECL
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">
          {skenarioLabel} — {item.periodeBerlakuDari}
        </span>
      </nav>

      {/* ECL parameter warning banner */}
      <div
        role="note"
        className="flex items-start gap-3 rounded-md border border-amber-300 bg-amber-50 px-4 py-3"
      >
        <AlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
          aria-hidden
        />
        <p className="text-sm text-amber-800">
          <strong>Parameter ECL</strong> — bobot skenario ini digunakan dalam
          formula weighted ECL (DEC-010). Workflow 6-eyes berlaku: dua tahap
          approval dengan MFA step-up wajib (DEC-027).
        </p>
      </div>

      {/* Title row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-2xl font-semibold">
            Skenario {skenarioLabel}
          </h1>
          <Badge variant="outline" className="font-mono text-sm">
            Bobot: {bobotPct}%
          </Badge>
          <WorkflowStatusBadge status={item.workflowStatus} />
        </div>
        <div className="flex items-center gap-2">
          {canEdit && (
            <Button size="sm" variant="outline" asChild>
              <Link href={`/master/bobot-skenario/${id}/edit`}>
                <Pencil className="mr-1.5 h-4 w-4" aria-hidden />
                Edit
              </Link>
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                className="h-9 w-9"
                aria-label="Aksi lainnya"
              >
                <MoreHorizontal className="h-4 w-4" aria-hidden />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {canSubmit && (
                <DropdownMenuItem onClick={handleSubmit} disabled={workflowSubmitting}>
                  {item.workflowStatus === "RETURNED"
                    ? "Kirim Ulang untuk Review"
                    : "Kirim untuk Review"}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem asChild>
                <Link href={`/master/bobot-skenario/${id}/history`}>
                  Lihat Riwayat Audit
                </Link>
              </DropdownMenuItem>
              {canDelete && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive"
                    onClick={() => setDeleteOpen(true)}
                  >
                    Hapus
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Body: 2-col layout */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[3fr_2fr]">
        {/* Left: detail + trio panel */}
        <div className="space-y-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Detail Parameter Bobot Skenario
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow label="Skenario" value={skenarioLabel} />
                <DetailRow
                  label="Bobot"
                  value={
                    <span className="font-mono font-bold text-lg">
                      {bobotPct}%
                    </span>
                  }
                />
                <DetailRow
                  label="Bobot (desimal)"
                  value={
                    <code className="font-mono text-xs text-muted-foreground">
                      {item.bobot}
                    </code>
                  }
                />
                <DetailRow
                  label="Berlaku Dari"
                  value={fmtDate(item.periodeBerlakuDari)}
                />
                <DetailRow
                  label="Berlaku Sampai"
                  value={
                    item.periodeBerlakuSampai ? (
                      fmtDate(item.periodeBerlakuSampai)
                    ) : (
                      <span className="text-green-700 font-medium">
                        Sekarang (tidak ada tanggal berakhir)
                      </span>
                    )
                  }
                />
                {item.catatan && (
                  <div className="col-span-2 flex flex-col gap-0.5">
                    <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                      Catatan
                    </span>
                    <p className="text-sm whitespace-pre-wrap">{item.catatan}</p>
                  </div>
                )}
              </div>
            </CardContent>
          </Card>

          {/* Trio summary panel */}
          <TrioPanel currentItem={item} />

          {/* Metadata */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm uppercase tracking-wide text-muted-foreground font-semibold">
                Metadata
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <DetailRow label="Dibuat oleh" value={item.createdBy} />
                <DetailRow label="Dibuat pada" value={formatDt(item.createdAt)} />
                <DetailRow label="Diperbarui oleh" value={item.updatedBy} />
                <DetailRow label="Diperbarui pada" value={formatDt(item.updatedAt)} />
                <DetailRow label="Versi" value={item.rowVersion} />
                <DetailRow
                  label="ID"
                  value={
                    <code className="font-mono text-xs text-muted-foreground break-all">
                      {item.id}
                    </code>
                  }
                />
              </div>
            </CardContent>
          </Card>

          <Link
            href={`/master/bobot-skenario/${id}/history`}
            className="text-sm text-primary hover:underline"
          >
            Lihat Riwayat Audit &rarr;
          </Link>
        </div>

        {/* Right: 6-eyes workflow panel */}
        <div>
          <Card>
            <CardContent className="pt-6">
              {item.workflow ? (
                <MakerReviewerApproverPanel
                  workflowData={item.workflow}
                  currentUserId={perms.userId}
                  entityStatus={item.workflowStatus}
                  submitting={workflowSubmitting}
                  onSubmit={handleSubmit}
                  onReview={handleReview}
                  onApprove={handleApprove}
                  onApprove2={handleApprove2}
                  onReject={handleReject}
                />
              ) : (
                <div className="space-y-2">
                  <p className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
                    Proses Persetujuan (6-eyes)
                  </p>
                  <Separator />
                  <p className="text-sm text-muted-foreground">
                    Data belum disubmit ke workflow.
                  </p>
                  {canSubmit && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={workflowSubmitting}
                      onClick={handleSubmit}
                    >
                      {workflowSubmitting ? "Memproses..." : "Kirim untuk Review"}
                    </Button>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Delete confirmation dialog */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Hapus Bobot Skenario {skenarioLabel}?</DialogTitle>
            <DialogDescription>
              Bobot skenario <strong>{skenarioLabel}</strong> (Bobot:{" "}
              {bobotPct}%) untuk periode{" "}
              <strong>{item.periodeBerlakuDari}</strong> akan dihapus
              (soft-delete). Trio skenario periode tersebut akan menjadi tidak
              lengkap dan sum validator akan menampilkan peringatan.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteOpen(false)}
              disabled={deleting}
            >
              Batal
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleting}
            >
              {deleting ? "Menghapus..." : "Hapus"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

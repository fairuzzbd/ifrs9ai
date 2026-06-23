/**
 * /admin/scheduled-emails — Scheduled email configuration.
 * ROLE-AKUN-CTL: create, soft-delete configs.
 * ROLE-AUDIT: read-only.
 * Others: 403 server-side; client absent-from-DOM for write actions.
 */

"use client";

import * as React from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { Plus, Mail } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Separator } from "@/components/ui/separator";
import { ScheduledEmailForm } from "@/components/blips/reporting/ScheduledEmailForm";
import { ScheduledEmailTable } from "@/components/blips/reporting/ScheduledEmailTable";
import { notify } from "@/lib/notify";
import {
  scheduledEmailApi,
  reportingQueryKeys,
} from "@/lib/api/reporting.api";
import type { ScheduledEmailFormInput } from "@/lib/schemas/reporting.schema";
import { parseRecipients } from "@/lib/schemas/reporting.schema";

// ---------------------------------------------------------------------------
// Persona gate (client stub — real gate on server via JWT)
// ---------------------------------------------------------------------------

function useCurrentUserRoles(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem("blips_roles");
    return raw ? (JSON.parse(raw) as string[]) : [];
  } catch {
    return [];
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function ScheduledEmailsPage() {
  const queryClient = useQueryClient();
  const roles = useCurrentUserRoles();
  const isAkunCtl = roles.includes("ROLE-AKUN-CTL");

  const [createOpen, setCreateOpen] = React.useState(false);
  const [deleteTarget, setDeleteTarget] = React.useState<{
    id: string;
    reportSlug: string;
  } | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: reportingQueryKeys.schedEmailList(),
    queryFn: () => scheduledEmailApi.list({ limit: 50, sort: "createdAt:desc" }),
  });

  const createMutation = useMutation({
    mutationFn: ({
      body,
      ik,
    }: {
      body: Parameters<typeof scheduledEmailApi.create>[0];
      ik: string;
    }) => scheduledEmailApi.create(body, ik),
    onSuccess: (res) => {
      notify.success(
        `Jadwal email ${res.data.reportSlug} (${res.data.frequency} ${res.data.sendTime}) berhasil dibuat. ${res.data.recipients.length} penerima aktif.`,
      );
      setCreateOpen(false);
      void queryClient.invalidateQueries({ queryKey: reportingQueryKeys.schedEmailList() });
    },
    onError: (err) => {
      notify.error(err as Parameters<typeof notify.error>[0]);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: ({ id, ik }: { id: string; ik: string }) =>
      scheduledEmailApi.delete(id, ik),
    onSuccess: (_, vars) => {
      notify.success("Jadwal email berhasil dihapus (soft-delete).");
      setDeleteTarget(null);
      void queryClient.invalidateQueries({ queryKey: reportingQueryKeys.schedEmailList() });
    },
    onError: (err) => {
      notify.error(err as Parameters<typeof notify.error>[0]);
      setDeleteTarget(null);
    },
  });

  const handleCreate = async (values: ScheduledEmailFormInput) => {
    createMutation.mutate({
      body: {
        reportSlug: values.reportSlug,
        format: values.format,
        frequency: values.frequency,
        sendTime: values.sendTime,
        recipients: parseRecipients(values.recipients),
        active: values.active,
        subjectTemplate: values.subjectTemplate ?? undefined,
        bodyTemplate: values.bodyTemplate ?? undefined,
      },
      ik: uuidv4(),
    });
  };

  const handleDeleteConfirm = () => {
    if (!deleteTarget) return;
    deleteMutation.mutate({ id: deleteTarget.id, ik: uuidv4() });
  };

  const items = data?.data ?? [];

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Mail className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
          <h1 className="text-xl font-semibold">Jadwal Email Laporan</h1>
        </div>
        {/* Absent from DOM for non-AKUN-CTL */}
        {isAkunCtl && (
          <Button
            type="button"
            size="sm"
            onClick={() => setCreateOpen(true)}
            className="gap-1.5"
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            Tambah Jadwal
          </Button>
        )}
      </div>

      <Separator />

      <ScheduledEmailTable
        items={items}
        isAkunCtl={isAkunCtl}
        onDelete={(id, reportSlug) => setDeleteTarget({ id, reportSlug })}
        loading={isLoading}
      />

      {/* Create dialog — ROLE-AKUN-CTL only */}
      {isAkunCtl && (
        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>Tambah Jadwal Email</DialogTitle>
              <DialogDescription>
                Konfigurasi laporan yang dikirim otomatis via email sesuai jadwal.
              </DialogDescription>
            </DialogHeader>
            <ScheduledEmailForm
              onSubmit={handleCreate}
              submitting={createMutation.isPending}
              onCancel={() => setCreateOpen(false)}
            />
          </DialogContent>
        </Dialog>
      )}

      {/* Delete confirmation */}
      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Hapus Jadwal Email?</AlertDialogTitle>
            <AlertDialogDescription>
              Jadwal email untuk laporan{" "}
              <strong>{deleteTarget?.reportSlug}</strong> akan dihapus (soft-delete). Email
              yang sedang dalam antrian Asynq akan diabaikan. Aksi ini tidak bisa di-undo
              secara langsung.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>Batal</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDeleteConfirm}
              disabled={deleteMutation.isPending}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleteMutation.isPending ? "Menghapus..." : "Hapus Jadwal"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

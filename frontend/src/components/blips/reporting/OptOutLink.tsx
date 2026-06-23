/**
 * OptOutLink — public page component for recipient opt-out.
 * Accessed via signed token URL in email (no auth required).
 * Route: /admin/scheduled-emails/opt-out?id=...&token=...&email=...
 */

"use client";

import * as React from "react";
import { CheckCircle, XCircle, Loader2 } from "lucide-react";
import { scheduledEmailApi } from "@/lib/api/reporting.api";

interface OptOutLinkProps {
  schedId: string;
  token: string;
  email: string;
}

type State = "idle" | "loading" | "success" | "error";

export function OptOutLink({ schedId, token, email }: OptOutLinkProps) {
  const [state, setState] = React.useState<State>("idle");
  const [errorMsg, setErrorMsg] = React.useState("");
  const calledRef = React.useRef(false);

  React.useEffect(() => {
    if (calledRef.current) return;
    calledRef.current = true;

    setState("loading");
    scheduledEmailApi
      .optOut(schedId, token, email)
      .then(() => setState("success"))
      .catch((err) => {
        setErrorMsg(
          (err as { message?: string })?.message ??
            "Token tidak valid atau sudah kadaluwarsa.",
        );
        setState("error");
      });
  }, [schedId, token, email]);

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <div className="max-w-md w-full rounded-xl border bg-card p-8 shadow-sm text-center space-y-4">
        {state === "loading" && (
          <>
            <Loader2 className="mx-auto h-10 w-10 animate-spin text-muted-foreground" aria-hidden="true" />
            <p className="text-sm text-muted-foreground">Memproses opt-out...</p>
          </>
        )}
        {state === "success" && (
          <>
            <CheckCircle className="mx-auto h-10 w-10 text-green-600" aria-hidden="true" />
            <h1 className="text-lg font-semibold">Opt-out Berhasil</h1>
            <p className="text-sm text-muted-foreground">
              Alamat email <strong>{email}</strong> tidak akan menerima laporan terjadwal ini
              ke depannya.
            </p>
            <p className="text-xs text-muted-foreground">
              Konfigurasi jadwal email tidak dihapus — hanya email ini yang dikeluarkan dari
              daftar penerima.
            </p>
          </>
        )}
        {state === "error" && (
          <>
            <XCircle className="mx-auto h-10 w-10 text-red-600" aria-hidden="true" />
            <h1 className="text-lg font-semibold">Opt-out Gagal</h1>
            <p className="text-sm text-muted-foreground">{errorMsg}</p>
          </>
        )}
        {state === "idle" && null}
      </div>
    </div>
  );
}

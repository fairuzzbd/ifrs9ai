import * as React from "react";
import Link from "next/link";
import { Calendar, Plus } from "lucide-react";
import { cookies } from "next/headers";
import { PeriodeTimelineSidebar } from "./_components/PeriodeTimelineSidebar";

// Server component — reads JWT for permission check
function hasPermission(permission: string): boolean {
  const cookieStore = cookies();
  const token = cookieStore.get("blips_token")?.value;
  if (!token) return false;
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return false;
    const raw = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const padded = raw.padEnd(raw.length + ((4 - (raw.length % 4)) % 4), "=");
    const payload = JSON.parse(Buffer.from(padded, "base64").toString("utf-8")) as {
      permissions?: string[];
    };
    return (payload.permissions ?? []).includes(permission);
  } catch {
    return false;
  }
}

export default function PeriodeBukuLayout({ children }: { children: React.ReactNode }) {
  const canRead = hasPermission("periode.read");
  const canCreate = hasPermission("periode.create");

  return (
    <div className="flex flex-col min-h-full">
      {/* Skip-to-main */}
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded focus:bg-background focus:px-3 focus:py-1.5 focus:text-sm focus:shadow focus:ring-2"
      >
        Lewati ke konten utama
      </a>

      {/* Breadcrumb + page header */}
      <div className="border-b px-6 py-3">
        <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground mb-1">
          <Link href="/dashboard" className="hover:underline">
            Beranda
          </Link>
          <span className="mx-1.5" aria-hidden="true">/</span>
          <Link href="/periode-buku" className="hover:underline text-foreground font-medium">
            Periode Buku
          </Link>
        </nav>
        <div className="flex items-center gap-2">
          <Calendar className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
          <h1 className="text-xl font-semibold">Periode Buku</h1>
        </div>
      </div>

      {/* Two-column layout */}
      <div className="flex flex-1">
        {/* Timeline sidebar — col-3 */}
        {canRead && (
          <aside
            className="w-72 shrink-0 border-r overflow-y-auto"
            aria-label="Navigasi Periode Buku"
          >
            <div className="p-4 space-y-1">
              <PeriodeTimelineSidebar canCreate={canCreate} />
            </div>
          </aside>
        )}

        {/* Main content — col-9 */}
        <main id="main-content" className="flex-1 overflow-auto" role="main">
          {children}
        </main>
      </div>
    </div>
  );
}

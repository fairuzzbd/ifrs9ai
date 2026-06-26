/**
 * Shared layout for all /transaksi/* routes.
 *
 * Server Component — reads JWT permissions from cookie and renders the tab nav
 * only for tabs the current user has permission to see.
 *
 * Active-state highlight is delegated to <TransaksiTabs> (client island)
 * which reads usePathname().
 */

import * as React from "react";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { cookies } from "next/headers";
import { TransaksiTabs } from "@/components/blips/transaksi/TransaksiTabs";
import type { TransaksiTab } from "@/components/blips/transaksi/TransaksiTabs";

// ---------------------------------------------------------------------------
// JWT permission decode (no crypto — trust API gateway validation)
// ---------------------------------------------------------------------------

function getPermissionsFromCookie(): string[] {
  try {
    const cookieStore = cookies();
    const token = cookieStore.get("blips_token")?.value;
    if (!token) return [];
    const parts = token.split(".");
    if (parts.length !== 3) return [];
    const raw = parts[1];
    const padded = raw
      .replace(/-/g, "+")
      .replace(/_/g, "/")
      .padEnd(raw.length + ((4 - (raw.length % 4)) % 4), "=");
    const json = Buffer.from(padded, "base64").toString("utf-8");
    const payload = JSON.parse(json) as { permissions?: string[] };
    return payload.permissions ?? [];
  } catch {
    return [];
  }
}

// ---------------------------------------------------------------------------
// All possible tabs (order = display order)
// ---------------------------------------------------------------------------

const ALL_TABS: TransaksiTab[] = [
  {
    label: "Penempatan",
    href: "/transaksi/penempatan",
    permission: "transaksi.penempatan.read",
  },
  {
    label: "MTM",
    href: "/transaksi/mtm",
    permission: "transaksi.mtm.read",
  },
  {
    label: "Renewal",
    href: "/transaksi/renewal",
    permission: "transaksi.renewal.read",
  },
  {
    label: "Penjualan",
    href: "/transaksi/penjualan",
    permission: "transaksi.penjualan.read",
  },
  {
    label: "Jatuh Tempo",
    href: "/transaksi/jatuh-tempo",
    permission: "transaksi.jatuhtempo.read",
  },
  {
    label: "Akrual",
    href: "/transaksi/akrual",
    permission: "transaksi.akrual.read",
  },
];

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

interface TransaksiLayoutProps {
  children: React.ReactNode;
}

export default function TransaksiLayout({ children }: TransaksiLayoutProps) {
  const permissions = getPermissionsFromCookie();

  // Filter tabs by permission — absent-from-DOM if user lacks permission.
  // If no cookie / empty permissions, show all tabs (graceful fallback —
  // actual route protection happens in each page.tsx via requirePermission).
  const visibleTabs =
    permissions.length > 0
      ? ALL_TABS.filter((tab) => permissions.includes(tab.permission))
      : ALL_TABS;

  return (
    <div className="flex min-h-screen flex-col">
      {/* Page header + breadcrumb */}
      <header className="border-b bg-background px-6 pt-5 pb-0">
        <nav aria-label="Breadcrumb" className="mb-3 flex items-center gap-1 text-sm text-muted-foreground">
          <Link href="/dashboard" className="hover:text-foreground hover:underline">
            Beranda
          </Link>
          <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
          <span className="text-foreground font-medium">Transaksi</span>
        </nav>

        <h1 className="text-xl font-semibold text-foreground mb-3">
          Transaksi Portofolio
        </h1>

        {/* Tab nav — client island for active highlighting */}
        <TransaksiTabs tabs={visibleTabs} />
      </header>

      {/* Route content */}
      <main className="flex-1">
        {children}
      </main>
    </div>
  );
}

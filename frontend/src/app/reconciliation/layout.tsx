import * as React from "react";
import Link from "next/link";

export default function ReconciliationLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col min-h-full">
      {/* Skip-to-main */}
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded focus:bg-background focus:px-3 focus:py-1.5 focus:text-sm focus:shadow focus:ring-2"
      >
        Lewati ke konten utama
      </a>

      {/* Breadcrumb */}
      <div className="border-b px-6 py-3">
        <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
          <Link href="/dashboard" className="hover:underline">
            Beranda
          </Link>
          <span className="mx-1.5" aria-hidden="true">/</span>
          <span className="text-foreground font-medium">Rekonsiliasi</span>
        </nav>
      </div>

      {/* Main content */}
      <main id="main-content" className="flex-1" role="main">
        {children}
      </main>
    </div>
  );
}

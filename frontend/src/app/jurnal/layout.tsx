import * as React from "react";
import Link from "next/link";
import { FileText } from "lucide-react";
import { JurnalTabNav } from "@/components/blips/jurnal/JurnalTabNav";

export default function JurnalLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col min-h-full">
      {/* Skip-to-main */}
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded focus:bg-background focus:px-3 focus:py-1.5 focus:text-sm focus:shadow focus:ring-2"
      >
        Lewati ke konten utama
      </a>

      {/* Header */}
      <div className="border-b px-6 py-3">
        <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground mb-1">
          <Link href="/dashboard" className="hover:underline">
            Beranda
          </Link>
          <span className="mx-1.5" aria-hidden="true">/</span>
          <span className="text-foreground font-medium">Jurnal</span>
        </nav>
        <div className="flex items-center gap-2">
          <FileText className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
          <h1 className="text-xl font-semibold">Jurnal</h1>
        </div>
      </div>

      {/* Tab nav — client island */}
      <JurnalTabNav />

      {/* Main content */}
      <main id="main-content" className="flex-1" role="main">
        {children}
      </main>
    </div>
  );
}

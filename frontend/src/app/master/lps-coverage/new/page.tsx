import { LPSCoverageForm } from "@/app/master/lps-coverage/_components/LPSCoverageForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tambah LPS Coverage Cap — BLIPS IFRS9",
};

export default function NewLPSCoveragePage() {
  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/lps-coverage" className="hover:underline">
          LPS Coverage
        </a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah Coverage Cap</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah LPS Coverage Cap</h1>
      <LPSCoverageForm mode="create" />
    </div>
  );
}

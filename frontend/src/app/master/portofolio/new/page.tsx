import { PortofolioForm } from "@/app/master/portofolio/_components/PortofolioForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tambah Portofolio — BLIPS IFRS9",
};

export default function NewPortofolioPage() {
  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/portofolio" className="hover:underline">
          Portofolio
        </a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah Portofolio</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah Portofolio</h1>
      <PortofolioForm mode="create" />
    </div>
  );
}

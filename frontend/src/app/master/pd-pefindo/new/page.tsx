import { PDPefindoForm } from "@/app/master/pd-pefindo/_components/PDPefindoForm";
import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Buat PD Pefindo Manual — BLIPS IFRS9",
};

export default function NewPDPefindoPage() {
  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/pd-pefindo" className="hover:underline">
          PD Pefindo
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Buat Manual</span>
      </nav>
      <h1 className="text-2xl font-semibold">Buat PD Pefindo Manual</h1>
      <PDPefindoForm mode="create" />
    </div>
  );
}

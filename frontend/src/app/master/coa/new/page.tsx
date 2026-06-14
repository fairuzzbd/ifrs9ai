import { CoAForm } from "@/app/master/coa/_components/CoAForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Buat Akun — Chart of Accounts — BLIPS IFRS9",
};

export default function NewCoAPage() {
  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/coa" className="hover:underline">
          Chart of Accounts
        </a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Buat Akun</span>
      </nav>
      <h1 className="text-2xl font-semibold">Buat Akun Baru</h1>
      <CoAForm mode="create" />
    </div>
  );
}

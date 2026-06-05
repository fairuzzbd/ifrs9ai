"use client";

import Link from "next/link";
import { BobotSkenarioForm } from "@/app/master/bobot-skenario/_components/BobotSkenarioForm";

export default function NewBobotSkenarioPage() {
  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/bobot-skenario" className="hover:underline">
          Bobot Skenario ECL
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Buat Baru</span>
      </nav>
      <h1 className="text-2xl font-semibold">Buat Bobot Skenario Baru</h1>
      <p className="text-sm text-muted-foreground">
        Bobot skenario ECL menentukan bobot probabilitas tiap skenario makroekonomi
        (Good / Normal / Bad) dalam formula weighted ECL (DEC-010). Setelah disimpan,
        data perlu melalui review dan dua tahap approval (6-eyes) sebelum dapat
        digunakan dalam kalkulasi ECL. Total bobot ketiga skenario untuk satu periode
        harus = 100% (1.0).
      </p>
      <BobotSkenarioForm mode="create" />
    </div>
  );
}

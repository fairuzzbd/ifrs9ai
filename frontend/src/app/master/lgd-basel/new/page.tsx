"use client";

import Link from "next/link";
import { LGDBaselForm } from "@/app/master/lgd-basel/_components/LGDBaselForm";

export default function NewLGDBaselPage() {
  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/lgd-basel" className="hover:underline">
          LGD Basel Pool
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Buat Baru</span>
      </nav>
      <h1 className="text-2xl font-semibold">Buat LGD Pool Baru</h1>
      <p className="text-sm text-muted-foreground">
        Parameter LGD Basel ini merupakan input ECL. Setelah disimpan, data perlu
        melalui proses review dan dua tahap approval (6-eyes) sebelum dapat digunakan
        dalam kalkulasi ECL.
      </p>
      <LGDBaselForm mode="create" />
    </div>
  );
}

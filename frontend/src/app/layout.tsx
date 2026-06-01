import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "BLIPS IFRS9",
  description:
    "BLIPS IFRS9 — Sistem PSAK 71 / IFRS 9 PT Tugu Reasuransi Indonesia",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="id">
      <body>{children}</body>
    </html>
  );
}

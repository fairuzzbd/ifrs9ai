/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",

  async redirects() {
    return [
      // -----------------------------------------------------------------------
      // M16 — Transaksi (penempatan, MTM)
      // -----------------------------------------------------------------------
      {
        source: "/trx/penempatan/:path*",
        destination: "/transaksi/penempatan/:path*",
        permanent: true,
      },
      {
        source: "/trx/penempatan",
        destination: "/transaksi/penempatan",
        permanent: true,
      },
      {
        source: "/mtm/:path*",
        destination: "/transaksi/mtm/:path*",
        permanent: true,
      },
      {
        source: "/mtm",
        destination: "/transaksi/mtm",
        permanent: true,
      },

      // -----------------------------------------------------------------------
      // M17 — Periode Buku: old /master/periode-buku/* → /periode-buku/*
      // -----------------------------------------------------------------------
      {
        source: "/master/periode-buku/:path*",
        destination: "/periode-buku/:path*",
        permanent: true,
      },
      {
        source: "/master/periode-buku",
        destination: "/periode-buku",
        permanent: true,
      },

      // -----------------------------------------------------------------------
      // M17 — Mapping Jurnal duplicates → canonical /master/mapping-jurnal/*
      // -----------------------------------------------------------------------
      {
        source: "/mapping-jurnal/:path*",
        destination: "/master/mapping-jurnal/:path*",
        permanent: true,
      },
      {
        source: "/mapping-jurnal",
        destination: "/master/mapping-jurnal",
        permanent: true,
      },
      {
        source: "/jrnl/mapping/:path*",
        destination: "/master/mapping-jurnal/:path*",
        permanent: true,
      },
      {
        source: "/jrnl/mapping",
        destination: "/master/mapping-jurnal",
        permanent: true,
      },

      // -----------------------------------------------------------------------
      // M17 — Jurnal Header: old /jrnl/journal-entries/* → /jurnal/header/*
      // -----------------------------------------------------------------------
      {
        source: "/jrnl/journal-entries/:path*",
        destination: "/jurnal/header/:path*",
        permanent: true,
      },
      {
        source: "/jrnl/journal-entries",
        destination: "/jurnal/header",
        permanent: true,
      },
      // old /jrnl/post entry point
      {
        source: "/jrnl/post",
        destination: "/jurnal/header",
        permanent: true,
      },

      // -----------------------------------------------------------------------
      // M17 — DLQ: old /jrnl/gl-delivery-dlq/* → /jurnal/dlq/*
      // -----------------------------------------------------------------------
      {
        source: "/jrnl/gl-delivery-dlq/:path*",
        destination: "/jurnal/dlq/:path*",
        permanent: true,
      },
      {
        source: "/jrnl/gl-delivery-dlq",
        destination: "/jurnal/dlq",
        permanent: true,
      },
      // old /jrnl/dlq/* (before jurnal namespace)
      {
        source: "/jrnl/dlq/:path*",
        destination: "/jurnal/dlq/:path*",
        permanent: true,
      },
      {
        source: "/jrnl/dlq",
        destination: "/jurnal/dlq",
        permanent: true,
      },

      // -----------------------------------------------------------------------
      // M17 — Resolve: old /jrnl/resolve → /jurnal/resolve
      // -----------------------------------------------------------------------
      {
        source: "/jrnl/resolve",
        destination: "/jurnal/resolve",
        permanent: true,
      },

      // -----------------------------------------------------------------------
      // M17 — Reconciliation: old /jrnl/rekonsiliasi → /reconciliation/daily
      // -----------------------------------------------------------------------
      {
        source: "/jrnl/rekonsiliasi",
        destination: "/reconciliation/daily",
        permanent: true,
      },
      {
        source: "/jrnl/rekonsiliasi/:path*",
        destination: "/reconciliation/daily",
        permanent: true,
      },
      // also /reconciliation root → /reconciliation/daily
      {
        source: "/reconciliation",
        destination: "/reconciliation/daily",
        permanent: false,
      },
    ];
  },
};

module.exports = nextConfig;

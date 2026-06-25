/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",

  async redirects() {
    return [
      // penempatan: path-wildcard redirect
      {
        source: "/trx/penempatan/:path*",
        destination: "/transaksi/penempatan/:path*",
        permanent: true,
      },
      // penempatan: root redirect (no trailing path)
      {
        source: "/trx/penempatan",
        destination: "/transaksi/penempatan",
        permanent: true,
      },
      // mtm: path-wildcard redirect
      {
        source: "/mtm/:path*",
        destination: "/transaksi/mtm/:path*",
        permanent: true,
      },
      // mtm: root redirect
      {
        source: "/mtm",
        destination: "/transaksi/mtm",
        permanent: true,
      },
    ];
  },
};

module.exports = nextConfig;

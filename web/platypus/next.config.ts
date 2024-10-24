import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      {
        source: "/api/v1/:path*",
        destination: "http://localhost:7070/api/v1/:path*",
      },
    ];
  },
};

export default nextConfig;

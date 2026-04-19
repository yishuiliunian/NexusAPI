import type { NextConfig } from 'next';

const config: NextConfig = {
  reactStrictMode: true,
  transpilePackages: ['@nexusapi/shared'],
  async rewrites() {
    const backend = process.env.NEXUSAPI_BACKEND_URL ?? 'http://localhost:8080';
    return [{ source: '/api/:path*', destination: `${backend}/api/:path*` }];
  },
};

export default config;

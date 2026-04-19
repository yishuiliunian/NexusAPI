import type { NextConfig } from 'next';

const config: NextConfig = {
  reactStrictMode: true,
  // Docker 部署用 standalone 输出；Bazel 构建不走（dangling symlink）
  output: process.env.NEXT_OUTPUT_STANDALONE === '1' ? 'standalone' : undefined,
  transpilePackages: ['@nexusapi/shared'],
  async rewrites() {
    const backend = process.env.NEXUSAPI_BACKEND_URL ?? 'http://localhost:8080';
    return [{ source: '/api/:path*', destination: `${backend}/api/:path*` }];
  },
};

export default config;

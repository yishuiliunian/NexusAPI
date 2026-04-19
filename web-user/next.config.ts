import type { NextConfig } from 'next';

const config: NextConfig = {
  reactStrictMode: true,
  // Docker 部署用 standalone 输出减小镜像体积；Bazel 构建不走 standalone（会产生
  // dangling symlink）。Dockerfile 里 `ENV NEXT_OUTPUT_STANDALONE=1` 启用。
  output: process.env.NEXT_OUTPUT_STANDALONE === '1' ? 'standalone' : undefined,
  // 允许从 web-shared 工作区包导入 TS 源码
  transpilePackages: ['@nexusapi/shared'],
  // 后端 API 转发（开发阶段）。生产由 Traefik 处理。
  async rewrites() {
    const backend = process.env.NEXUSAPI_BACKEND_URL ?? 'http://localhost:8080';
    return [
      { source: '/api/:path*', destination: `${backend}/api/:path*` },
      { source: '/v1/:path*', destination: `${backend}/v1/:path*` },
    ];
  },
};

export default config;

#!/usr/bin/env node
// Next.js 生产启动入口（由 js_binary 打进 OCI 镜像用）。
// 直接 require 而不是 spawn，避免 symlink 解析问题。
process.argv = [process.argv[0], process.argv[1], 'start'];
require('next/dist/bin/next');

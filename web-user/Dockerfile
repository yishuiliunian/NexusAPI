# NexusAPI web-user Dockerfile（multi-stage → Next.js standalone）
#
# 用法（仓库根目录作为 context）：
#   docker build -f web-user/Dockerfile -t nexusapi/web-user .
#
# 约定：next.config.ts 里 output: 'standalone'
# pnpm workspace 依赖 @nexusapi/shared 工作区包，编译时会被 bundle 进 standalone。

# ---------- 依赖 ----------
FROM node:20-alpine AS deps

RUN corepack enable

WORKDIR /app

# 拷 workspace 根 + 子包清单
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY web-shared/package.json ./web-shared/
COPY web-user/package.json ./web-user/
COPY web-admin/package.json ./web-admin/

RUN pnpm install --frozen-lockfile

# ---------- 构建 ----------
FROM node:20-alpine AS builder

RUN corepack enable

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY --from=deps /app/web-shared/node_modules ./web-shared/node_modules
COPY --from=deps /app/web-user/node_modules ./web-user/node_modules

# 源码
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY web-shared ./web-shared
COPY web-user ./web-user

# Next telemetry 关掉
ENV NEXT_TELEMETRY_DISABLED=1

RUN cd web-user && pnpm run build

# ---------- 运行 ----------
FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

# standalone 产物结构：
#   web-user/.next/standalone/  （含 server.js + node_modules 子集）
#   web-user/.next/static/      （需要手动拷到 standalone）
#   web-user/public/            （需要手动拷到 standalone）
RUN addgroup --system --gid 1001 nodejs && adduser --system --uid 1001 nextjs

COPY --from=builder --chown=nextjs:nodejs /app/web-user/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/web-user/.next/static ./web-user/.next/static
# public 目录可选：存在则拷贝，不存在就跳过
# （当前仓库没有 web-user/public，所以省略）

USER nextjs

EXPOSE 3000

CMD ["node", "web-user/server.js"]

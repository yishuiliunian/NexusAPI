# NexusAPI web-admin Dockerfile（与 web-user 对称，只是入口改 web-admin）
#
# 用法（仓库根目录作为 context）：
#   docker build -f web-admin/Dockerfile -t nexusapi/web-admin .

# ---------- 依赖 ----------
FROM node:20-alpine AS deps

RUN corepack enable
WORKDIR /app

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
COPY --from=deps /app/web-admin/node_modules ./web-admin/node_modules

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY web-shared ./web-shared
COPY web-admin ./web-admin

ENV NEXT_TELEMETRY_DISABLED=1
ENV NEXT_OUTPUT_STANDALONE=1

RUN cd web-admin && pnpm run build

# ---------- 运行 ----------
FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3001
ENV HOSTNAME=0.0.0.0

RUN addgroup --system --gid 1001 nodejs && adduser --system --uid 1001 nextjs

COPY --from=builder --chown=nextjs:nodejs /app/web-admin/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/web-admin/.next/static ./web-admin/.next/static
# public 目录可选：当前仓库没有 web-admin/public，省略

USER nextjs

EXPOSE 3001

CMD ["node", "web-admin/server.js"]

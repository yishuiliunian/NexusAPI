# NexusAPI backend server Dockerfile（multi-stage → distroless）
#
# 用法（仓库根目录作为 context）：
#   docker build -f backend/Dockerfile -t nexusapi/server .
#
# 设计：
#   - Stage 1: golang:1.25 编译静态 binary
#   - Stage 2: distroless/static:nonroot 运行
#   - 最终镜像约 25 MB

# ---------- 编译 ----------
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /src

# 先复制 mod 文件充分利用缓存
COPY go.work ./
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download

# 复制源码
COPY backend ./backend

# 编译 server（静态 + 去符号）
RUN cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- 运行 ----------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/server /app/server
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/server"]

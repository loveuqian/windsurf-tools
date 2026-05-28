# Wails Linux 构建镜像 — 从 macOS 交叉打包 linux/amd64
# 用法：在项目根目录执行：
#   docker build -t wt-linux-builder -f build/linux/Dockerfile .
#   docker run --rm -v $(pwd)/build/bin:/out wt-linux-builder
#
# 产物：build/bin/windsurf-tools-wails (Linux ELF amd64)

FROM golang:1.22-bookworm AS builder

# 安装 Wails Linux 运行时依赖 + 构建依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgtk-3-dev \
    libwebkit2gtk-4.0-dev \
    pkg-config \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

# 安装 wails CLI
RUN go install github.com/wailsapp/wails/v2/cmd/wails@latest

WORKDIR /src

# 先复制 go.mod/sum 利用 Docker 层缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制前端 package.json 利用缓存
COPY frontend/package.json frontend/package-lock.json* ./frontend/
RUN cd frontend && npm ci --legacy-peer-deps 2>/dev/null || npm install

# 复制全部源码
COPY . .

# 构建
RUN wails build -platform linux/amd64 -o windsurf-tools-wails

# 输出阶段 — 只拷产物
FROM scratch AS export
COPY --from=builder /src/build/bin/windsurf-tools-wails /windsurf-tools-wails

# 默认 entrypoint：把产物拷到 /out 挂载卷
FROM debian:bookworm-slim AS runner
COPY --from=builder /src/build/bin/windsurf-tools-wails /windsurf-tools-wails
ENTRYPOINT ["cp", "/windsurf-tools-wails", "/out/windsurf-tools-wails"]

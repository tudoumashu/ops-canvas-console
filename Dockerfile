# 构建 Next.js 前端产物。
FROM oven/bun:1.3.13 AS web-build

WORKDIR /app/web
COPY web/package.json web/bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile --cache-dir=/root/.bun/install/cache
COPY VERSION /app/VERSION
COPY CHANGELOG.md /app/CHANGELOG.md
COPY web ./
RUN bun run build

# 构建 Go 后端入口。
FROM golang:1.25-alpine AS api-build

WORKDIR /app
COPY go.mod go.sum ./
COPY config ./config
COPY handler ./handler
COPY middleware ./middleware
COPY model ./model
COPY repository ./repository
COPY router ./router
COPY service ./service
COPY main.go ./
RUN go build -o /server .

# 运行镜像：Next.js 对外监听 3000，Go 只在容器内部监听 8080。
FROM oven/bun:1.3.13

WORKDIR /app
COPY VERSION /app/VERSION
COPY CHANGELOG.md /app/CHANGELOG.md
COPY --from=api-build /server /app/server
COPY --from=web-build /app/web /app/web
ENV PROMPT_DATA_DIR=/app/data/prompts
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates util-linux && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /app/data/prompts

EXPOSE 3000
# 先启动内部 Go API，再由 Next.js 提供页面并代理 /api/*。
CMD ["sh", "-c", "API_PORT_VALUE=${API_PORT:-8080}; API_BASE_URL_VALUE=${API_BASE_URL:-http://127.0.0.1:${API_PORT_VALUE}}; PORT=${API_PORT_VALUE} /app/server & cd /app/web && API_BASE_URL=${API_BASE_URL_VALUE} bun run start -- --hostname ${APP_HOSTNAME:-0.0.0.0} --port ${WEB_PORT:-3000}"]

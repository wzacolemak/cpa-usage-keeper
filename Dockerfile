FROM node:24-alpine AS web-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
ARG NPM_REGISTRY=https://registry.npmjs.org/
RUN npm ci --registry=${NPM_REGISTRY} --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-bookworm AS go-builder
WORKDIR /app

# go.mod 可能要求比基础镜像更新的工具链，允许自动下载
ENV GOTOOLCHAIN=auto
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=web-builder /app/web/dist ./web/dist
COPY web/static.go ./web/static.go
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X cpa-usage-keeper/internal/version.Version=${VERSION}" \
    -o /out/cpa-usage-keeper ./cmd/server/main.go

FROM golang:1.26-bookworm
WORKDIR /
RUN mkdir -p /data
COPY --from=go-builder /out/cpa-usage-keeper /app/cpa-usage-keeper
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN sed -i 's/\r$//' /usr/local/bin/docker-entrypoint.sh \
	&& chmod +x /usr/local/bin/docker-entrypoint.sh
VOLUME ["/data"]
EXPOSE 8080
# 先去除前缀外围的空格、tab 与换行；路径规范化由就绪路由处理。
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD base_path="${APP_BASE_PATH:-}"; \
    base_path="${base_path#"${base_path%%[![:space:]]*}"}"; \
    base_path="${base_path%"${base_path##*[![:space:]]}"}"; \
    wget -q -O /dev/null "http://127.0.0.1:${APP_PORT:-8080}${base_path}/readyz" || exit 1
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/app/cpa-usage-keeper"]

# syntax=docker/dockerfile:1
FROM golang:1.24.5-bookworm AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/suma-control ./cmd/suma-control \
 && CGO_ENABLED=0 go build -trimpath -o /out/suma-runner ./cmd/suma-runner \
 && CGO_ENABLED=0 go build -trimpath -o /out/suma ./cmd/suma

FROM node:24-bookworm-slim AS node-build-base
WORKDIR /src
RUN npm install --global pnpm@11.25.0
ENV ELECTRON_SKIP_BINARY_DOWNLOAD=1
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
COPY apps/desktop/package.json ./apps/desktop/package.json
COPY packages/contracts/package.json ./packages/contracts/package.json
COPY packages/agent-runtime/package.json ./packages/agent-runtime/package.json
COPY packages/knowledge/package.json ./packages/knowledge/package.json

FROM node-build-base AS web-build
RUN pnpm --filter @suma/web... install --frozen-lockfile
COPY apps/web ./apps/web
COPY packages/contracts ./packages/contracts
COPY packages/ui ./packages/ui
RUN pnpm --filter @suma/web build

FROM node-build-base AS knowledge-build
RUN pnpm --filter @suma/knowledge... install --frozen-lockfile
COPY packages/knowledge ./packages/knowledge
RUN pnpm --filter @suma/knowledge build \
 && pnpm --filter @suma/knowledge deploy --prod --legacy /out/knowledge

FROM debian:bookworm-slim AS runtime-base
RUN apt-get update \
 && apt-get install --yes --no-install-recommends ca-certificates curl \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid 10001 suma \
 && useradd --uid 10001 --gid 10001 --home-dir /var/lib/suma --no-create-home suma \
 && mkdir -p /var/lib/suma /opt/suma \
 && chown 10001:10001 /var/lib/suma /opt/suma
COPY --chmod=0755 deploy/entrypoint.sh /usr/local/bin/suma-entrypoint
USER 10001:10001
WORKDIR /opt/suma
ENTRYPOINT ["/usr/local/bin/suma-entrypoint"]

FROM runtime-base AS runner
USER root
RUN apt-get update \
 && apt-get install --yes --no-install-recommends git \
 && rm -rf /var/lib/apt/lists/*
COPY --from=go-build /out/suma-runner /usr/local/bin/suma-runner
USER 10001:10001
ENV HOME=/var/lib/suma
STOPSIGNAL SIGINT
CMD ["/usr/local/bin/suma-runner"]

FROM node:24-bookworm-slim AS knowledge
RUN apt-get update \
 && apt-get install --yes --no-install-recommends ca-certificates libstdc++6 libgomp1 \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /var/lib/suma /opt/suma/knowledge \
 && chown 10001:10001 /var/lib/suma /opt/suma/knowledge
COPY --from=knowledge-build --chown=10001:10001 /out/knowledge /opt/suma/knowledge
COPY --chmod=0755 deploy/entrypoint.sh /usr/local/bin/suma-entrypoint
USER 10001:10001
WORKDIR /opt/suma/knowledge
ENV SUMA_KNOWLEDGE_HOST=0.0.0.0 SUMA_KNOWLEDGE_PORT=4312 SUMA_KNOWLEDGE_DIR=/var/lib/suma/knowledge
EXPOSE 4312
ENTRYPOINT ["/usr/local/bin/suma-entrypoint"]
CMD ["node", "dist/src/server.js"]

# Default image serves the built web application and the Go API from one origin.
FROM runtime-base AS control
COPY --from=go-build /out/suma-control /out/suma /usr/local/bin/
COPY --from=web-build --chown=10001:10001 /src/apps/web/dist /opt/suma/web
ENV SUMA_ADDR=0.0.0.0:4310 SUMA_DATA_DIR=/var/lib/suma SUMA_WEB_DIR=/opt/suma/web
EXPOSE 4310
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD ["curl", "--fail", "--silent", "--show-error", "http://127.0.0.1:4310/healthz"]
CMD ["/usr/local/bin/suma-control"]

# Multi-stage build for a headless Anytype Heart with JsonAPI enabled.
# Exposes only the JsonAPI port (31009).

FROM golang:1.25-trixie AS builder
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates curl && rm -rf /var/lib/apt/lists/*

# Fetch sources
RUN git clone https://github.com/anyproto/anytype-heart.git .

# Copy local scripts to build machine
COPY scripts /src/scripts

# Download Tantivy prebuilt libs required for linking
RUN make download-tantivy-all

# Build server and bootstrap helper
RUN go build -o /out/grpcserver ./cmd/grpcserver && \
    go build -o /out/jsonapi_bootstrap ./scripts/jsonapi_bootstrap.go

# Runtime image
FROM debian:trixie-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*

# Copy binaries and required assets (Tantivy libs, configs, docs)
COPY --from=builder /out/grpcserver /out/jsonapi_bootstrap ./
COPY --from=builder /src/deps/libs /app/deps/libs
COPY --from=builder /src/core/anytype/config/nodes /app/core/anytype/config/nodes
COPY --from=builder /src/core/api/docs /app/core/api/docs

ENV DATA_ROOT=/data \
    JSONAPI_ADDR=0.0.0.0:31009 \
    GRPC_ADDR=0.0.0.0:31007 \
    GRPCWEB_ADDR=0.0.0.0:31008 \
    MNEMONIC= \
    APP_NAME=jsonapi-cli \
    PLATFORM=jsonapi-cli \
    VERSION=0.0.0-headless \
    TIMEOUT=3m \
    WAIT_SPACES=3m

VOLUME ["/data"]
EXPOSE 31009

COPY --from=builder /src/scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

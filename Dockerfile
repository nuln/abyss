# Build stage
FROM golang:1.26.0-alpine AS builder

# Install build dependencies
RUN apk add --no-cache make git upx && \
    go install mvdan.cc/garble@latest

WORKDIR /app

# Copy source code
COPY . .

# Build args
ARG TAGS=""

# Download dependencies and build
# Note: GITHUB_TOKEN is only needed if accessing private repositories
RUN --mount=type=secret,id=GITHUB_TOKEN \
    GITHUB_TOKEN=$(cat /run/secrets/GITHUB_TOKEN 2>/dev/null) && \
    if [ -n "$GITHUB_TOKEN" ]; then \
      git config --global url."https://x-access-token:${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"; \
    fi && \
    export GOPRIVATE=github.com/nuln/* GONOSUMDB=github.com/nuln/* GONOSUMCHECK=github.com/nuln/* && \
    go mod download && \
    if [ "$TAGS" = "pro" ]; then \
      garble -tiny -literals build -tags pro -ldflags="-s -w" -trimpath -o abyss .; \
    else \
      garble -tiny -literals build -ldflags="-s -w" -trimpath -o abyss .; \
    fi && \
    upx --best --lzma abyss

# Final stage
FROM scratch

# Copy binary and SSL certs from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/abyss /app/abyss

# Expose default port
EXPOSE 8080

# Run the app
ENTRYPOINT ["/app/abyss"]

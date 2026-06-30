# Stage 1: Build the portal-svc binary
FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o portal-svc ./cmd/remote/main.go

# Stage 2: Final minimal runtime image using official sing-box
FROM ghcr.io/sagernet/sing-box:latest

# Install ca-certificates for TLS handshakes (often included, but good to be explicit)
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Create core directory for sing-box so that local subcommand's relative path logic works
RUN mkdir -p /app/core

# Copy the built binary
COPY --from=builder /src/portal-svc /app/portal-svc

# Copy configuration templates
COPY templates/remote_config.tmpl.json /app/templates/remote_config.tmpl.json

# Set the entrypoint to the remote binary
ENTRYPOINT ["/app/portal-svc"]

# Stage 1: Build the unified portal-svc binary
FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o portal-svc ./cmd/svc/main.go

# Stage 2: Download the latest sing-box binary
FROM alpine:latest AS downloader

RUN apk add --no-cache curl jq tar

# Use GitHub API to find the latest release and download the linux-amd64 tarball
RUN LATEST_VERSION=$(curl -s "https://api.github.com/repos/SagerNet/sing-box/releases/latest" | jq -r .tag_name | sed 's/^v//') && \
    echo "Downloading sing-box version: ${LATEST_VERSION}" && \
    curl -fsSL -o sing-box.tar.gz "https://github.com/SagerNet/sing-box/releases/download/v${LATEST_VERSION}/sing-box-${LATEST_VERSION}-linux-amd64.tar.gz" && \
    tar -xzf sing-box.tar.gz && \
    mv "sing-box-${LATEST_VERSION}-linux-amd64/sing-box" /sing-box && \
    chmod +x /sing-box

# Stage 3: Final minimal runtime image
FROM alpine:latest

# Install ca-certificates for TLS handshakes
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Create core directory for sing-box so that dock subcommand's relative path logic works
RUN mkdir -p /app/core

# Copy the built binary
COPY --from=builder /src/portal-svc /app/portal-svc

# Copy configuration templates
COPY templates/dock_config.tmpl.json /app/templates/dock_config.tmpl.json
COPY templates/transit_config.tmpl.json /app/templates/transit_config.tmpl.json

# Copy sing-box binary to the location expected by 'dock', and symlink for 'transit' (in $PATH)
COPY --from=downloader /sing-box /app/core/sing-box
RUN ln -s /app/core/sing-box /usr/local/bin/sing-box

# Set the entrypoint to the unified binary and default command to 'transit'
ENTRYPOINT ["/app/portal-svc"]
CMD ["transit"]

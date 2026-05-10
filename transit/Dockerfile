FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o loader cmd/svc/main.go

FROM ghcr.io/sagernet/sing-box:latest
COPY templates/transit_config.tmpl.json /templates/transit_config.tmpl.json
COPY --from=builder /app/loader /loader
RUN chmod +x /loader

ENTRYPOINT ["/loader", "transit", "-config", "/templates/transit_config.tmpl.json"]
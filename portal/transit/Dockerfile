FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY main.go .
RUN go build -o loader main.go

FROM ghcr.io/sagernet/sing-box:latest
COPY --from=builder /app/loader /loader
RUN chmod +x /loader

ENTRYPOINT ["/loader"]
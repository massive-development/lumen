FROM golang:1.26.3-alpine AS builder
WORKDIR /app
COPY proxy.go .
RUN go mod init bitnet/proxy && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o proxy .

FROM alpine:3
COPY --from=builder /app/proxy /proxy
ENTRYPOINT ["/proxy"]

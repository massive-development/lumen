FROM golang:1.26.3-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tools .

FROM alpine:3.20
RUN apk add --no-cache wget
COPY --from=builder /app/tools /tools
EXPOSE 8083
ENTRYPOINT ["/tools"]

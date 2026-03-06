FROM golang:1.26.0-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /auditor ./cmd/auditor/main.go

FROM alpine:3.23
RUN apk --no-cache add ca-certificates tzdata
RUN adduser -D -u 10001 inwheel
USER inwheel
COPY --from=builder /auditor /auditor
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
ENV TZ=UTC
CMD ["/auditor"]

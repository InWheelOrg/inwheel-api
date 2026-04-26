FROM golang:1.26.2-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /api ./cmd/api/main.go

FROM alpine:3.23
RUN apk --no-cache add ca-certificates tzdata
RUN adduser -D -u 10001 inwheel
USER inwheel
COPY --from=builder /api /api
ENV TZ=UTC
EXPOSE 8080
ENTRYPOINT ["/api"]

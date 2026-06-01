# syntax=docker/dockerfile:1

# ---- Builder stage ----
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Cache dependency download terpisah dari source agar layer dapat di-reuse.
COPY go.mod ./
COPY go.sum* ./
RUN go mod download

COPY . .

# Binary statik tanpa CGO agar portabel di image alpine minimal.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- Final stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && addgroup -S blips && adduser -S -G blips blips

WORKDIR /app
COPY --from=builder /out/api /app/api

USER blips
EXPOSE 8081

ENTRYPOINT ["/app/api"]

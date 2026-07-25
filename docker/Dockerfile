# --- Build stage ---
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go build -mod=readonly -ldflags="-s -w" -buildvcs=false -o /bin/api ./cmd/api
RUN go build -mod=readonly -ldflags="-s -w" -buildvcs=false -o /bin/worker ./cmd/worker
RUN go build -mod=readonly -ldflags="-s -w" -buildvcs=false -o /bin/migrate ./cmd/migrate
RUN go build -mod=readonly -ldflags="-s -w" -buildvcs=false -o /bin/seed ./cmd/seed

# --- Runtime stage (minimal, does not include Go toolchain) ---
FROM alpine:3.24
# tzdata: the postgres driver (pgx, via gorm.io/driver/postgres) resolves the
# DSN's TimeZone=Asia/Jakarta param client-side via Go's time.LoadLocation,
# which reads /usr/share/zoneinfo from the OS — Alpine doesn't ship it by
# default. Without this, every binary here (api/worker/migrate/seed) fails
# to open a DB connection at all with "unknown time zone Asia/Jakarta",
# regardless of what the Postgres server itself supports.
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/api /bin/worker /bin/migrate /bin/seed ./

RUN addgroup -S app && adduser -S app -G app
USER app:app

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/healthz || exit 1
CMD ["./api"]

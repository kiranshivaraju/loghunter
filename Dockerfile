# ─── Build stage ─────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /bin/loghunter-server ./cmd/server
RUN CGO_ENABLED=0 go build -o /bin/loghunter ./cmd/cli

# ─── Runtime stage ───────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

RUN adduser -D -u 1000 loghunter

COPY --from=builder /bin/loghunter-server /usr/local/bin/loghunter-server
COPY --from=builder /bin/loghunter /usr/local/bin/loghunter
COPY migrations /migrations

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

USER loghunter

ENTRYPOINT ["loghunter-server"]

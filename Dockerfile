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

COPY --from=builder /bin/loghunter-server /usr/local/bin/loghunter-server
COPY --from=builder /bin/loghunter /usr/local/bin/loghunter
COPY migrations /migrations

EXPOSE 8080

ENTRYPOINT ["loghunter-server"]

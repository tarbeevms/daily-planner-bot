# Multi-stage build for the Telegram daily planner bot.
FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /app/dailyplanner ./cmd/dailyplanner

# Runtime image
FROM debian:12-slim

WORKDIR /app
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/dailyplanner /usr/local/bin/dailyplanner

# Create a writable directory for SQLite database.
RUN mkdir -p /data
VOLUME ["/data"]

ENV DATABASE_URL=/data/daily_planner.db

ENTRYPOINT ["/usr/local/bin/dailyplanner"]

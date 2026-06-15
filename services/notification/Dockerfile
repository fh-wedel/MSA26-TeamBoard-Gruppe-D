# ── dev stage (air hot-reload, used by docker-compose.override.yml) ───────────
FROM golang:1.25-alpine AS dev
RUN apk add --no-cache git && \
    go install github.com/air-verse/air@latest
WORKDIR /workspace/services/notification
CMD ["air", "-c", ".air.toml"]

# ── build stage ────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /workspace
COPY shared/go/ shared/go/
COPY services/notification/ services/notification/
WORKDIR /workspace/services/notification
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/server ./cmd/server

# ── production stage ───────────────────────────────────────────────────────────
FROM alpine:3.20 AS production
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /bin/server /server
EXPOSE 8005
ENTRYPOINT ["/server"]

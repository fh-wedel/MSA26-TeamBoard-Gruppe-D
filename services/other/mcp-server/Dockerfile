# syntax=docker/dockerfile:1

# ── build stage ────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/mcp-server ./cmd/mcp-server

# ── production stage ───────────────────────────────────────────────────────────
FROM alpine:3.20 AS production
RUN apk add --no-cache ca-certificates
COPY --from=build /bin/mcp-server /mcp-server
EXPOSE 8080
# Deployed as a shared, multi-tenant HTTP server behind the gateway.
ENV MCP_TRANSPORT=http
ENTRYPOINT ["/mcp-server"]

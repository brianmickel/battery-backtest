FROM golang:1.27-alpine AS backend-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o api-server ./cmd/api

FROM node:24-alpine AS frontend-builder
WORKDIR /build
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM alpine:3.24
RUN apk --no-cache add ca-certificates tzdata wget && \
    addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser
WORKDIR /app
COPY --from=backend-builder --chown=appuser:appuser /build/api-server ./
COPY --from=frontend-builder --chown=appuser:appuser /build/dist ./web/dist
COPY --chown=appuser:appuser examples/batteries/ ./examples/batteries/
COPY --chown=appuser:appuser data/locations.json ./data/locations.json
USER appuser
EXPOSE 8080
ENV API_PORT=8080 GIN_MODE=release STATIC_DIR=./web/dist
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1
CMD ["./api-server"]

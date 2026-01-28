# Stage 1: Build Go backend
FROM golang:1.21-alpine AS backend-builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o api-server ./cmd/api

# Stage 2: Build React frontend
FROM node:18-alpine AS frontend-builder

# Set working directory
WORKDIR /build

# Copy package files
COPY web/package*.json ./

# Install dependencies
RUN npm ci

# Copy frontend source
COPY web/ ./

# Build the frontend
RUN npm run build

# Stage 3: Final runtime image
FROM alpine:latest

# Install ca-certificates and wget for HTTPS requests and healthcheck
RUN apk --no-cache add ca-certificates tzdata wget

# Create app user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Set working directory
WORKDIR /app

# Copy backend binary from builder
COPY --from=backend-builder /build/api-server .

# Copy frontend build from builder
COPY --from=frontend-builder /build/dist ./web/dist

# Copy example files needed at runtime
# Copy from backend-builder stage where all files were already copied
COPY --from=backend-builder --chown=appuser:appuser /build/examples ./examples
COPY --from=backend-builder --chown=appuser:appuser /build/data ./data

# Verify files were copied (for debugging) - run as root before switching user
RUN echo "=== Verifying copied files ===" && \
    echo "Working directory:" && pwd && \
    echo "=== /app contents ===" && \
    ls -la /app/ && \
    echo "=== /app/examples contents ===" && \
    ls -la /app/examples/ && \
    echo "=== /app/examples/batteries contents ===" && \
    ls -la /app/examples/batteries/ && \
    echo "=== File count ===" && \
    find /app/examples/batteries -name "*.yaml" 2>/dev/null | wc -l && \
    echo "=== Files found ===" && \
    find /app/examples/batteries -name "*.yaml" 2>/dev/null || echo "No YAML files found"

# Change ownership to app user
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Verify files are accessible as appuser (this will fail if files don't exist)
RUN echo "=== Verifying as appuser ===" && \
    ls -la /app/examples/batteries/ && \
    echo "=== Battery files as appuser ===" && \
    find /app/examples/batteries -name "*.yaml" || echo "ERROR: Cannot find battery files"

# Expose port
EXPOSE 8080

# Set environment variables
ENV API_PORT=8080
ENV API_ENV=production
ENV STATIC_DIR=./web/dist

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the server
CMD ["./api-server"]

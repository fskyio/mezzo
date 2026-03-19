# Build stage
FROM golang:1.26 AS builder
WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=$(git rev-parse --short HEAD)"


# Run stage
FROM scratch AS release

# Import binary from builder
COPY --from=builder /app/mezzo /usr/local/bin/mezzo

# Import other essentials from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Document that we intend to expose port 8006 to whoever runs the container
ENV PORT=8006
EXPOSE 8006

# Add container metadata
LABEL org.opencontainers.image.title="Mezzo" \
      org.opencontainers.image.description="A private and lightweight GIF viewer for Tenor" \
      org.opencontainers.image.authors="FSKY" \
      org.opencontainers.image.url="https://mezzo.fsky.io/" \
      org.opencontainers.image.source="https://foundry.fsky.io/fsky/mezzo" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later"

CMD ["mezzo"]

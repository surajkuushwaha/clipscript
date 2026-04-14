# Stage 1: Build Go binary
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o clipscript ./cmd/api/main.go

# Stage 2: Runtime image with yt-dlp + ffmpeg
FROM python:3.11-slim

# Install ffmpeg and yt-dlp
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg && \
    rm -rf /var/lib/apt/lists/* && \
    pip install --no-cache-dir yt-dlp

# Copy Go binary from builder
COPY --from=builder /app/clipscript /usr/local/bin/clipscript

EXPOSE 8080

CMD ["clipscript"]

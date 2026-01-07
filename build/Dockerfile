# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install git and make
RUN apk add --no-cache git make

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments
ARG APP_ID
ARG APP_HASH

# Build binaries
# We use a script or multiple RUN commands to build for different platforms
# Outputting to /build

RUN mkdir -p /build

# Linux amd64
RUN GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.AppID=${APP_ID} -X main.AppHash=${APP_HASH}" -o /build/tgblobsync-linux-amd64 ./cmd/tgblobsync

# Windows amd64
RUN GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X main.AppID=${APP_ID} -X main.AppHash=${APP_HASH}" -o /build/tgblobsync-windows-amd64.exe ./cmd/tgblobsync

# macOS amd64 (Intel)
RUN GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w -X main.AppID=${APP_ID} -X main.AppHash=${APP_HASH}" -o /build/tgblobsync-darwin-amd64 ./cmd/tgblobsync

# macOS arm64 (Apple Silicon)
RUN GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w -X main.AppID=${APP_ID} -X main.AppHash=${APP_HASH}" -o /build/tgblobsync-darwin-arm64 ./cmd/tgblobsync

# Final stage - essentially just to export the artifacts
# We can use a scratch image or alpine to hold the files.
# But usually, if we want to extract them, we can use `docker create` + `docker cp` or BuildKit output mount.
# For simplicity, let's keep it in this stage or use a minimal image.

FROM scratch AS export
COPY --from=builder /build /

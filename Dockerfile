# Multi-stage Dockerfile: build a static Linux binary and produce a minimal runtime image
FROM golang:1.21-alpine AS builder
WORKDIR /src

# Disable CGO for a static binary, target linux/amd64 by default
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Cache modules
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org,direct && \
    go mod download

# Copy rest of the source and build
COPY . .
RUN go build -ldflags="-s -w" -o /app/server

# Final image: minimal scratch image containing only the binary
FROM scratch
COPY --from=builder /app/server /server

# Port used by the application
EXPOSE 8081

ENTRYPOINT ["/server"]

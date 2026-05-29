FROM golang:1.22-alpine AS builder
WORKDIR /app

# Copy go.mod and go.sum first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o /swiftkart-api ./cmd/api

# Final image
FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /swiftkart-api /usr/local/bin/swiftkart-api
EXPOSE 8080
ENTRYPOINT ["swiftkart-api"]

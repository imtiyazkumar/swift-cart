FROM golang:1.27.3-alpine AS builder
WORKDIR /app

# Copy go.mod and go.sum first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o /yember-core-api ./cmd/api

# Final image
FROM alpine:3.23
RUN apk --no-cache add ca-certificates
COPY --from=builder /yember-core-api /usr/local/bin/yember-core-api
RUN addgroup -S yember && adduser -S -G yember yember && mkdir -p /data/documents && chown yember:yember /data/documents
WORKDIR /data
ENV DOCUMENT_LOCAL_DIR=/data/documents
USER yember
EXPOSE 8080
ENTRYPOINT ["yember-core-api"]

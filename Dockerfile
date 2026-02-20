FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install git for downloading dependencies
RUN apk add --no-cache git

# Copy the rest of the source code
COPY . .

# Generate go.sum and download dependencies
RUN go env -w GOPROXY=https://goproxy.io,direct && go mod tidy

# Build the binary statically
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-w -s" -o fileserver .

# Final stage
FROM alpine:latest

WORKDIR /app

# Add tzdata and ca-certificates
RUN apk --no-cache add ca-certificates tzdata

# Copy the binary
COPY --from=builder /app/fileserver .

# Create the data directory
RUN mkdir -p /data
ENV SERVE_DIR=/data

# Expose port
EXPOSE 8080

ENTRYPOINT ["/app/fileserver"]

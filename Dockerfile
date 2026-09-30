# Build stage
FROM golang:1.27.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o serviceProvider .

# Run stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/serviceProvider .
COPY serviceProvider/sample.conf .
EXPOSE 9270 9271
CMD ["./serviceProvider", "--config", "sample.conf"]

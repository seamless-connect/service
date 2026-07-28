# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o exampleservice .

# Run stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/exampleservice .
COPY templates/ templates/
COPY static/ static/
EXPOSE 8080
CMD ["./exampleservice"]

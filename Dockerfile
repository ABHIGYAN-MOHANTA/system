# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Run stage
FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app

# App writes ssh_host_key (in /app) and data/ (persist with volume)
COPY --from=builder /server .

EXPOSE 23234
ENTRYPOINT ["/app/server"]

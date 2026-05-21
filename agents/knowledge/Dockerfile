FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o knowledge .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/knowledge .
ENV OTEL_SERVICE_NAME=knowledge-agent
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
ENV REDIS_URL=redis://localhost:6379
CMD ["./knowledge"]

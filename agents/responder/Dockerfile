FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o responder .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/responder .
ENV OTEL_SERVICE_NAME=responder-agent
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
CMD ["./responder"]

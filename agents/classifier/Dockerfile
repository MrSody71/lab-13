FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o classifier .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/classifier .
ENV OTEL_SERVICE_NAME=classifier-agent
ENV OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
CMD ["./classifier"]

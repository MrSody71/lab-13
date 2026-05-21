package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/nats-io/nats.go"
)

func main() {
	ctx := context.Background()

	shutdown, err := initTracer(ctx)
	if err != nil {
		slog.Warn("tracer init failed, continuing without tracing", "err", err)
	} else {
		defer shutdown(ctx)
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		slog.Error("failed to connect to NATS", "url", natsURL, "err", err)
		os.Exit(1)
	}
	defer nc.Drain()

	slog.Info("connected to NATS", "url", natsURL)

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	agentID := "responder-" + hostname
	var count atomic.Int64

	tracer := otel.Tracer("responder")

	_, err = nc.Subscribe("tickets.generate_response", func(msg *nats.Msg) {
		ctx, span := tracer.Start(extractCtx(msg), "process.tickets.generate_response")
		defer span.End()

		var req ResponseRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to decode request", "err", err)
			return
		}

		span.SetAttributes(
			attribute.String("ticket_id", req.TicketID),
			attribute.String("category", req.Category),
			attribute.String("priority", req.Priority),
		)

		slog.Info("generating response",
			"ticket_id", req.TicketID,
			"priority", req.Priority,
			"category", req.Category,
			"articles", len(req.Articles),
		)

		result := generateResponse(req)

		span.SetAttributes(
			attribute.String("eta", result.EstimatedResolution),
			attribute.Int("kb_refs", len(result.KBReferences)),
		)

		payload, err := json.Marshal(result)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to encode result", "ticket_id", req.TicketID, "err", err)
			return
		}

		out := nats.NewMsg("tickets.response_ready")
		out.Data = payload
		injectHeaders(ctx, out)

		if err := nc.PublishMsg(out); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to publish result", "ticket_id", req.TicketID, "err", err)
			return
		}

		count.Add(1)
		slog.Info("response published",
			"ticket_id", result.TicketID,
			"priority", req.Priority,
			"eta", result.EstimatedResolution,
			"kb_refs", len(result.KBReferences),
		)
	})
	if err != nil {
		slog.Error("failed to subscribe", "subject", "tickets.generate_response", "err", err)
		os.Exit(1)
	}

	slog.Info("subscribed, waiting for requests", "subject", "tickets.generate_response")

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			hb, _ := json.Marshal(map[string]any{
				"agent_id":  agentID,
				"type":      "responder",
				"status":    "healthy",
				"processed": count.Load(),
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			_ = nc.Publish("agents.heartbeat", hb)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
}

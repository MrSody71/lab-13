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
	"go.opentelemetry.io/otel/trace"

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

	agentID := newAgentID()
	slog.Info("agent identity", "agent_id", agentID)
	slog.Info("connected to NATS", "url", natsURL)

	tracer := otel.Tracer("classifier")
	var queueDepth, count atomic.Int64

	// classifyHandler is shared by both the broadcast and direct-inbox subscriptions.
	classifyHandler := makeClassifyHandler(nc, tracer, &queueDepth, &count)

	// Broadcast channel — used in non-auction (or fallback) mode.
	if _, err = nc.Subscribe("tickets.classify", classifyHandler); err != nil {
		slog.Error("failed to subscribe", "subject", "tickets.classify", "err", err)
		os.Exit(1)
	}

	// Direct inbox — used when auction orchestrator selects this agent as winner.
	if _, err = nc.Subscribe("agents.task."+agentID, classifyHandler); err != nil {
		slog.Error("failed to subscribe", "subject", "agents.task."+agentID, "err", err)
		os.Exit(1)
	}

	// Bid-request handler — responds to auction bid requests.
	if _, err = nc.Subscribe("agents.bid_request", func(msg *nats.Msg) {
		var req BidRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			slog.Warn("bid_request: failed to decode", "err", err)
			return
		}
		if req.TaskType != "classify" {
			return
		}
		depth := queueDepth.Load()
		bid := computeBid(agentID, depth, depth > 0)
		payload, err := json.Marshal(bid)
		if err != nil {
			return
		}
		if err := nc.Publish("agents.bid_response."+req.TaskID, payload); err != nil {
			slog.Warn("bid publish failed", "task_id", req.TaskID, "err", err)
			return
		}
		slog.Info("bid submitted",
			"task_id", req.TaskID,
			"agent_id", agentID,
			"cost", bid.Cost,
			"capacity", bid.Capacity,
			"queue_depth", depth,
		)
	}); err != nil {
		slog.Error("failed to subscribe", "subject", "agents.bid_request", "err", err)
		os.Exit(1)
	}

	slog.Info("agent ready",
		"agent_id", agentID,
		"subjects", "tickets.classify, agents.bid_request, agents.task."+agentID,
	)

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			hb, _ := json.Marshal(map[string]any{
				"agent_id":  agentID,
				"type":      "classifier",
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

	slog.Info("shutting down", "processed", count.Load())
}

// makeClassifyHandler returns the NATS handler used for both broadcast and
// direct task delivery.  Extracted so the logic is shared across subscriptions
// and easy to unit-test indirectly via classifier_test.go.
func makeClassifyHandler(
	nc *nats.Conn,
	tracer trace.Tracer,
	queueDepth *atomic.Int64,
	count *atomic.Int64,
) func(*nats.Msg) {
	return func(msg *nats.Msg) {
		queueDepth.Add(1)
		defer queueDepth.Add(-1)

		ctx, span := tracer.Start(extractCtx(msg), "process.tickets.classify")
		defer span.End()

		var ticket Ticket
		if err := json.Unmarshal(msg.Data, &ticket); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to decode ticket", "err", err)
			return
		}

		span.SetAttributes(attribute.String("ticket_id", ticket.TicketID))

		result := classify(ticket)

		span.SetAttributes(
			attribute.String("category", result.Category),
			attribute.String("priority", result.Priority),
		)

		payload, err := json.Marshal(result)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to encode classification", "ticket_id", ticket.TicketID, "err", err)
			return
		}

		out := nats.NewMsg("tickets.classified")
		out.Data = payload
		injectHeaders(ctx, out)

		if err := nc.PublishMsg(out); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to publish classification", "ticket_id", ticket.TicketID, "err", err)
			return
		}

		n := count.Add(1)
		slog.Info("ticket classified",
			"ticket_id", result.TicketID,
			"category", result.Category,
			"priority", result.Priority,
			"confidence", result.Confidence,
		)
		if n%10 == 0 {
			slog.Info("processed count milestone", "count", n)
		}
	}
}

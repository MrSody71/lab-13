package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/nats-io/nats.go"
)

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

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

	tracer := otel.Tracer("escalation")

	_, err = nc.Subscribe("tickets.escalate", func(msg *nats.Msg) {
		ctx, span := tracer.Start(extractCtx(msg), "process.tickets.escalate")
		defer span.End()

		var req EscalationRequest
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
			attribute.Int("attempts", req.Attempts),
			attribute.String("reason", req.Reason),
		)

		slog.Info("escalating ticket",
			"ticket_id", req.TicketID,
			"priority", req.Priority,
			"category", req.Category,
			"attempts", req.Attempts,
			"reason", req.Reason,
		)

		result, audit := escalate(req, time.Now(), newUUID())

		span.SetAttributes(
			attribute.String("escalated_to", result.EscalatedTo),
			attribute.String("notification_time", result.NotificationTime),
			attribute.String("escalation_id", result.EscalationID),
		)

		resultPayload, err := json.Marshal(result)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to encode result", "ticket_id", req.TicketID, "err", err)
			return
		}

		out := nats.NewMsg("tickets.escalated")
		out.Data = resultPayload
		injectHeaders(ctx, out)

		if err := nc.PublishMsg(out); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to publish escalation", "ticket_id", req.TicketID, "err", err)
			return
		}

		// Audit log also carries trace context for correlation.
		if auditPayload, err := json.Marshal(audit); err == nil {
			auditMsg := nats.NewMsg("tickets.audit")
			auditMsg.Data = auditPayload
			injectHeaders(ctx, auditMsg)
			if err := nc.PublishMsg(auditMsg); err != nil {
				slog.Warn("audit publish failed", "ticket_id", req.TicketID, "err", err)
			}
		}

		slog.Info("ticket escalated",
			"ticket_id", result.TicketID,
			"escalated_to", result.EscalatedTo,
			"notify", result.NotificationTime,
			"escalation_id", result.EscalationID,
		)
	})
	if err != nil {
		slog.Error("failed to subscribe", "subject", "tickets.escalate", "err", err)
		os.Exit(1)
	}

	slog.Info("subscribed, waiting for requests", "subject", "tickets.escalate")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
}

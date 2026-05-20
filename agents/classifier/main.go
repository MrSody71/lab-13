package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

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

	tracer := otel.Tracer("classifier")
	var count atomic.Int64

	_, err = nc.Subscribe("tickets.classify", func(msg *nats.Msg) {
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
	})
	if err != nil {
		slog.Error("failed to subscribe", "subject", "tickets.classify", "err", err)
		os.Exit(1)
	}

	slog.Info("subscribed, waiting for messages", "subject", "tickets.classify")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down", "processed", count.Load())
}

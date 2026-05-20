package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/nats-io/nats.go"
)

func main() {
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

	var count atomic.Int64

	_, err = nc.Subscribe("tickets.classify", func(msg *nats.Msg) {
		var ticket Ticket
		if err := json.Unmarshal(msg.Data, &ticket); err != nil {
			slog.Error("failed to decode ticket", "err", err)
			return
		}

		result := classify(ticket)

		payload, err := json.Marshal(result)
		if err != nil {
			slog.Error("failed to encode classification", "ticket_id", ticket.TicketID, "err", err)
			return
		}

		if err := nc.Publish("tickets.classified", payload); err != nil {
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

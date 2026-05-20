package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
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

	_, err = nc.Subscribe("tickets.generate_response", func(msg *nats.Msg) {
		var req ResponseRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			slog.Error("failed to decode request", "err", err)
			return
		}

		slog.Info("generating response",
			"ticket_id", req.TicketID,
			"priority", req.Priority,
			"category", req.Category,
			"articles", len(req.Articles),
		)

		result := generateResponse(req)

		payload, err := json.Marshal(result)
		if err != nil {
			slog.Error("failed to encode result", "ticket_id", req.TicketID, "err", err)
			return
		}

		if err := nc.Publish("tickets.response_ready", payload); err != nil {
			slog.Error("failed to publish result", "ticket_id", req.TicketID, "err", err)
			return
		}

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
}

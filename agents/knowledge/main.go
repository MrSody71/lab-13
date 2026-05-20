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
	slog.Info("knowledge base loaded", "articles", len(knowledgeBase))

	_, err = nc.Subscribe("tickets.find_solution", func(msg *nats.Msg) {
		var req SolutionRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			slog.Error("failed to decode request", "err", err)
			return
		}

		slog.Info("searching knowledge base",
			"ticket_id", req.TicketID,
			"category", req.Category,
		)

		articles, found := search(knowledgeBase, req.Category, req.Title, req.Description, 3)

		resp := SolutionResponse{
			TicketID: req.TicketID,
			Articles: articles,
			Found:    found,
		}

		payload, err := json.Marshal(resp)
		if err != nil {
			slog.Error("failed to encode response", "ticket_id", req.TicketID, "err", err)
			return
		}

		if err := nc.Publish("tickets.solution_found", payload); err != nil {
			slog.Error("failed to publish response", "ticket_id", req.TicketID, "err", err)
			return
		}

		slog.Info("solution search complete",
			"ticket_id", req.TicketID,
			"found", found,
			"articles_returned", len(articles),
		)
	})
	if err != nil {
		slog.Error("failed to subscribe", "subject", "tickets.find_solution", "err", err)
		os.Exit(1)
	}

	slog.Info("subscribed, waiting for requests", "subject", "tickets.find_solution")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
}

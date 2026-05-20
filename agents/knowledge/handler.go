package main

import (
	"context"
	"encoding/json"
	"log/slog"
)

// processQuery checks the cache, runs a search on miss, updates counters,
// and caches the result. Returns (response, cacheHit).
func processQuery(ctx context.Context, store Store, req SolutionRequest) (SolutionResponse, bool) {
	if cached, ok := store.GetCached(ctx, req.TicketID); ok {
		var resp SolutionResponse
		if err := json.Unmarshal(cached, &resp); err == nil {
			slog.Info("cache hit", "ticket_id", req.TicketID)
			return resp, true
		}
	}

	articles, found := search(knowledgeBase, req.Category, req.Title, req.Description, 3)

	store.IncrTotalQueries(ctx)
	if found {
		store.IncrFoundCount(ctx)
	}
	for _, a := range articles {
		store.IncrArticleHit(ctx, a.ID)
	}

	resp := SolutionResponse{
		TicketID: req.TicketID,
		Articles: articles,
		Found:    found,
	}

	if data, err := json.Marshal(resp); err == nil {
		store.SetCached(ctx, req.TicketID, data)
	}

	return resp, false
}

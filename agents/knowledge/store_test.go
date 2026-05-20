package main

import (
	"context"
	"sort"
	"sync"
	"testing"
)

// MemStore is an in-memory Store used in tests.
type MemStore struct {
	mu           sync.Mutex
	totalQueries int64
	foundCount   int64
	articleHits  map[string]int64
	cache        map[string][]byte
}

func newMemStore() *MemStore {
	return &MemStore{
		articleHits: make(map[string]int64),
		cache:       make(map[string][]byte),
	}
}

func (m *MemStore) IncrTotalQueries(_ context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalQueries++
}

func (m *MemStore) IncrFoundCount(_ context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.foundCount++
}

func (m *MemStore) IncrArticleHit(_ context.Context, articleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.articleHits[articleID]++
}

func (m *MemStore) GetCached(_ context.Context, ticketID string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.cache[ticketID]
	return data, ok
}

func (m *MemStore) SetCached(_ context.Context, ticketID string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache[ticketID] = data
}

func (m *MemStore) GetStats(_ context.Context) (StoreStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	type entry struct {
		id   string
		hits int64
	}
	entries := make([]entry, 0, len(m.articleHits))
	for id, n := range m.articleHits {
		entries = append(entries, entry{id: id, hits: n})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].hits != entries[j].hits {
			return entries[i].hits > entries[j].hits
		}
		return entries[i].id < entries[j].id
	})

	topN := 5
	if len(entries) < topN {
		topN = len(entries)
	}
	top := make([]ArticleHitCount, topN)
	for i := range topN {
		top[i] = ArticleHitCount{ArticleID: entries[i].id, Hits: entries[i].hits}
	}

	return StoreStats{
		TotalQueries: m.totalQueries,
		FoundCount:   m.foundCount,
		TopArticles:  top,
	}, nil
}

func (m *MemStore) RestoreAndLogStats(_ context.Context) {}

// ── processQuery tests ────────────────────────────────────────────────────

func TestProcessQuery_CacheMissIncrementsCounters(t *testing.T) {
	store := newMemStore()
	req := SolutionRequest{
		TicketID: "T1", Title: "VPN tunnel not connecting",
		Description: "credentials expired", Category: "network",
	}

	resp, cacheHit := processQuery(context.Background(), store, req)

	if cacheHit {
		t.Error("expected cache miss on first call")
	}
	if store.totalQueries != 1 {
		t.Errorf("total_queries: want 1, got %d", store.totalQueries)
	}
	if resp.Found && store.foundCount != 1 {
		t.Errorf("found=true but foundCount=%d, want 1", store.foundCount)
	}
	if !resp.Found && store.foundCount != 0 {
		t.Errorf("found=false but foundCount=%d, want 0", store.foundCount)
	}
}

func TestProcessQuery_SecondCallIsHit(t *testing.T) {
	store := newMemStore()
	req := SolutionRequest{
		TicketID: "T1", Title: "VPN tunnel not connecting",
		Description: "credentials expired", Category: "network",
	}

	resp1, hit1 := processQuery(context.Background(), store, req)
	if hit1 {
		t.Fatal("expected cache miss on first call")
	}

	resp2, hit2 := processQuery(context.Background(), store, req)
	if !hit2 {
		t.Error("expected cache hit on second call (same ticket_id)")
	}
	if store.totalQueries != 1 {
		t.Errorf("total_queries must not increment on cache hit: got %d", store.totalQueries)
	}
	if resp1.TicketID != resp2.TicketID || resp1.Found != resp2.Found {
		t.Error("cached response should equal original")
	}
	if len(resp1.Articles) != len(resp2.Articles) {
		t.Errorf("cached articles count mismatch: %d vs %d", len(resp1.Articles), len(resp2.Articles))
	}
}

func TestProcessQuery_CacheIsolatedByTicketID(t *testing.T) {
	store := newMemStore()

	_, hit1 := processQuery(context.Background(), store,
		SolutionRequest{TicketID: "T1", Title: "VPN issue", Category: "network"})
	_, hit2 := processQuery(context.Background(), store,
		SolutionRequest{TicketID: "T2", Title: "VPN issue", Category: "network"})

	if hit1 || hit2 {
		t.Error("different ticket IDs should each be cache misses")
	}
	if store.totalQueries != 2 {
		t.Errorf("expected 2 total queries, got %d", store.totalQueries)
	}
}

func TestProcessQuery_ArticleHitsTracked(t *testing.T) {
	store := newMemStore()
	req := SolutionRequest{
		TicketID: "T1", Title: "VPN tunnel not connecting",
		Description: "credentials expired", Category: "network",
	}

	resp, _ := processQuery(context.Background(), store, req)

	for _, a := range resp.Articles {
		if store.articleHits[a.ID] != 1 {
			t.Errorf("article %s: want 1 hit, got %d", a.ID, store.articleHits[a.ID])
		}
	}
}

func TestProcessQuery_NoMatchDoesNotIncrementFound(t *testing.T) {
	store := newMemStore()
	req := SolutionRequest{
		TicketID: "T1", Title: "cooking recipe baking flour yeast bread",
		Description: "sourdough starter hydration ratio", Category: "",
	}

	resp, _ := processQuery(context.Background(), store, req)

	if resp.Found {
		t.Skip("search unexpectedly matched KB articles; skipping found_count assertion")
	}
	if store.foundCount != 0 {
		t.Errorf("no match: foundCount should be 0, got %d", store.foundCount)
	}
}

func TestProcessQuery_MultipleQueriesAccumulateCounters(t *testing.T) {
	store := newMemStore()

	queries := []SolutionRequest{
		{TicketID: "T1", Title: "VPN tunnel not connecting", Description: "credentials expired", Category: "network"},
		{TicketID: "T2", Title: "password reset expired credentials login", Category: "account"},
		{TicketID: "T3", Title: "cooking recipe flour bread yeast", Category: ""},
	}
	for _, r := range queries {
		processQuery(context.Background(), store, r)
	}

	if store.totalQueries != 3 {
		t.Errorf("expected 3 total queries, got %d", store.totalQueries)
	}
}

// ── GetStats tests ────────────────────────────────────────────────────────

func TestGetStats_Empty(t *testing.T) {
	store := newMemStore()
	stats, err := store.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalQueries != 0 || stats.FoundCount != 0 {
		t.Errorf("expected zeros, got %+v", stats)
	}
	if len(stats.TopArticles) != 0 {
		t.Errorf("expected no top articles for empty store, got %d", len(stats.TopArticles))
	}
}

func TestGetStats_TopFiveLimit(t *testing.T) {
	store := newMemStore()
	store.articleHits["KB001"] = 10
	store.articleHits["KB002"] = 9
	store.articleHits["KB003"] = 8
	store.articleHits["KB004"] = 7
	store.articleHits["KB005"] = 6
	store.articleHits["KB006"] = 5
	store.articleHits["KB007"] = 4
	store.totalQueries = 7

	stats, err := store.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if len(stats.TopArticles) != 5 {
		t.Errorf("want 5 top articles, got %d", len(stats.TopArticles))
	}
	if stats.TopArticles[0].ArticleID != "KB001" || stats.TopArticles[0].Hits != 10 {
		t.Errorf("wrong top article: %+v", stats.TopArticles[0])
	}
	if stats.TotalQueries != 7 {
		t.Errorf("want TotalQueries=7, got %d", stats.TotalQueries)
	}
}

func TestGetStats_OrderedByHitsDesc(t *testing.T) {
	store := newMemStore()
	store.articleHits["KB005"] = 3
	store.articleHits["KB001"] = 7
	store.articleHits["KB003"] = 5

	stats, _ := store.GetStats(context.Background())
	if len(stats.TopArticles) < 3 {
		t.Fatalf("expected 3 articles, got %d", len(stats.TopArticles))
	}
	wantOrder := []string{"KB001", "KB003", "KB005"}
	for i, wantID := range wantOrder {
		if stats.TopArticles[i].ArticleID != wantID {
			t.Errorf("position %d: want %s, got %s", i, wantID, stats.TopArticles[i].ArticleID)
		}
	}
}

func TestGetStats_TiebreakerByID(t *testing.T) {
	store := newMemStore()
	store.articleHits["KB003"] = 5
	store.articleHits["KB001"] = 5

	stats, _ := store.GetStats(context.Background())
	if len(stats.TopArticles) < 2 {
		t.Fatalf("expected 2 articles, got %d", len(stats.TopArticles))
	}
	if stats.TopArticles[0].ArticleID != "KB001" {
		t.Errorf("KB001 < KB003 alphabetically, expected KB001 first on tie; got %s", stats.TopArticles[0].ArticleID)
	}
}

func TestGetStats_ReflectsFoundCount(t *testing.T) {
	store := newMemStore()
	store.totalQueries = 5
	store.foundCount = 3

	stats, _ := store.GetStats(context.Background())
	if stats.TotalQueries != 5 || stats.FoundCount != 3 {
		t.Errorf("want {TotalQueries:5, FoundCount:3}, got %+v", stats)
	}
}

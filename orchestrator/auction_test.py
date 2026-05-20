"""Unit tests for auction winner selection logic.

These tests cover select_winner() only — a pure function with no I/O.
Run with:  python -m unittest auction_test   (no extra packages required)
           pytest auction_test.py            (if pytest is installed)
"""
import sys
import types
import unittest

# ---------------------------------------------------------------------------
# Stub the heavy dependencies so auction.py can be imported without a live
# NATS broker, OTel collector, or Docker daemon.
# ---------------------------------------------------------------------------
for _mod in (
    "nats", "nats.aio", "nats.aio.client",
    "opentelemetry", "opentelemetry.propagate",
    "opentelemetry.trace", "opentelemetry.sdk",
    "opentelemetry.sdk.trace", "opentelemetry.sdk.resources",
    "opentelemetry.sdk.trace.export",
    "opentelemetry.exporter.otlp.proto.http.trace_exporter",
    "opentelemetry.baggage", "opentelemetry.baggage.propagation",
    "opentelemetry.propagators", "opentelemetry.propagators.composite",
    "opentelemetry.trace.propagation",
    "opentelemetry.trace.propagation.tracecontext",
    "dotenv", "docker", "docker.errors",
):
    sys.modules.setdefault(_mod, types.ModuleType(_mod))

# Provide a minimal AgentOrchestrator stub so AuctionOrchestrator can inherit.
_main_stub = types.ModuleType("main")


class _StubOrchestrator:
    pass


_main_stub.AgentOrchestrator = _StubOrchestrator  # type: ignore[attr-defined]
sys.modules["main"] = _main_stub

# pipeline stub
_pipeline_stub = types.ModuleType("pipeline")


class _StubStep:
    pass


_pipeline_stub.PipelineStep = _StubStep  # type: ignore[attr-defined]
sys.modules["pipeline"] = _pipeline_stub

# Now safe to import the pure pieces from auction.
from auction import Bid, select_winner  # noqa: E402


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

class TestSelectWinner(unittest.TestCase):

    def test_lowest_cost_per_capacity_wins(self):
        """Agent with best cost/capacity ratio should win."""
        bids = [
            Bid("agent-1", cost=60.0, capacity=3, eta_ms=200),  # score ≈ 20.0
            Bid("agent-2", cost=50.0, capacity=1, eta_ms=100),  # score = 50.0
            Bid("agent-3", cost=80.0, capacity=3, eta_ms=300),  # score ≈ 26.7
        ]
        winner = select_winner(bids)
        self.assertIsNotNone(winner)
        self.assertEqual(winner.agent_id, "agent-1")

    def test_single_bid_is_winner(self):
        bids = [Bid("solo", cost=50.0, capacity=1, eta_ms=100)]
        winner = select_winner(bids)
        self.assertIsNotNone(winner)
        self.assertEqual(winner.agent_id, "solo")

    def test_no_bids_returns_none(self):
        self.assertIsNone(select_winner([]))

    def test_zero_capacity_bids_excluded(self):
        """An agent with capacity=0 should never win."""
        bids = [
            Bid("broken", cost=1.0, capacity=0, eta_ms=10),
            Bid("healthy", cost=50.0, capacity=1, eta_ms=100),
        ]
        winner = select_winner(bids)
        self.assertIsNotNone(winner)
        self.assertEqual(winner.agent_id, "healthy")

    def test_all_zero_capacity_returns_none(self):
        bids = [
            Bid("a", cost=10.0, capacity=0, eta_ms=50),
            Bid("b", cost=20.0, capacity=0, eta_ms=50),
        ]
        self.assertIsNone(select_winner(bids))

    def test_high_capacity_wins_at_equal_cost(self):
        """Same cost → agent with more capacity has lower score → wins."""
        bids = [
            Bid("busy",  cost=50.0, capacity=1, eta_ms=100),  # score = 50.0
            Bid("free",  cost=50.0, capacity=3, eta_ms=100),  # score ≈ 16.7
        ]
        winner = select_winner(bids)
        self.assertIsNotNone(winner)
        self.assertEqual(winner.agent_id, "free")

    def test_lower_cost_wins_at_equal_capacity(self):
        bids = [
            Bid("cheap",     cost=40.0, capacity=2, eta_ms=100),  # score = 20.0
            Bid("expensive", cost=80.0, capacity=2, eta_ms=100),  # score = 40.0
        ]
        winner = select_winner(bids)
        self.assertIsNotNone(winner)
        self.assertEqual(winner.agent_id, "cheap")

    def test_winner_is_deterministic_for_same_score(self):
        """Tie on score: select_winner must return *some* valid bid, not crash."""
        bids = [
            Bid("a", cost=50.0, capacity=1, eta_ms=100),  # score = 50.0
            Bid("b", cost=100.0, capacity=2, eta_ms=200), # score = 50.0
        ]
        winner = select_winner(bids)
        self.assertIsNotNone(winner)
        self.assertIn(winner.agent_id, ("a", "b"))

    def test_negative_capacity_excluded(self):
        """Negative capacity (malformed bid) must not win."""
        bids = [
            Bid("bad", cost=1.0, capacity=-1, eta_ms=10),
            Bid("ok",  cost=50.0, capacity=2,  eta_ms=100),
        ]
        winner = select_winner(bids)
        self.assertIsNotNone(winner)
        self.assertEqual(winner.agent_id, "ok")

    def test_returns_bid_object(self):
        """Return type must be a Bid, not just the agent_id string."""
        bids = [Bid("x", cost=50.0, capacity=2, eta_ms=80)]
        winner = select_winner(bids)
        self.assertIsInstance(winner, Bid)
        self.assertEqual(winner.cost, 50.0)
        self.assertEqual(winner.capacity, 2)
        self.assertEqual(winner.eta_ms, 80)

    def test_many_bids_correct_winner(self):
        """Winner from a larger pool is still the minimum score."""
        bids = [Bid(f"agent-{i}", cost=float(50 + i * 5), capacity=1, eta_ms=100)
                for i in range(10)]
        # agent-0 has the lowest cost (50) with capacity 1 → score 50
        winner = select_winner(bids)
        self.assertEqual(winner.agent_id, "agent-0")


if __name__ == "__main__":
    unittest.main()

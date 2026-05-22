"""Auction-based task distribution for the classify step.

Protocol:
  1. Orchestrator broadcasts a BidRequest to "agents.bid_request".
  2. Each available classifier publishes a BidResponse to
     "agents.bid_response.<task_id>" within the collection window.
  3. Orchestrator selects the winner (lowest cost / capacity score).
  4. Task is delivered directly to the winner via "agents.task.<agent_id>".
  5. If no bids arrive, fall back to broadcast on "tickets.classify".
"""
from __future__ import annotations

import asyncio
import json
import logging
import uuid
from dataclasses import dataclass
from typing import Any

logger = logging.getLogger(__name__)

_BID_WINDOW_SECS = 2.0   # how long to wait for bids


# ── pure types and selection logic ────────────────────────────────────────────

@dataclass
class Bid:
    agent_id: str
    cost: float
    capacity: int
    eta_ms: int


def select_winner(bids: list[Bid]) -> Bid | None:
    """Return the bid with the lowest cost-per-capacity score.

    Bids with capacity <= 0 are excluded (agent not available).
    Returns None when no valid bids exist.
    """
    valid = [b for b in bids if b.capacity > 0]
    if not valid:
        return None
    return min(valid, key=lambda b: b.cost / b.capacity)


# ── AuctionOrchestrator ────────────────────────────────────────────────────────
# AgentOrchestrator is imported at module level.  This is safe because main.py
# only imports AuctionOrchestrator *inside* the _run() function (not at module
# level), so main is already fully loaded before this import runs.

try:
    from main import AgentOrchestrator as _Base
except ImportError:  # pragma: no cover — happens only in isolated unit-test runs
    _Base = object   # type: ignore[assignment,misc]

from pipeline import PipelineStep  # noqa: E402  (after the try/except)


class AuctionOrchestrator(_Base):  # type: ignore[misc]
    """Extends AgentOrchestrator with auction-based dispatch for the classify step.

    All other pipeline steps continue to use direct broadcast.
    """

    # ── override: dispatch via auction for classify, broadcast for everything else

    async def _publish_task(
        self,
        step: PipelineStep,
        payload: dict,
        headers: dict,
        span,
    ) -> None:
        if step.name != "classify":
            await self._nc.publish(
                step.publish_subject,
                json.dumps(payload).encode(),
                headers=headers,
            )
            return

        task_id = str(uuid.uuid4())
        winner = await self._run_auction(task_id, payload)

        if winner:
            span.set_attribute("auction.winner_id", winner.agent_id)
            span.set_attribute("auction.cost_score", winner.cost / winner.capacity)
            span.add_event("auction_winner", {
                "agent_id": winner.agent_id,
                "cost": winner.cost,
                "capacity": winner.capacity,
            })
            logger.info(
                "auction: winner=%s cost=%.1f capacity=%d task_id=%s",
                winner.agent_id, winner.cost, winner.capacity, task_id,
            )
            await self._nc.publish(
                f"agents.task.{winner.agent_id}",
                json.dumps(payload).encode(),
                headers=headers,
            )
        else:
            logger.info(
                "auction: no bids received, falling back to broadcast task_id=%s",
                task_id,
            )
            span.add_event("auction_fallback_broadcast")
            await self._nc.publish(
                step.publish_subject,
                json.dumps(payload).encode(),
                headers=headers,
            )

    # ── auction internals ──────────────────────────────────────────────────────

    async def _run_auction(self, task_id: str, payload: dict) -> Bid | None:
        """Subscribe to bid responses, broadcast the bid request, collect for
        _BID_WINDOW_SECS, then return the winner (or None if no bids)."""
        bid_subject = f"agents.bid_response.{task_id}"
        bids: list[Bid] = []

        async def _collect(msg: Any) -> None:
            try:
                data = json.loads(msg.data)
                bids.append(Bid(
                    agent_id=data["agent_id"],
                    cost=float(data["cost"]),
                    capacity=int(data["capacity"]),
                    eta_ms=int(data["eta_ms"]),
                ))
                logger.debug(
                    "auction: bid received agent=%s cost=%.1f capacity=%d task_id=%s",
                    data["agent_id"], data["cost"], data["capacity"], task_id,
                )
            except (KeyError, ValueError, json.JSONDecodeError) as exc:
                logger.warning("auction: malformed bid task_id=%s: %s", task_id, exc)

        sub = await self._nc.subscribe(bid_subject, cb=_collect)
        try:
            await self._nc.publish(
                "agents.bid_request",
                json.dumps({
                    "task_id": task_id,
                    "task_type": "classify",
                    "payload": payload,
                    "deadline_ms": 5000,
                }).encode(),
            )
            await asyncio.sleep(_BID_WINDOW_SECS)
        finally:
            await sub.unsubscribe()

        logger.info(
            "auction: collected %d bid(s) for task_id=%s", len(bids), task_id
        )
        return select_winner(bids)

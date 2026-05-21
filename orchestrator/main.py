"""Orchestrator — coordinates the multi-agent IT support pipeline."""

from __future__ import annotations

import asyncio
import json
import logging
import os
from datetime import datetime, timezone

import nats
import redis.asyncio as aioredis
from dotenv import load_dotenv
from opentelemetry.propagate import inject, extract
from opentelemetry.trace import StatusCode

from pipeline import Pipeline, PipelineStep
from scaler import AgentScaler
from tracing import init_tracer

load_dotenv()
logger = logging.getLogger(__name__)

# Subjects the orchestrator listens on for agent replies
_REPLY_SUBJECTS = (
    "tickets.classified",
    "tickets.solution_found",
    "tickets.response_ready",
    "tickets.escalated",
    "tickets.llm_enhanced",
)


class AgentOrchestrator:
    def __init__(
        self,
        nats_url: str | None = None,
        step_timeout: float = 30.0,
        max_retries: int = 3,
    ) -> None:
        self._nats_url = nats_url or os.getenv("NATS_URL", "nats://localhost:4222")
        self._redis_url = os.getenv("REDIS_URL", "redis://redis:6379")
        self._step_timeout = step_timeout
        self._max_retries = max_retries
        self._nc = None
        self._redis = None
        # Keyed by (reply_subject, ticket_id) so concurrent tickets never collide
        self._pending: dict[tuple[str, str], asyncio.Future] = {}
        self._tracer = init_tracer()

    # ── lifecycle ──────────────────────────────────────────────────────────────

    async def connect(self) -> None:
        self._redis = aioredis.from_url(self._redis_url, decode_responses=True)
        self._nc = await nats.connect(self._nats_url)
        for subject in _REPLY_SUBJECTS:
            await self._nc.subscribe(subject, cb=self._make_handler(subject))
        await self._nc.subscribe("tickets.incoming", cb=self._handle_incoming)
        logger.info("connected nats_url=%s redis_url=%s", self._nats_url, self._redis_url)

    async def close(self) -> None:
        if self._nc:
            await self._nc.drain()
        if self._redis:
            await self._redis.aclose()

    async def __aenter__(self) -> AgentOrchestrator:
        await self.connect()
        return self

    async def __aexit__(self, *_) -> None:
        await self.close()

    # ── message routing ────────────────────────────────────────────────────────

    def _make_handler(self, subject: str):
        """Return a NATS message handler that resolves the pending future for
        the arriving ticket_id on this subject."""
        async def _handler(msg) -> None:
            try:
                data = json.loads(msg.data)
            except json.JSONDecodeError:
                logger.warning("non-JSON message subject=%s", subject)
                return
            ticket_id = data.get("ticket_id")
            if not ticket_id:
                return
            fut = self._pending.get((subject, ticket_id))
            if fut and not fut.done():
                # Carry headers so _execute_step can extract the agent's trace context
                fut.set_result((data, msg.headers or {}))

        return _handler

    async def _handle_incoming(self, msg) -> None:
        """Handle manually submitted tickets from the dashboard."""
        try:
            data = json.loads(msg.data)
            asyncio.create_task(self.process_ticket(
                ticket_id=data["ticket_id"],
                title=data["title"],
                description=data["description"],
            ))
            logger.info("incoming ticket queued ticket_id=%s", data.get("ticket_id"))
        except Exception as exc:
            logger.warning("incoming ticket handler error: %s", exc)

    # ── step execution with retry ──────────────────────────────────────────────

    async def _execute_step(
        self, step: PipelineStep, payload: dict, ticket_id: str
    ) -> dict:
        with self._tracer.start_as_current_span(f"step.{step.name}") as span:
            span.set_attribute("ticket_id", ticket_id)
            span.set_attribute("subject", step.publish_subject)

            key = (step.reply_subject, ticket_id)
            for attempt in range(step.max_retries + 1):
                fut: asyncio.Future = asyncio.get_running_loop().create_future()
                self._pending[key] = fut

                # Propagate current span context into the outgoing NATS message
                headers: dict[str, str] = {}
                inject(headers)

                await self._publish_task(step, payload, headers, span)
                span.add_event("task_sent", {"attempt": attempt + 1})

                try:
                    result_data, resp_headers = await asyncio.wait_for(
                        fut, timeout=step.timeout
                    )
                    span.add_event("result_received")
                    # Record the agent's own trace ID for cross-service correlation
                    if resp_headers and (tid := resp_headers.get("traceId")):
                        span.set_attribute("agent.trace_id", tid)
                    return result_data

                except asyncio.TimeoutError:
                    self._pending.pop(key, None)
                    span.add_event("timeout", {"attempt": attempt + 1})
                    if attempt < step.max_retries:
                        span.add_event("retry_attempt", {"next_attempt": attempt + 2})
                        logger.warning(
                            "step '%s' timed out — attempt %d/%d ticket_id=%s",
                            step.name, attempt + 1, step.max_retries, ticket_id,
                        )
                    else:
                        logger.error(
                            "step '%s' exhausted %d retries ticket_id=%s",
                            step.name, step.max_retries, ticket_id,
                        )
                        span.set_status(
                            StatusCode.ERROR,
                            f"timed out after {step.max_retries} retries",
                        )
                        raise TimeoutError(
                            f"step '{step.name}' failed after {step.max_retries} retries"
                            f" (ticket_id={ticket_id})"
                        )
                except BaseException as exc:
                    self._pending.pop(key, None)
                    span.record_exception(exc)
                    span.set_status(StatusCode.ERROR, str(exc))
                    raise

    async def _publish_task(
        self, step: PipelineStep, payload: dict, headers: dict, span
    ) -> None:
        """Publish a task to the appropriate NATS subject.

        Subclasses can override this to replace direct broadcast with an
        auction or other dispatch strategy.
        """
        await self._nc.publish(
            step.publish_subject,
            json.dumps(payload).encode(),
            headers=headers,
        )

    def _make_step(self, name: str, pub: str, reply: str) -> PipelineStep:
        return PipelineStep(
            name=name,
            publish_subject=pub,
            reply_subject=reply,
            timeout=self._step_timeout,
            max_retries=self._max_retries,
        )

    # ── public API ─────────────────────────────────────────────────────────────

    async def process_ticket(
        self, ticket_id: str, title: str, description: str
    ) -> dict:
        """Run the full multi-agent pipeline for one ticket.

        Pipeline:
          1. classify        → tickets.classify / tickets.classified
          2. find_solution   → tickets.find_solution / tickets.solution_found
          3a. generate_response (if found=True and priority≠critical)
          3b. escalate       (if found=False OR priority=critical)
          4. publish tickets.completed
        """
        with self._tracer.start_as_current_span("pipeline.process_ticket") as root_span:
            root_span.set_attribute("ticket_id", ticket_id)
            try:
                return await self._run_pipeline(ticket_id, title, description)
            except Exception as exc:
                root_span.set_status(StatusCode.ERROR, str(exc))
                raise

    async def _run_pipeline(
        self, ticket_id: str, title: str, description: str
    ) -> dict:
        started_at = datetime.now(timezone.utc)
        logger.info("processing ticket_id=%s", ticket_id)

        # Steps 1 & 2 run as a linear Pipeline
        pipeline = (
            Pipeline()
            .add_step(self._make_step("classify", "tickets.classify", "tickets.classified"))
            .add_step(self._make_step("find_solution", "tickets.find_solution", "tickets.solution_found"))
        )

        ctx: dict = {
            "ticket_id": ticket_id,
            "title": title,
            "description": description,
        }

        async def _run_step(step: PipelineStep, current_ctx: dict) -> dict:
            return await self._execute_step(
                step, _build_payload(step.name, current_ctx), ticket_id
            )

        ctx = await pipeline.run(ctx, _run_step)

        logger.info(
            "classified ticket_id=%s category=%s priority=%s confidence=%s",
            ticket_id, ctx.get("category"), ctx.get("priority"), ctx.get("confidence"),
        )
        logger.info(
            "solution ticket_id=%s found=%s articles=%d",
            ticket_id, ctx.get("found"), len(ctx.get("articles", [])),
        )

        # Step 3: branch on whether a solution was found and ticket priority
        found: bool = bool(ctx.get("found", False))
        priority: str = ctx.get("priority", "low")

        if found and priority != "critical":
            outcome = await self._execute_step(
                self._make_step(
                    "generate_response",
                    "tickets.generate_response",
                    "tickets.response_ready",
                ),
                _build_payload("generate_response", ctx),
                ticket_id,
            )
            outcome_type = "response"
            logger.info("response generated ticket_id=%s", ticket_id)
            if os.getenv("LLM_ENABLED", "false").lower() in ("1", "true", "yes"):
                ctx.update(outcome)
                outcome = await self._execute_step(
                    self._make_step(
                        "llm_enhance",
                        "tickets.llm_enhance",
                        "tickets.llm_enhanced",
                    ),
                    _build_payload("llm_enhance", ctx),
                    ticket_id,
                )
                outcome_type = "llm_enhanced"
                logger.info("llm enhanced ticket_id=%s", ticket_id)
        else:
            ctx["escalation_reason"] = (
                "critical_priority" if found else "no_solution_found"
            )
            outcome = await self._execute_step(
                self._make_step("escalate", "tickets.escalate", "tickets.escalated"),
                _build_payload("escalate", ctx),
                ticket_id,
            )
            outcome_type = "escalated"
            logger.info(
                "escalated ticket_id=%s escalated_to=%s",
                ticket_id, outcome.get("escalated_to"),
            )

        # Step 4: broadcast completion event
        completed_at = datetime.now(timezone.utc)
        result = {
            "ticket_id": ticket_id,
            "title": title,
            "category": ctx.get("category"),
            "priority": priority,
            "found": found,
            "outcome_type": outcome_type,
            "outcome": outcome,
            "completed_at": completed_at.isoformat(),
            "latency_ms": int((completed_at - started_at).total_seconds() * 1000),
        }
        await self._nc.publish("tickets.completed", json.dumps(result).encode())
        if self._redis:
            try:
                await self._redis.lpush("tickets:history", json.dumps(result))
                await self._redis.ltrim("tickets:history", 0, 49)
            except Exception as exc:
                logger.warning("ticket history save failed: %s", exc)
        logger.info("completed ticket_id=%s outcome=%s latency_ms=%d",
                    ticket_id, outcome_type, result["latency_ms"])
        return result


# ── payload construction ───────────────────────────────────────────────────────

def _build_payload(step_name: str, ctx: dict) -> dict:
    """Map a step name + current pipeline context to the JSON payload for that agent."""
    tid = ctx["ticket_id"]

    if step_name == "classify":
        return {
            "ticket_id": tid,
            "title": ctx["title"],
            "description": ctx["description"],
        }

    if step_name == "find_solution":
        return {
            "ticket_id": tid,
            "category": ctx.get("category"),
            "title": ctx["title"],
            "description": ctx["description"],
        }

    if step_name == "generate_response":
        return {
            "ticket_id": tid,
            "category": ctx.get("category"),
            "priority": ctx.get("priority"),
            "articles": ctx.get("articles", []),
            "title": ctx["title"],
        }

    if step_name == "escalate":
        return {
            "ticket_id": tid,
            "category": ctx.get("category"),
            "priority": ctx.get("priority"),
            "reason": ctx.get("escalation_reason", "no_solution_found"),
            "attempts": ctx.get("attempts", 1),
        }

    if step_name == "llm_enhance":
        return {
            "ticket_id": tid,
            "title": ctx["title"],
            "description": ctx.get("description", ""),
            "category": ctx.get("category"),
            "kb_articles": ctx.get("articles", []),
            "draft_response": ctx.get("response_text", ""),
        }

    return {"ticket_id": tid}


# ── entry point ────────────────────────────────────────────────────────────────

async def _run() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )

    use_auction = os.getenv("USE_AUCTION", "false").lower() in ("1", "true", "yes")
    if use_auction:
        from auction import AuctionOrchestrator
        orch_class: type[AgentOrchestrator] = AuctionOrchestrator
        logger.info("dispatch mode: auction")
    else:
        orch_class = AgentOrchestrator
        logger.info("dispatch mode: broadcast")

    # Start the autoscaler as a background task.
    scaler: AgentScaler | None = None
    scaler_task: asyncio.Task | None = None
    try:
        scaler = AgentScaler()
        scaler_task = asyncio.create_task(
            scaler.monitor_queue_depth(), name="agent-scaler"
        )
        logger.info("scaler background task started")
    except Exception as exc:
        logger.warning("scaler init failed (docker unavailable?): %s", exc)

    try:
        async with orch_class() as orch:
            result = await orch.process_ticket(
                ticket_id="DEMO-001",
                title="VPN not connecting",
                description="Cannot connect to company VPN since this morning, tried restarting",
            )
            print(json.dumps(result, indent=2))

            # Keep the process alive so the scaler continues to monitor.
            try:
                await asyncio.get_running_loop().create_future()
            except asyncio.CancelledError:
                pass
    finally:
        if scaler_task is not None:
            scaler_task.cancel()
            await asyncio.gather(scaler_task, return_exceptions=True)
        if scaler is not None:
            await scaler.stop_all_extra()
        logger.info("orchestrator shutdown complete")


if __name__ == "__main__":
    asyncio.run(_run())

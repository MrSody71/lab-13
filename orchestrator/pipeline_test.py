"""Integration tests for Pipeline, PipelineStep, and _build_payload.

Pipeline has no external I/O, so these tests run without a live NATS/Redis.
Run with:  pytest orchestrator/ --asyncio-mode=auto
"""
from __future__ import annotations

import sys
import types

import pytest

# auction_test.py installs a partial pipeline stub into sys.modules before us
# (alphabetical collection order). Evict it so we import the real module.
sys.modules.pop("pipeline", None)
from pipeline import Pipeline, PipelineStep  # noqa: E402


# ── PipelineStep ───────────────────────────────────────────────────────────────

class TestPipelineStep:
    def test_required_fields(self) -> None:
        step = PipelineStep("classify", "tickets.classify", "tickets.classified")
        assert step.name == "classify"
        assert step.publish_subject == "tickets.classify"
        assert step.reply_subject == "tickets.classified"

    def test_defaults(self) -> None:
        step = PipelineStep("x", "pub.x", "reply.x")
        assert step.timeout == 30.0
        assert step.max_retries == 3

    def test_custom_timeout_and_retries(self) -> None:
        step = PipelineStep("x", "pub.x", "reply.x", timeout=5.0, max_retries=1)
        assert step.timeout == 5.0
        assert step.max_retries == 1


# ── Pipeline ───────────────────────────────────────────────────────────────────

class TestPipeline:
    @pytest.mark.asyncio
    async def test_empty_pipeline_returns_context_unchanged(self) -> None:
        async def runner(step: PipelineStep, ctx: dict) -> dict:
            return {}  # pragma: no cover — never called

        ctx = {"ticket_id": "T-001", "title": "wifi down"}
        result = await Pipeline().run(ctx, runner)
        assert result == ctx

    @pytest.mark.asyncio
    async def test_steps_run_in_order(self) -> None:
        order: list[str] = []

        async def runner(step: PipelineStep, ctx: dict) -> dict:
            order.append(step.name)
            return {}

        pipeline = (
            Pipeline()
            .add_step(PipelineStep("classify", "p.c", "r.c"))
            .add_step(PipelineStep("find_solution", "p.f", "r.f"))
        )
        await pipeline.run({"ticket_id": "T-001"}, runner)
        assert order == ["classify", "find_solution"]

    @pytest.mark.asyncio
    async def test_context_merges_between_steps(self) -> None:
        async def runner(step: PipelineStep, ctx: dict) -> dict:
            if step.name == "classify":
                return {"category": "network", "priority": "critical"}
            assert ctx["category"] == "network"
            assert ctx["priority"] == "critical"
            return {"found": False}

        pipeline = (
            Pipeline()
            .add_step(PipelineStep("classify", "p.c", "r.c"))
            .add_step(PipelineStep("find_solution", "p.f", "r.f"))
        )
        result = await pipeline.run({"ticket_id": "T-002"}, runner)
        assert result["category"] == "network"
        assert result["found"] is False

    @pytest.mark.asyncio
    async def test_original_context_not_mutated(self) -> None:
        async def runner(step: PipelineStep, ctx: dict) -> dict:
            return {"injected": True}

        original = {"ticket_id": "T-003"}
        pipeline = Pipeline().add_step(PipelineStep("x", "p.x", "r.x"))
        await pipeline.run(original, runner)
        assert "injected" not in original

    def test_add_step_returns_self_for_chaining(self) -> None:
        p = Pipeline()
        s = PipelineStep("x", "p.x", "r.x")
        result = p.add_step(s)
        assert result is p

    def test_steps_property_returns_copy(self) -> None:
        p = Pipeline()
        p.add_step(PipelineStep("x", "p.x", "r.x"))
        steps = p.steps
        steps.clear()
        assert len(p.steps) == 1


# ── _build_payload ─────────────────────────────────────────────────────────────
# Stub heavy dependencies so main.py can be imported without live services.
# All stubs are registered before the import below.

for _mod in (
    "nats", "nats.aio", "nats.aio.client",
    "redis", "redis.asyncio",
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

for _mod_name, _attrs in (
    ("scaler", {"AgentScaler": type("AgentScaler", (), {})}),
    ("tracing", {"init_tracer": lambda: None}),
):
    _m = types.ModuleType(_mod_name)
    for _k, _v in _attrs.items():
        setattr(_m, _k, _v)
    sys.modules.setdefault(_mod_name, _m)

# main.py does `from dotenv import load_dotenv` and calls load_dotenv() at import
# time — the stub needs to be callable.
_dotenv = sys.modules["dotenv"]
_dotenv.load_dotenv = lambda *a, **kw: None  # type: ignore[attr-defined]

# main.py does `from opentelemetry.propagate import inject`
_otel_propagate = sys.modules["opentelemetry.propagate"]
_otel_propagate.inject = lambda *a, **kw: None  # type: ignore[attr-defined]

# main.py does `from opentelemetry.trace import StatusCode`
_otel_trace = sys.modules["opentelemetry.trace"]
_otel_trace.StatusCode = type("StatusCode", (), {"ERROR": "ERROR"})  # type: ignore[attr-defined]

sys.modules.setdefault("pipeline", __import__("pipeline"))

# auction_test.py also stubs sys.modules["main"] (only AgentOrchestrator, no
# _build_payload). Evict it so we import the real main.py below.
sys.modules.pop("main", None)
from main import _build_payload  # noqa: E402


class TestBuildPayload:
    def test_classify_payload(self) -> None:
        ctx = {"ticket_id": "T-10", "title": "VPN issue", "description": "can't connect"}
        payload = _build_payload("classify", ctx)
        assert payload == {"ticket_id": "T-10", "title": "VPN issue", "description": "can't connect"}

    def test_find_solution_payload(self) -> None:
        ctx = {
            "ticket_id": "T-10", "title": "VPN issue", "description": "can't connect",
            "category": "network", "priority": "critical",
        }
        payload = _build_payload("find_solution", ctx)
        assert payload["ticket_id"] == "T-10"
        assert payload["category"] == "network"
        assert "description" in payload

    def test_generate_response_payload(self) -> None:
        ctx = {
            "ticket_id": "T-10", "title": "VPN issue",
            "category": "network", "priority": "medium",
            "articles": [{"id": "KB-1"}],
        }
        payload = _build_payload("generate_response", ctx)
        assert payload["priority"] == "medium"
        assert payload["articles"] == [{"id": "KB-1"}]

    def test_escalate_payload(self) -> None:
        ctx = {
            "ticket_id": "T-10", "category": "network", "priority": "critical",
            "escalation_reason": "critical_priority", "attempts": 2,
        }
        payload = _build_payload("escalate", ctx)
        assert payload["reason"] == "critical_priority"
        assert payload["attempts"] == 2

    def test_llm_enhance_payload(self) -> None:
        ctx = {
            "ticket_id": "T-10", "title": "VPN issue", "description": "broken",
            "category": "network", "articles": [], "response_text": "Try rebooting",
        }
        payload = _build_payload("llm_enhance", ctx)
        assert payload["draft_response"] == "Try rebooting"
        assert payload["kb_articles"] == []

    def test_unknown_step_returns_ticket_id_only(self) -> None:
        ctx = {"ticket_id": "T-10", "title": "x"}
        payload = _build_payload("unknown_step", ctx)
        assert payload == {"ticket_id": "T-10"}

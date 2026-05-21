"""FastAPI dashboard for monitoring the Multi-Agent System."""

from __future__ import annotations

import asyncio
import json
import logging
import os
import uuid
from contextlib import asynccontextmanager
from datetime import datetime, timezone

import httpx
import nats
import redis.asyncio as aioredis
from dotenv import load_dotenv
from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import HTMLResponse
from fastapi.templating import Jinja2Templates
from pydantic import BaseModel, Field

load_dotenv()

_fmt = logging.Formatter("%(asctime)s %(levelname)s %(name)s %(message)s")
_console = logging.StreamHandler()
_console.setFormatter(_fmt)
_file = logging.FileHandler("dashboard.log")
_file.setFormatter(_fmt)
logging.basicConfig(level=logging.INFO, handlers=[_console, _file])
logger = logging.getLogger(__name__)

NATS_URL = os.getenv("NATS_URL", "nats://localhost:4222")
NATS_MONITOR_URL = os.getenv("NATS_MONITOR_URL", "http://nats:8222")
REDIS_URL = os.getenv("REDIS_URL", "redis://redis:6379")

_agents: dict[str, dict] = {}
_nc = None
_redis = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _nc, _redis
    _redis = aioredis.from_url(REDIS_URL, decode_responses=True)
    try:
        _nc = await nats.connect(
            NATS_URL,
            reconnect_time_wait=2,
            max_reconnect_attempts=-1,
        )
        await _nc.subscribe("agents.heartbeat", cb=_heartbeat_handler)
    except Exception as exc:
        logger.warning("NATS connect failed: %s", exc)
    yield
    if _nc and not _nc.is_closed:
        await _nc.drain()
    if _redis:
        await _redis.aclose()


app = FastAPI(lifespan=lifespan)
templates = Jinja2Templates(directory="templates")


async def _heartbeat_handler(msg) -> None:
    try:
        data = json.loads(msg.data)
        agent_id = data.get("agent_id")
        if agent_id:
            data["last_seen"] = datetime.now(timezone.utc).isoformat()
            _agents[agent_id] = data
    except Exception as exc:
        logger.debug("heartbeat parse error: %s", exc)


@app.get("/", response_class=HTMLResponse)
async def index(request: Request):
    return templates.TemplateResponse("index.html", {"request": request})


@app.get("/api/status")
async def get_status():
    nats_info: dict = {"status": "disconnected"}
    subjects: dict[str, int] = {}

    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            varz_resp = await client.get(f"{NATS_MONITOR_URL}/varz")
            varz = varz_resp.json()
            nats_info = {
                "status": "connected",
                "in_msgs": varz.get("in_msgs", 0),
                "out_msgs": varz.get("out_msgs", 0),
                "connections": varz.get("connections", 0),
                "uptime": varz.get("uptime", ""),
            }
            subsz_resp = await client.get(f"{NATS_MONITOR_URL}/subsz?subs=1")
            subsz = subsz_resp.json()
            for sub in subsz.get("subscriptions", []):
                subj = sub.get("subject", "")
                pending = sub.get("num_pending", 0)
                if subj and not subj.startswith("_"):
                    subjects[subj] = subjects.get(subj, 0) + pending
    except Exception as exc:
        nats_info = {"status": "error", "error": str(exc)}

    redis_stats: dict = {}
    try:
        if _nc and not _nc.is_closed:
            reply = await _nc.request("knowledge.stats", b"", timeout=3.0)
            redis_stats = json.loads(reply.data)
    except Exception as exc:
        logger.debug("knowledge.stats request failed: %s", exc)

    now = datetime.now(timezone.utc)
    active_agents = []
    for agent_id, data in list(_agents.items()):
        try:
            last_seen = datetime.fromisoformat(data.get("last_seen", ""))
            age = (now - last_seen).total_seconds()
        except (ValueError, TypeError):
            age = 9999.0
        active_agents.append({
            **data,
            "age_seconds": round(age, 1),
            "online": age < 15,
        })

    return {
        "nats": nats_info,
        "subjects": subjects,
        "redis_stats": redis_stats,
        "agents": active_agents,
    }


@app.get("/api/tickets")
async def get_tickets():
    if not _redis:
        return {"tickets": []}
    try:
        raw = await _redis.lrange("tickets:history", 0, 49)
        return {"tickets": [json.loads(t) for t in raw]}
    except Exception as exc:
        return {"tickets": [], "error": str(exc)}


class TicketInput(BaseModel):
    title: str = Field(..., min_length=1, max_length=200)
    description: str = Field(..., min_length=1, max_length=2000)


@app.post("/api/tickets")
async def submit_ticket(body: TicketInput):
    if not _nc or _nc.is_closed:
        raise HTTPException(status_code=503, detail="NATS not connected")
    ticket_id = f"MANUAL-{uuid.uuid4().hex[:8].upper()}"
    payload = {
        "ticket_id": ticket_id,
        "title": body.title,
        "description": body.description,
    }
    await _nc.publish("tickets.incoming", json.dumps(payload).encode())
    return {"ticket_id": ticket_id, "status": "submitted"}


@app.get("/api/metrics")
async def get_metrics():
    if not _redis:
        return {"processed_count": 0, "avg_latency_ms": 0, "error_rate": 0.0}
    try:
        raw = await _redis.lrange("tickets:history", 0, 49)
        tickets = [json.loads(t) for t in raw]
        if not tickets:
            return {"processed_count": 0, "avg_latency_ms": 0, "error_rate": 0.0}
        processed = len(tickets)
        latencies = [t["latency_ms"] for t in tickets if t.get("latency_ms") is not None]
        avg_latency = int(sum(latencies) / len(latencies)) if latencies else 0
        escalated = sum(1 for t in tickets if t.get("outcome_type") == "escalated")
        return {
            "processed_count": processed,
            "avg_latency_ms": avg_latency,
            "error_rate": round(escalated / processed, 3),
        }
    except Exception as exc:
        return {"processed_count": 0, "avg_latency_ms": 0, "error_rate": 0.0, "error": str(exc)}

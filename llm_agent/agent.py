"""LLM Agent — enhances IT support responses using the Anthropic API."""

from __future__ import annotations

import asyncio
import json
import logging
import os

import nats
from dotenv import load_dotenv

load_dotenv()
logger = logging.getLogger(__name__)

_SYSTEM_PROMPT = (
    "You are a helpful IT support agent. Improve the draft response to be more "
    "empathetic, clear, and actionable. Keep it under 150 words."
)

MODEL = "claude-sonnet-4-20250514"
MAX_TOKENS = 500


class LLMSupportAgent:
    def __init__(self, nats_url: str | None = None) -> None:
        self._nats_url = nats_url or os.getenv("NATS_URL", "nats://localhost:4222")
        self._nc = None
        api_key = os.getenv("ANTHROPIC_API_KEY", "")
        if api_key:
            import anthropic
            self._client = anthropic.Anthropic(api_key=api_key)
        else:
            self._client = None
            logger.warning("ANTHROPIC_API_KEY not set — will fall back to draft response")

    async def connect(self) -> None:
        self._nc = await nats.connect(self._nats_url)
        await self._nc.subscribe("tickets.llm_enhance", cb=self._handle)
        logger.info("connected nats_url=%s", self._nats_url)

    async def close(self) -> None:
        if self._nc:
            await self._nc.drain()

    async def __aenter__(self) -> LLMSupportAgent:
        await self.connect()
        return self

    async def __aexit__(self, *_) -> None:
        await self.close()

    async def _handle(self, msg) -> None:
        try:
            data = json.loads(msg.data)
        except json.JSONDecodeError:
            logger.warning("non-JSON message on tickets.llm_enhance")
            return

        ticket_id = data.get("ticket_id", "")
        title = data.get("title", "")
        category = data.get("category", "")
        draft = data.get("draft_response", "")
        kb_articles = data.get("kb_articles", [])

        enhanced, model_used = await asyncio.get_running_loop().run_in_executor(
            None, self._enhance, title, category, draft, kb_articles
        )

        result = {
            "ticket_id": ticket_id,
            "enhanced_response": enhanced,
            "model_used": model_used,
        }
        await self._nc.publish("tickets.llm_enhanced", json.dumps(result).encode())
        logger.info("llm enhanced ticket_id=%s model=%s", ticket_id, model_used)

    def _enhance(
        self, title: str, category: str, draft: str, kb_articles: list
    ) -> tuple[str, str]:
        if self._client is None:
            return draft, "fallback"

        kb_summary = "; ".join(
            a.get("title", str(a)) if isinstance(a, dict) else str(a)
            for a in kb_articles[:3]
        )
        user_prompt = (
            f"Ticket: {title}\n"
            f"Category: {category}\n"
            f"Draft: {draft}\n"
            f"KB context: {kb_summary}\n"
            f"Improve this response."
        )

        try:
            resp = self._client.messages.create(
                model=MODEL,
                max_tokens=MAX_TOKENS,
                system=_SYSTEM_PROMPT,
                messages=[{"role": "user", "content": user_prompt}],
            )
            text = resp.content[0].text
            logger.info(
                "anthropic usage input_tokens=%d output_tokens=%d",
                resp.usage.input_tokens,
                resp.usage.output_tokens,
            )
            return text, MODEL
        except Exception as exc:
            logger.error("anthropic api error: %s — using draft", exc)
            return draft, "fallback"


async def _run() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    async with LLMSupportAgent() as agent:  # noqa: F841
        logger.info("LLM agent ready")
        try:
            await asyncio.get_running_loop().create_future()
        except asyncio.CancelledError:
            pass


if __name__ == "__main__":
    asyncio.run(_run())

from __future__ import annotations

from dataclasses import dataclass
from typing import Awaitable, Callable


@dataclass
class PipelineStep:
    name: str
    publish_subject: str
    reply_subject: str
    timeout: float = 30.0
    max_retries: int = 3


# Async callable that executes one step given the step descriptor and current context
StepRunner = Callable[[PipelineStep, dict], Awaitable[dict]]


class Pipeline:
    """Chains PipelineSteps sequentially, threading a shared context dict.

    Transport is fully delegated to the caller-supplied StepRunner so Pipeline
    has no NATS dependency and is straightforward to test in isolation.
    """

    def __init__(self) -> None:
        self._steps: list[PipelineStep] = []

    def add_step(self, step: PipelineStep) -> Pipeline:
        """Append *step* and return self for fluent chaining."""
        self._steps.append(step)
        return self

    @property
    def steps(self) -> list[PipelineStep]:
        return list(self._steps)

    async def run(self, context: dict, runner: StepRunner) -> dict:
        """Run every step in order.

        Each step's returned dict is merged into *context* so later steps can
        read outputs produced by earlier ones.
        """
        ctx = dict(context)
        for step in self._steps:
            result = await runner(step, ctx)
            ctx.update(result)
        return ctx

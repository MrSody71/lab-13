"""Dynamic classifier scaling driven by NATS queue depth."""
from __future__ import annotations

import asyncio
import json
import logging
import os
import urllib.error
import urllib.request

import docker
import docker.errors

logger = logging.getLogger(__name__)

_SCALE_UP_THRESHOLD = 10    # pending > this  → add a replica
_SCALE_DOWN_THRESHOLD = 2   # pending < this AND replicas > 1 → remove oldest extra
_MAX_REPLICAS = 5
_MIN_REPLICAS = 1
_POLL_INTERVAL_SECS = 10


class AgentScaler:
    """Monitors NATS queue depth every 10 s and scales the classifier service.

    The one container started by docker-compose is the permanent base replica
    (always counted, never touched). Extra containers are tracked in
    _extra_containers (oldest first) so scale-down always removes the oldest.
    """

    def __init__(
        self,
        nats_monitor_url: str | None = None,
        nats_url: str | None = None,
        otel_endpoint: str | None = None,
        classifier_image: str | None = None,
        docker_network: str | None = None,
    ) -> None:
        self._nats_monitor_url = (
            nats_monitor_url
            or os.getenv("NATS_MONITOR_URL", "http://nats:8222")
        )
        self._nats_url = nats_url or os.getenv("NATS_URL", "nats://nats:4222")
        self._otel_endpoint = otel_endpoint or os.getenv(
            "OTEL_EXPORTER_OTLP_ENDPOINT", "http://jaeger:4318"
        )
        self._classifier_image = classifier_image or os.getenv(
            "CLASSIFIER_IMAGE", "lab13-mas-support-classifier"
        )
        self._docker_network = docker_network or os.getenv(
            "DOCKER_NETWORK", "lab13-mas-support_default"
        )
        self._docker = docker.from_env()
        # Container IDs of extras we started, oldest first.
        self._extra_containers: list[str] = []

    @property
    def replica_count(self) -> int:
        """1 base replica (docker-compose) plus any extras we started."""
        return 1 + len(self._extra_containers)

    # ── main monitoring loop ───────────────────────────────────────────────────

    async def monitor_queue_depth(self) -> None:
        """Poll NATS /subsz every 10 s and scale up or down as needed.

        Runs until the task is cancelled.
        """
        logger.info(
            "scaler: started poll_interval=%ds up_threshold=%d down_threshold=%d"
            " min=%d max=%d image=%s",
            _POLL_INTERVAL_SECS,
            _SCALE_UP_THRESHOLD,
            _SCALE_DOWN_THRESHOLD,
            _MIN_REPLICAS,
            _MAX_REPLICAS,
            self._classifier_image,
        )
        while True:
            await asyncio.sleep(_POLL_INTERVAL_SECS)
            try:
                pending = await self._get_pending_count()
                logger.debug(
                    "scaler: pending=%d replicas=%d", pending, self.replica_count
                )
                await self._evaluate(pending)
            except asyncio.CancelledError:
                raise
            except Exception as exc:
                logger.warning("scaler: monitor cycle error: %s", exc)

    # ── scaling decisions ──────────────────────────────────────────────────────

    async def _evaluate(self, pending: int) -> None:
        if pending > _SCALE_UP_THRESHOLD and self.replica_count < _MAX_REPLICAS:
            await self._scale_up(pending)
        elif pending < _SCALE_DOWN_THRESHOLD and self.replica_count > _MIN_REPLICAS:
            await self._scale_down(pending)

    async def _scale_up(self, pending: int) -> None:
        loop = asyncio.get_running_loop()
        container_id = await loop.run_in_executor(None, self._start_classifier)
        self._extra_containers.append(container_id)
        logger.info(
            "scaler: scale up  pending=%d → replicas=%d  new_container=%s",
            pending, self.replica_count, container_id[:12],
        )

    async def _scale_down(self, pending: int) -> None:
        oldest_id = self._extra_containers.pop(0)
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(None, self._stop_container, oldest_id)
        logger.info(
            "scaler: scale down pending=%d → replicas=%d  stopped=%s",
            pending, self.replica_count, oldest_id[:12],
        )

    # ── NATS monitoring (blocking helper runs in executor) ─────────────────────

    async def _get_pending_count(self) -> int:
        """Return total num_pending for 'tickets.classify' subscriptions."""
        url = f"{self._nats_monitor_url}/subsz?subs=1"
        loop = asyncio.get_running_loop()
        data: dict = await loop.run_in_executor(None, _fetch_json, url)
        pending = 0
        for sub in data.get("subscriptions", []):
            if sub.get("subject") == "tickets.classify":
                pending += sub.get("num_pending", 0)
        return pending

    # ── Docker operations (blocking; always run in executor) ───────────────────

    def _start_classifier(self) -> str:
        """Start one extra classifier container; returns its full container ID."""
        container = self._docker.containers.run(
            image=self._classifier_image,
            environment={
                "NATS_URL": self._nats_url,
                "OTEL_SERVICE_NAME": "classifier-agent",
                "OTEL_EXPORTER_OTLP_ENDPOINT": self._otel_endpoint,
            },
            network=self._docker_network,
            labels={
                "managed_by": "agent_scaler",
                "service": "classifier",
            },
            detach=True,
        )
        return container.id

    def _stop_container(self, container_id: str) -> None:
        """Stop and remove a container; tolerates already-gone containers."""
        try:
            container = self._docker.containers.get(container_id)
            container.stop(timeout=10)
            container.remove()
        except docker.errors.NotFound:
            logger.warning(
                "scaler: container %s already gone, skipping stop",
                container_id[:12],
            )
        except Exception as exc:
            logger.warning(
                "scaler: error stopping container %s: %s", container_id[:12], exc
            )

    # ── shutdown ───────────────────────────────────────────────────────────────

    async def stop_all_extra(self) -> None:
        """Stop and remove every extra container started by this scaler instance."""
        if not self._extra_containers:
            return
        loop = asyncio.get_running_loop()
        for cid in list(self._extra_containers):
            await loop.run_in_executor(None, self._stop_container, cid)
        self._extra_containers.clear()
        logger.info("scaler: all extra containers stopped and removed")


# ── module-level helpers ───────────────────────────────────────────────────────

def _fetch_json(url: str) -> dict:
    """Blocking HTTP GET that returns parsed JSON; raises on non-200 or error."""
    with urllib.request.urlopen(url, timeout=5) as resp:
        return json.loads(resp.read())

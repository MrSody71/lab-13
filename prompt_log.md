# Prompt Log — Lab 13

## 2026-05-20

**Prompt:** Create a new project called lab13-mas-support with the full directory
structure (agents/, orchestrator/, api/, llm_agent/, dashboard/, .github/workflows/),
a docker-compose.yml with NATS, Redis, and Jaeger, a root .gitignore for Go and
Python, a README.md with placeholder sections, and this prompt_log.md.

**Result:** Project scaffolded; git repository initialized.

---

**Prompt:** Implement all four Go agents — classifier, knowledge, responder, and
escalation — each in their own subdirectory under agents/. Each agent should connect
to NATS, subscribe to its input subject, process messages, and publish results to its
output subject. Add Dockerfiles and go.mod files. Include unit tests.

**Result:** Four Go agents written with NATS pub/sub, structured slog logging,
Dockerfiles, go.mod with nats.go dependency, and table-driven unit tests. Subjects:
tickets.classify → tickets.classified, tickets.find_solution → tickets.solution_found,
tickets.generate_response → tickets.response_ready, tickets.escalate → tickets.escalated.

---

**Prompt:** Add a Python orchestrator in orchestrator/ that connects to NATS and
drives a multi-step pipeline: classify → find_solution → generate_response (or escalate
if no solution or critical priority) → publish tickets.completed. Include retry logic
with configurable timeout and a Pipeline abstraction for chaining steps.

**Result:** orchestrator/main.py with AgentOrchestrator class, pipeline.py with
Pipeline/PipelineStep dataclasses, configurable step_timeout and max_retries,
branching logic for escalation path, and tickets.completed broadcast at the end.

---

**Prompt:** Add distributed tracing with OpenTelemetry to all four Go agents and the
Python orchestrator. Propagate trace context via NATS message headers. Export traces
to Jaeger via OTLP/HTTP. Update docker-compose.yml to include the Jaeger all-in-one
image and wire OTEL_EXPORTER_OTLP_ENDPOINT into each service's environment.

**Result:** tracing.go added to each Go agent (OTLP exporter + resource setup),
tracing.py added to orchestrator, W3C TraceContext injected/extracted via NATS headers,
jaeger service added to docker-compose.yml on port 16686 with COLLECTOR_OTLP_ENABLED=true.

---

**Prompt:** Make the knowledge agent stateful by adding Redis persistence. Cache KB
lookup results with a TTL so repeated queries for the same category are served from
cache. Restore and log cache stats on startup. Expose a knowledge.stats NATS request
subject that replies with hit/miss counts.

**Result:** store.go with Redis-backed cache using HSET/HGET/EXPIRE, cache hit/miss
counters stored as Redis hashes, RestoreAndLogStats on startup, knowledge.stats reply
handler added to main.go. REDIS_URL wired into knowledge service in docker-compose.yml.

---

**Prompt:** Add dynamic autoscaling for the classifier agent inside the orchestrator.
Poll the NATS /subsz HTTP endpoint every 10 seconds. If tickets.classify queue depth
exceeds 10, start an extra classifier container using the Docker SDK. If depth drops
below 2 and extra replicas exist, stop the oldest. Cap at 5 replicas and 1 minimum.

**Result:** orchestrator/scaler.py with AgentScaler class polling NATS monitoring API,
using docker-py to run/stop containers. Scale-up threshold 10, scale-down threshold 2,
max 5, min 1. AgentScaler started as a background asyncio task in main.py. Docker socket
mounted as a volume in docker-compose.yml.

---

**Prompt:** Replace the broadcast dispatch for the classify step with an auction
mechanism. The orchestrator sends a BidRequest to agents.bid_request. Each classifier
replies with a BidResponse (cost, capacity, eta_ms) to agents.bid_response.<task_id>.
The orchestrator waits 2 seconds, picks the winner by lowest cost/capacity score, and
delivers the task directly to agents.task.<agent_id>. Fall back to broadcast if no
bids arrive.

**Result:** orchestrator/auction.py with AuctionOrchestrator subclass overriding
_publish_task, select_winner pure function, 2-second bid collection window.
agents/classifier/bidding.go with BidRequest/BidResponse types, computeBid pure
function (queue-depth-weighted cost), and bid_request subscription in main.go.
USE_AUCTION env var toggles the mode at startup.

---

**Prompt:** Add an LLM agent in llm_agent/ that subscribes to tickets.llm_enhance,
calls the Anthropic API (claude-sonnet-4-20250514) to rewrite the draft support
response to be more empathetic and actionable, and publishes the result to
tickets.llm_enhanced. Fall back to the original draft when ANTHROPIC_API_KEY is not
set. Activate the step in the orchestrator when LLM_ENABLED=true.

**Result:** llm_agent/agent.py with LLMSupportAgent class, system prompt tuned for
IT support, anthropic SDK usage with graceful fallback, Dockerfile, requirements.txt.
Orchestrator updated with llm_enhance step conditional on LLM_ENABLED. llm_agent
service added to docker-compose.yml under the llm profile.

---

**Prompt:** Build a web monitoring dashboard in dashboard/ using FastAPI. It should
display live agent status (heartbeats from agents.heartbeat), NATS message counters
from /varz, Redis KB stats from knowledge.stats, and the last 50 processed tickets
from Redis. Add a form to submit new tickets. Serve on port 8080.

**Result:** dashboard/app.py with FastAPI lifespan, /api/status endpoint aggregating
NATS varz + agent heartbeats + Redis KB stats, /api/tickets GET/POST endpoints,
/api/metrics endpoint with avg latency and escalation rate. Jinja2 HTML template with
auto-refresh table views. dashboard service added to docker-compose.yml on port 8080.

---

## 2026-05-22

**Prompt:** Update README.md with full content: overview, Mermaid architecture diagram
(all agents, NATS subjects, orchestrator, Redis, Jaeger, Dashboard), agents table with
NATS subjects and descriptions, tasks completed list, how-to-run instructions with
prerequisites and curl example, environment variables table, and project structure tree.
Also update prompt_log.md with all prompts from the session.

**Result:** README.md fully rewritten with complete Mermaid flowchart, agents table,
8-task summary table, quick-start and curl examples, full env-var reference, and
annotated project tree. prompt_log.md updated with all session prompts and results.

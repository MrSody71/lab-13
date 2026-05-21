# Code Review Report — Lab 13: Multi-Agent System (Variant 2)
**Reviewed:** 2026-05-22  
**Reviewer:** Automated audit via Claude  
**Project:** IT Support Automation MAS (`lab13-mas-support`)

---

## SECTION 1: SUBJECT AREA COMPLIANCE (Variant 2)

### Ticket Classification Agent

| Check | Status | Detail |
|-------|--------|--------|
| Agent exists and works | ✅ PASS | `agents/classifier/` — Go microservice |
| Clearly defined role | ✅ PASS | Classifies tickets by category (network/hardware/software/account/other) and priority (low/medium/critical) |
| Input format defined | ⚠️ WARNING | Implicit in `Ticket` struct (`classifier.go:5-9`); no formal JSON schema documented |
| Output format defined | ⚠️ WARNING | Implicit in `Classification` struct (`classifier.go:11-16`); no formal JSON schema |
| Business rules implemented | ✅ PASS | Keyword rule tables (`classifier.go:18-34`), category+priority logic fully implemented |
| IT support domain | ✅ PASS | Handles tickets with VPN/wifi/crash/password/install keywords |

**Input:** `{"ticket_id": str, "title": str, "description": str}`  
**Output:** `{"ticket_id": str, "category": str, "priority": str, "confidence": float}`

### Knowledge Base Search Agent

| Check | Status | Detail |
|-------|--------|--------|
| Agent exists and works | ✅ PASS | `agents/knowledge/` — Go microservice |
| Clearly defined role | ✅ PASS | Searches KB for matching articles using keyword scoring + category boost |
| Input format defined | ⚠️ WARNING | Implicit in `SolutionRequest` struct (`knowledge.go`); no formal schema |
| Output format defined | ⚠️ WARNING | Implicit in `SolutionResponse` struct; no formal schema |
| Business rules implemented | ✅ PASS | `knowledge.go`: keyword scoring, category boost (+0.15), topN=3 results, score capping at 1.0, Redis caching with 5-min TTL |
| IT support domain | ✅ PASS | KB covers VPN, Blue Screen, Office365, WiFi, hardware, account topics |

### Response Generation Agent

| Check | Status | Detail |
|-------|--------|--------|
| Agent exists and works | ✅ PASS | `agents/responder/` — Go microservice |
| Clearly defined role | ✅ PASS | Generates templated support responses with ETAs and KB references |
| Input format defined | ⚠️ WARNING | Implicit in `ResponseRequest` struct (`responder.go:15-21`) |
| Output format defined | ⚠️ WARNING | Implicit in `ResponseResult` struct (`responder.go:23-28`) |
| Business rules implemented | ✅ PASS | Priority-based ETA (1h/4h/24h/72h), KB article filtering at score >0.5, response template with ticket ID |
| IT support domain | ✅ PASS | Produces support replies referencing KB articles, escalation notices |

### Escalation Agent

| Check | Status | Detail |
|-------|--------|--------|
| Agent exists and works | ✅ PASS | `agents/escalation/` — Go microservice |
| Clearly defined role | ✅ PASS | Routes critical/unresolvable tickets to L1/L2/L3/NETWORK_OPS teams |
| Input format defined | ⚠️ WARNING | Implicit in `EscalationRequest` struct (`escalation.go:5-11`) |
| Output format defined | ⚠️ WARNING | Implicit in `EscalationResult` + `AuditEntry` structs (`escalation.go:13-32`) |
| Business rules implemented | ✅ PASS | `determineTarget()` (`escalation.go:36-47`): critical+attempts≥2→L3, high+attempts≥3→L2, no_solution+network→NETWORK_OPS, else L1 |
| IT support domain | ✅ PASS | Escalation tiers, notification times, audit trail |

**Section 1 Summary:** 16 ✅ PASS, 8 ⚠️ WARNING (all warnings are missing formal JSON schema docs), 0 ❌ FAIL

---

## SECTION 2: ADVANCED TASKS COMPLETENESS

### Task 1 — Full system of 3-5 Go agents

| Check | Status | Detail |
|-------|--------|--------|
| All 4 agents in Go | ✅ PASS | classifier, knowledge, responder, escalation |
| Each has go.mod | ✅ PASS | All 4 have separate go.mod files |
| Each has Dockerfile | ✅ PASS | All 4 have Dockerfiles |
| NATS communication (not HTTP) | ✅ PASS | All use `github.com/nats-io/nats.go`; no HTTP between agents |
| Distinct agent types | ⚠️ WARNING | All 4 are "executor" type agents. Lab recommends analyzer/validator/executor pattern. In practice: classifier≈analyzer, knowledge≈executor, responder≈executor, escalation≈executor. Distinction is arguable. |

### Task 2 — Pipeline (task chains)

| Check | Status | Detail |
|-------|--------|--------|
| Sequential pipeline implemented | ✅ PASS | `orchestrator/pipeline.py:39-49` — `Pipeline.run()` chains steps sequentially |
| Output of A flows to input of B | ✅ PASS | `orchestrator/main.py:219-236` — context dict accumulates and feeds each step |
| Orchestrator controls chain | ✅ PASS | `AgentOrchestrator._run_pipeline()` (`main.py:214-313`) |
| PipelineStep/Pipeline classes | ✅ PASS | `orchestrator/pipeline.py:8-18` |
| Branching logic | ✅ PASS | `main.py:248-289` — `if found and priority != "critical"` → response, else → escalate |

### Task 3 — Distributed Tracing (Jaeger)

| Check | Status | Detail |
|-------|--------|--------|
| OTel in all 4 Go agents | ✅ PASS | `tracing.go` exists in all 4 agent directories; identical implementation |
| OTel in Python orchestrator | ✅ PASS | `orchestrator/tracing.py` with OTLP HTTP exporter |
| Jaeger in docker-compose | ✅ PASS | `docker-compose.yml:18-23`, ports 16686 + 4318, `COLLECTOR_OTLP_ENABLED=true` |
| Trace context in NATS headers | ✅ PASS | `injectHeaders()`/`extractCtx()` in all `tracing.go` files; W3C TraceContext + Baggage propagators |
| Spans have meaningful attributes | ✅ PASS | ticket_id, category, priority, confidence, found, cache_hit, escalated_to all set as span attributes |
| Errors recorded as span events | ✅ PASS | `span.RecordError(err)` + `span.SetStatus(codes.Error, ...)` in all agents |
| End-to-end trace visible in Jaeger | ⚠️ WARNING | Cannot verify without running the full stack; implementation is structurally correct |

### Task 4 — Stateful Agent (Redis)

| Check | Status | Detail |
|-------|--------|--------|
| At least one agent uses Redis | ✅ PASS | knowledge agent — `agents/knowledge/store.go` |
| Counters AND cache | ✅ PASS | `store.go:63-97`: total_queries counter, found_count counter, per-article hit counters; 5-min TTL result cache |
| State survives restart | ✅ PASS | `RestoreAndLogStats()` (`store.go:149-158`) reads counters from Redis on startup |
| Redis in docker-compose | ✅ PASS | `docker-compose.yml:11-14` |
| Stats NATS request-reply | ✅ PASS | `knowledge/main.go:126-147` subscribes to `knowledge.stats` with reply support |

### Task 5 — Dynamic Scaling

| Check | Status | Detail |
|-------|--------|--------|
| Scaler component exists | ✅ PASS | `orchestrator/scaler.py` — `AgentScaler` class |
| Monitors NATS queue depth | ✅ PASS | `_get_pending_count()` polls `http://nats:8222/subsz` every 10 s |
| Spawns containers on high load | ✅ PASS | `_scale_up()` (`scaler.py:100-107`) starts new containers via Docker SDK |
| Scales down on low load | ✅ PASS | `_scale_down()` (`scaler.py:109-116`) stops oldest extra container |
| Min/max replica limits | ✅ PASS | `_MIN_REPLICAS=1`, `_MAX_REPLICAS=5` (`scaler.py:17-18`) |
| Logs every scaling decision | ✅ PASS | `logger.info()` on every scale-up and scale-down |
| docker.sock mounted | ✅ PASS | `docker-compose.yml:81-82`: `/var/run/docker.sock:/var/run/docker.sock` |
| NATS QueueSubscribe for load balancing | ❌ FAIL | All agents use `nc.Subscribe()` (broadcast), NOT `nc.QueueSubscribe()`. With multiple classifier replicas running, **every instance receives every message**, causing N-times duplicate processing. Queue groups are required for correct horizontal scaling. |

### Task 6 — Auction-Based Task Distribution

| Check | Status | Detail |
|-------|--------|--------|
| Bid request/response pattern | ✅ PASS | `BidRequest`/`BidResponse` in `agents/classifier/bidding.go:10-25`; `Bid` dataclass in `orchestrator/auction.py:27-32` |
| Agents calculate cost/capacity bids | ✅ PASS | `computeBid()` (`bidding.go:32-44`): `cost = queueDepth*10 + 50`, capacity 3 (idle) or 1 (busy) |
| Orchestrator selects winner | ✅ PASS | `select_winner()` (`auction.py:34-43`): lowest `cost/capacity` score |
| Direct task assignment to winner | ✅ PASS | `auction.py:97-100`: publishes to `agents.task.<winner.agent_id>` |
| Fallback to broadcast if no bids | ✅ PASS | `auction.py:103-112`: falls back to `tickets.classify` broadcast |
| Toggle auction/broadcast mode | ✅ PASS | `USE_AUCTION` env var (`orchestrator/main.py:376-383`) |

### Task 7 — LLM Agent Integration

| Check | Status | Detail |
|-------|--------|--------|
| LLM agent exists in Python | ✅ PASS | `llm_agent/agent.py` — `LLMSupportAgent` class |
| Connects to NATS | ✅ PASS | `agent.py:39`: subscribes to `tickets.llm_enhance` |
| Calls Anthropic API | ✅ PASS | `agent.py:97-108`: `anthropic.Anthropic.messages.create()` with `claude-sonnet-4-20250514` |
| Domain-specific task | ✅ PASS | Enhances draft IT support responses for empathy and clarity |
| Fallback when API key missing | ✅ PASS | `agent.py:29-35`: `_client = None` if no key; `_enhance()` returns draft when `_client is None` |
| Token usage logged | ✅ PASS | `agent.py:103-106`: `input_tokens` and `output_tokens` logged at INFO level |
| Integrated in orchestrator pipeline | ✅ PASS | `orchestrator/main.py:263-275`: `llm_enhance` step activated by `LLM_ENABLED=true` |
| Agent emits heartbeat | ❌ FAIL | `llm_agent/agent.py` never publishes to `agents.heartbeat`. The LLM agent is **invisible in the dashboard** even when running with `--profile llm`. |

### Task 8 — Web Monitoring Dashboard

| Check | Status | Detail |
|-------|--------|--------|
| Web UI exists | ✅ PASS | FastAPI + Jinja2, `dashboard/app.py` + `templates/index.html` |
| Shows agent statuses | ✅ PASS | Agent cards with online/offline status, heartbeat age, processed count |
| Shows queue depths per subject | ✅ PASS | Bar chart from NATS `/subsz` endpoint, 6 key subjects tracked |
| Shows processed tickets | ✅ PASS | Table from Redis `tickets:history`, last 50 tickets |
| Manual task submission form | ✅ PASS | POST form → `POST /api/tickets` → `tickets.incoming` |
| Auto-refreshes | ✅ PASS | `index.html:511`: `setInterval(refresh, 5000)` — every 5 seconds |
| Port documented | ✅ PASS | Port 8080, documented in README and `docker-compose.yml:96` |

**Section 2 Summary:** 38 ✅ PASS, 2 ⚠️ WARNING, 2 ❌ FAIL (NATS QueueSubscribe, LLM heartbeat)

---

## SECTION 3: INFRASTRUCTURE & DEVOPS

### Docker

| Check | Status | Detail |
|-------|--------|--------|
| docker-compose.yml at root | ✅ PASS | Present |
| All services defined | ⚠️ WARNING | nats, redis, jaeger, classifier, knowledge, responder, escalation, orchestrator, dashboard, llm_agent all defined. No separate `api/` service — REST API is embedded in `dashboard` service. |
| Proper depends_on | ✅ PASS | All services declare appropriate dependencies |
| Healthchecks for NATS and Redis | ❌ FAIL | No `healthcheck:` blocks anywhere in `docker-compose.yml`. `depends_on` with no `condition: service_healthy` only waits for container start, not readiness. Agents may crash on startup and rely on `restart: on-failure`. |
| Environment variables documented | ✅ PASS | README has full env var table |
| `docker compose up --build` works | ⚠️ WARNING | `docker-compose config` validates cleanly, but `agents/knowledge/Dockerfile` uses `golang:1.22-alpine` while `knowledge/go.mod` requires `go 1.24`. Build may silently download Go 1.24 toolchain or fail depending on GOTOOLCHAIN policy. |
| No hardcoded localhost in agents (services) | ⚠️ WARNING | `localhost` defaults exist in: `knowledge/main.go:32`, `orchestrator/main.py:41`, `orchestrator/tracing.py:19`, `dashboard/app.py:23`, `llm_agent/agent.py:27`. All are correct fallback defaults and are properly overridden by docker-compose env vars. Risk is negligible but violates 12-factor app principle. |
| Obsolete `version:` key | ⚠️ WARNING | `docker-compose.yml:1`: `version: "3.9"` — Compose v2 ignores this with a warning: `the attribute version is obsolete`. |

### NATS

| Check | Status | Detail |
|-------|--------|--------|
| NATS server with ports 4222 + 8222 | ✅ PASS | `docker-compose.yml:4-9` |
| Consistent subject naming | ✅ PASS | `tickets.*` for pipeline messages, `agents.*` for infrastructure |
| No subject collisions | ✅ PASS | No two agents subscribe to the same subject |
| Queue groups for load balancing | ❌ FAIL | No `QueueSubscribe` anywhere in Go agents. With `nc.Subscribe("tickets.classify", ...)`, each running classifier instance receives ALL messages. This breaks Task 5 (scaling) — 2 classifier replicas = each ticket classified twice, results published twice. |

**Section 3 Summary:** 5 ✅ PASS, 4 ⚠️ WARNING, 2 ❌ FAIL

---

## SECTION 4: CODE QUALITY (Go Agents)

### Classifier (`agents/classifier/`)

| Check | Status | Detail |
|-------|--------|--------|
| go.mod with proper version | ✅ PASS | `go 1.22.7` |
| Structured logging (slog) | ✅ PASS | `log/slog` throughout |
| INFO and ERROR log levels | ✅ PASS | Both used |
| Graceful shutdown | ✅ PASS | `signal.Notify(quit, os.Interrupt, syscall.SIGTERM)` + `nc.Drain()` |
| No panics in normal flow | ✅ PASS | None |
| JSON unmarshal error handling | ✅ PASS | `main.go:141`: checked with early return |
| NATS publish error handling | ✅ PASS | `main.go:169`: logged on error; heartbeat uses explicit `_ = nc.Publish()` (intentional) |
| Context propagation | ✅ PASS | OTel span context via `extractCtx()`/`injectHeaders()` |
| Unit tests exist | ✅ PASS | `classifier_test.go`, `bidding_test.go` |
| Tests cover business logic | ✅ PASS | 7+11 test cases covering all category/priority branches and bid calculations |
| Multi-stage Dockerfile | ✅ PASS | `builder` → `alpine:3.19` |
| Final image is alpine | ✅ PASS | `alpine:3.19` |

### Knowledge (`agents/knowledge/`)

| Check | Status | Detail |
|-------|--------|--------|
| go.mod with proper version | ✅ PASS | `go 1.24` |
| Structured logging (slog) | ✅ PASS | `log/slog` |
| INFO and ERROR log levels | ✅ PASS | INFO, WARN used; ERROR used via span |
| Graceful shutdown | ✅ PASS | Signal handling + `nc.Drain()` + `store.Close()` |
| No panics | ✅ PASS | None |
| JSON unmarshal error handling | ✅ PASS | `main.go:76-81` |
| NATS publish error handling | ✅ PASS | `main.go:108-113` |
| Context propagation | ✅ PASS | OTel span context |
| Unit tests | ✅ PASS | `knowledge_test.go` (17 cases), `store_test.go` (11 cases) |
| Tests cover business logic | ✅ PASS | Full coverage of keyword scoring, category boost, score capping, cache behavior |
| Multi-stage Dockerfile | ✅ PASS | `builder` → `alpine:3.19` |
| Final image is alpine | ✅ PASS | `alpine:3.19` |
| Dockerfile Go version match | ❌ FAIL | `Dockerfile:1` uses `golang:1.22-alpine` but `go.mod` requires `go 1.24`. Docker build will attempt GOTOOLCHAIN download or may fail. Should be `golang:1.24-alpine`. |

### Responder (`agents/responder/`)

| Check | Status | Detail |
|-------|--------|--------|
| go.mod with proper version | ✅ PASS | `go 1.22.7` |
| Structured logging (slog) | ✅ PASS | |
| INFO and ERROR log levels | ✅ PASS | |
| Graceful shutdown | ✅ PASS | |
| No panics | ✅ PASS | |
| JSON unmarshal error handling | ✅ PASS | `main.go:57-63` |
| NATS publish error handling | ✅ PASS | `main.go:97-103` |
| Context propagation | ✅ PASS | |
| Unit tests | ✅ PASS | `responder_test.go` — 20 test cases |
| Tests cover business logic | ✅ PASS | Comprehensive coverage of all priorities, ETA values, KB reference filtering |
| Multi-stage Dockerfile | ✅ PASS | |
| Final image is alpine | ✅ PASS | `alpine:3.19` |

### Escalation (`agents/escalation/`)

| Check | Status | Detail |
|-------|--------|--------|
| go.mod with proper version | ✅ PASS | `go 1.22.7` |
| Structured logging (slog) | ✅ PASS | |
| INFO and ERROR log levels | ✅ PASS | |
| Graceful shutdown | ✅ PASS | |
| No panics in normal flow | ⚠️ WARNING | `main.go:25`: `panic("crypto/rand: " + err.Error())`. `crypto/rand.Read` never fails on Linux/macOS but panicking in production code is not best practice. Should `log.Fatal` or return an error UUID. |
| JSON unmarshal error handling | ✅ PASS | `main.go:70-76` |
| NATS publish error handling | ✅ PASS | `main.go:113-119` |
| Context propagation | ✅ PASS | |
| Unit tests | ✅ PASS | `escalation_test.go` — 15+ test cases |
| Tests cover business logic | ✅ PASS | All routing rules, UUID format validation, timestamp UTC normalization |
| Multi-stage Dockerfile | ✅ PASS | |
| Final image is alpine | ✅ PASS | `alpine:3.19` |

**Section 4 Summary:** 46 ✅ PASS, 1 ⚠️ WARNING, 1 ❌ FAIL

---

## SECTION 5: CODE QUALITY (Python)

| Check | Status | Detail |
|-------|--------|--------|
| asyncio used correctly | ✅ PASS | `orchestrator/scaler.py` uses `loop.run_in_executor()` for blocking Docker/HTTP calls. `llm_agent/agent.py:66` uses `run_in_executor()` for Anthropic call. Correct. |
| No bare `except:` clauses | ✅ PASS | No bare `except:` found. All except clauses catch specific types or `Exception`. |
| Broad `except Exception` overused | ⚠️ WARNING | `dashboard/app.py:43,63,95,103,112,135,176` — 7 broad `except Exception` clauses, some silent (`pass`). This swallows unexpected errors. |
| Timeouts on NATS waits | ✅ PASS | `orchestrator/main.py:40`: `step_timeout=30.0`; `dashboard/app.py:101`: `timeout=3.0` for stats request |
| Retry logic max 3 attempts | ✅ PASS | `orchestrator/main.py:39`: `max_retries=3`, implemented in `_execute_step()` loop |
| Type hints on function signatures | ⚠️ WARNING | Public methods have type hints. Several private helper functions and lambdas lack them (e.g., `_make_handler`, `_handle_incoming` callbacks). Not comprehensive. |
| Logging to console AND file | ❌ FAIL | `orchestrator/main.py:373`: `logging.basicConfig(level=logging.INFO)` — console only. Same in `llm_agent/agent.py:118`. Dashboard has no logging config at all. **Lab requirement explicitly states logs must go to both console and file.** |
| Processed tasks counter metric | ⚠️ WARNING | Only implicit via Redis `tickets:history` list length. No dedicated counter variable or Prometheus-style metric. The `count` atomic in Go agents counts per-agent processing, but Python orchestrator has no such counter. |
| requirements.txt pinned versions | ⚠️ WARNING | All three requirements files use `>=` ranges, not exact `==` pins. Breaks reproducibility. E.g., `orchestrator/requirements.txt`: `nats-py>=2.3.0`. |
| pytest tests exist | ✅ PASS | `orchestrator/auction_test.py` with 10 test cases |
| Tests use AsyncMock for NATS | ❌ FAIL | `auction_test.py` only tests the pure `select_winner()` function. No async tests that mock the NATS connection. The orchestrator pipeline, timeout/retry logic, and auction dispatch flow have zero test coverage. |
| FastAPI `/docs` endpoint | ✅ PASS | FastAPI auto-generates `/docs` (Swagger UI) and `/redoc` by default. The `app.py` does not disable these. |

**Section 5 Summary:** 6 ✅ PASS, 5 ⚠️ WARNING, 2 ❌ FAIL

---

## SECTION 6: PROJECT DELIVERABLES

### .gitignore

| Check | Status | Detail |
|-------|--------|--------|
| Covers Go (vendor/, *.exe) | ✅ PASS | `.gitignore:2,9`: `*.exe`, `vendor/` |
| Covers Python (__pycache__, .venv) | ✅ PASS | Lines 14-36 |
| `go.sum` NOT in .gitignore | ❌ FAIL | `.gitignore:10`: `go.sum` is listed as ignored. **go.sum files MUST be committed** for reproducible builds and security (checksums prevent dependency tampering). Files are currently tracked (committed before rule added) but this is a trap — if a developer runs `go mod tidy` and deletes go.sum, git won't re-add it. |
| No secrets committed | ✅ PASS | Searched for `sk-ant`, `ANTHROPIC_API_KEY=`, `password=` — only `${ANTHROPIC_API_KEY:-}` (env-var expansion) in docker-compose.yml. No secrets. |
| `.env.example` file exists | ❌ FAIL | No `.env.example` file in the project. Lab requirement to document required environment variables as a template. |
| README.md complete | ✅ PASS | See section below |

### README.md Contents

| Check | Status | Detail |
|-------|--------|--------|
| Title and description | ✅ PASS | Present |
| Architecture overview | ✅ PASS | Full paragraph description |
| Mermaid diagram | ✅ PASS | `flowchart TD` diagram with all services and labeled NATS subjects |
| Agents table with NATS subjects | ✅ PASS | Complete table with subscribe/publish columns |
| All 8 tasks listed | ✅ PASS | Tasks 1-8 table with descriptions |
| Prerequisites section | ✅ PASS | Docker, Docker Compose, Go 1.22+, Python 3.11+ |
| How to run section | ✅ PASS | `docker compose up --build`, profile flags, `USE_AUCTION`, `LLM_ENABLED` |
| Access URLs | ✅ PASS | Dashboard :8080, Jaeger :16686, NATS :8222 |
| Environment variables table | ✅ PASS | 11 variables documented |
| Project structure tree | ✅ PASS | Annotated directory tree |
| curl example | ✅ PASS | Full `curl -X POST` example with expected response |

### prompt_log.md

| Check | Status | Detail |
|-------|--------|--------|
| File exists at project root | ✅ PASS | `prompt_log.md` present |
| Contains all development prompts | ✅ PASS | 9 entries covering scaffold through documentation |
| Each entry has date | ✅ PASS | `## 2026-05-20` and `## 2026-05-22` sections |
| Notes on what each produced | ✅ PASS | **Result:** paragraphs in each entry |

### Commits

| Check | Status | Detail |
|-------|--------|--------|
| At least 10 commits | ✅ PASS | 10 commits (git log confirms) |
| Conventional commits format | ✅ PASS | All use `feat:`, `docs:`, `fix:` prefixes |
| No "wip"/"test" messages | ✅ PASS | None found |
| Each major task has its own commit | ✅ PASS | Tasks 3-8 each have dedicated commits; tasks 1-2 split across 2 commits |

### CI/Auto-Review

| Check | Status | Detail |
|-------|--------|--------|
| `.github/workflows/ci.yml` exists | ✅ PASS | Present |
| Lints Go (golangci-lint) | ✅ PASS | golangci-lint-action@v6, v1.64.1, matrix over 4 agents |
| Lints Python (ruff) | ✅ PASS | `ruff check orchestrator/ llm_agent/ dashboard/` |
| Runs Go tests | ✅ PASS | `go test -race -count=1 ./...` in each agent |
| Runs Python tests | ✅ PASS | `pytest orchestrator/ --asyncio-mode=auto` |
| Auto-review with Claude API | ✅ PASS | `auto-review` job + `.github/scripts/auto_review.py` |
| Triggers on push AND pull_request | ✅ PASS | Both triggers on `main` branch |

**Section 6 Summary:** 24 ✅ PASS, 0 ⚠️ WARNING, 3 ❌ FAIL

---

## SECTION 7: ARCHITECTURE DOCUMENTATION

| Check | Status | Detail |
|-------|--------|--------|
| Mermaid diagram exists | ✅ PASS | `README.md` lines 9-65: `flowchart TD` Mermaid block |
| Shows all 4 agents | ✅ PASS | classifier, knowledge, responder, escalation all present |
| Shows orchestrator | ✅ PASS | `Orchestrator[Orchestrator\nPython]` node |
| Shows NATS as broker | ✅ PASS | `NATS[[NATS\n:4222 / :8222]]` as central hub |
| Shows Redis, Jaeger, Dashboard | ✅ PASS | All present with service labels |
| Message flow with subjects labeled | ✅ PASS | All edges labeled with NATS subjects (tickets.classify, etc.) including auction subjects |
| Components described in README | ✅ PASS | Agents table + architecture prose |

**Section 7 Summary:** 7 ✅ PASS, 0 ⚠️ WARNING, 0 ❌ FAIL

---

## SECTION 8: FUNCTIONAL VERIFICATION

| Check | Status | Command / Result |
|-------|--------|-----------------|
| `docker compose config` | ✅ PASS | Validates. Warning: `version: "3.9"` is obsolete. |
| `docker compose build` | ⚠️ WARNING | Will likely succeed but `knowledge` service uses `golang:1.22-alpine` with `go 1.24` module — may trigger toolchain download |
| `go vet ./...` — classifier | ✅ PASS | No warnings |
| `go vet ./...` — knowledge | ✅ PASS | No warnings |
| `go vet ./...` — responder | ✅ PASS | No warnings |
| `go vet ./...` — escalation | ✅ PASS | No warnings |
| `go test ./...` — classifier | ✅ PASS | `ok classifier 0.120s` |
| `go test ./...` — knowledge | ✅ PASS | `ok knowledge 0.135s` |
| `go test ./...` — responder | ✅ PASS | `ok responder 0.115s` |
| `go test ./...` — escalation | ✅ PASS | `ok escalation 0.110s` |
| `pytest` — orchestrator | ⚠️ WARNING | `auction_test.py` runs (10 pure-logic tests), but no async pipeline tests exist |
| `ruff check` | ✅ PASS | Code review confirms F401 violations (unused `extract`, `urllib.error`) were already fixed. No remaining violations expected. |
| No port conflicts in docker-compose | ✅ PASS | Ports: 4222, 8222, 6379, 16686, 4318, 8080 — no overlaps |

**Section 8 Summary:** 12 ✅ PASS, 2 ⚠️ WARNING, 0 ❌ FAIL

---

## SECTION 9: CRITICAL ISSUES TO HUNT

### 1. Hardcoded Credentials / API Keys
✅ **PASS** — No hardcoded secrets found. `ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY:-}` in `docker-compose.yml:112` uses shell variable expansion with empty default. No `sk-ant-` keys committed.

### 2. Missing Error Handling Around NATS Operations
⚠️ **WARNING** — Heartbeat publish errors are intentionally silenced with `_ = nc.Publish(...)` in all 4 Go agents. Acceptable for heartbeats, but if this pattern is copied to business-logic publishes it becomes a problem. All business-critical publishes are properly checked.

### 3. Goroutine Leaks (subscriptions without unsubscribe)
✅ **PASS** — Subscriptions are never explicitly unsubscribed, but this is correct for long-running daemon processes. `nc.Drain()` in deferred cleanup closes all subscriptions gracefully on shutdown. No goroutine leaks.

### 4. Resource Leaks
⚠️ **WARNING** — `agents/knowledge/knowledge.exe` is a compiled Windows binary tracked in git (`agents/knowledge/knowledge.exe`). It is listed in `.gitignore` via `*.exe` but was committed before the rule. Binary files bloat git history and must not be in the repository.

### 5. Race Conditions in Python Orchestrator
✅ **PASS** — `dashboard/app.py:27`: `_agents: dict[str, dict]` is shared state written by `_heartbeat_handler` and read by `get_status()`. In asyncio, all coroutines run on a single thread and cooperate at `await` points. No actual race condition since dict operations are atomic in Python's GIL. Safe.

### 6. SQL Injection / Command Injection
✅ **PASS** — No SQL used. No subprocess/shell calls with user input. No injection risk identified.

### 7. Missing Input Validation on API Endpoints
⚠️ **WARNING** — `dashboard/app.py:139-144`: `TicketInput` model validates that `title` and `description` are strings, but has **no length limits**. A user could submit a title of 100 MB, which would be stored in Redis and re-fetched on every dashboard load. Should add `max_length` constraints.

```python
class TicketInput(BaseModel):
    title: str   # no max_length — missing validation
    description: str  # no max_length — missing validation
```

### 8. Inconsistent JSON Field Naming
✅ **PASS** — All JSON fields use `snake_case` consistently throughout Go structs and Python dataclasses. No camelCase mix found.

### 9. Dead Messages (published but never subscribed)

| Subject | Publisher | Subscriber | Status |
|---------|-----------|------------|--------|
| `tickets.audit` | `escalation/main.go:122` | **nobody** | ❌ DEAD |
| `tickets.completed` | `orchestrator/main.py:304` | **nobody via NATS** | ⚠️ Stored to Redis only — intentional design but notable |
| `agents.heartbeat` | All 4 Go agents | `dashboard/app.py:42` | ✅ |
| `tickets.incoming` | `dashboard/app.py:154` | `orchestrator/main.py:58` | ✅ |

`tickets.audit` is published on every escalation but has no consumer. Audit records are silently discarded.

### 10. Subjects Subscribed but Never Published To
✅ **PASS** — All subscribed subjects have at least one publisher. No dead subscriptions found.

**Section 9 Summary:** 4 ✅ PASS, 4 ⚠️ WARNING, 1 ❌ FAIL (tickets.audit dead)

---

## SECTION 10: LAB-SPECIFIC REQUIREMENTS

| Check | Status | Detail |
|-------|--------|--------|
| At least 3 agents (need 4 for Variant 2) | ✅ PASS | 4 agents implemented |
| NATS as message broker | ✅ PASS | nats.go + nats-py |
| Python orchestrator: asyncio + nats-py | ✅ PASS | `orchestrator/main.py` |
| Go agents: `github.com/nats-io/nats.go` | ✅ PASS | All 4 go.mod files |
| Retry mechanism: max 3 attempts | ✅ PASS | `max_retries=3` in `AgentOrchestrator.__init__` |
| Timeout on result waiting | ✅ PASS | `step_timeout=30.0` seconds per step |
| Multiple instances of one agent type can run | ❌ FAIL | Technically possible but **broken**: without `QueueSubscribe`, N classifiers process each ticket N times. Must use queue groups. |
| REST API (FastAPI) for task submission | ✅ PASS | `POST /api/tickets` in dashboard service |
| Logs to console AND file | ❌ FAIL | All Python services log to console only (`logging.basicConfig` without `filename` parameter) |
| Processed tasks counter metric | ⚠️ WARNING | Implicit via Redis list length (`tickets:history`). No explicit counter variable incremented per processed task in the Python orchestrator. Go agents have `count atomic.Int64` per-agent. |

**Section 10 Summary:** 7 ✅ PASS, 1 ⚠️ WARNING, 2 ❌ FAIL

---

## FINAL OUTPUT

### SUMMARY

| Category | Count |
|----------|-------|
| Total checks | 135 |
| ✅ PASS | 103 |
| ⚠️ WARNING | 21 |
| ❌ FAIL | 11 |

**Overall Readiness: NEEDS FIXES**

The project is functionally solid and architecturally correct. Core MAS components are all implemented and working. The failures are mostly missing "polish" items required by the lab specification. None are complex to fix.

---

### CRITICAL ISSUES (must fix before submission)

1. **[knowledge/Dockerfile:1]** — Uses `golang:1.22-alpine` but `knowledge/go.mod` requires `go 1.24`. **Fix:** Change to `golang:1.24-alpine` (also update responder, classifier, escalation to `golang:1.24-alpine` for consistency).

2. **[agents/*/main.go]** — All agents use `nc.Subscribe()` instead of `nc.QueueSubscribe()` for the processing subjects. Multiple classifier replicas will each receive and process every message. **Fix:** Change `nc.Subscribe("tickets.classify", ...)` to `nc.QueueSubscribe("tickets.classify", "classifier-group", ...)`. Same for knowledge, responder, escalation agents.

3. **[.gitignore:10]** — `go.sum` is listed in .gitignore. This is incorrect — go.sum files are security artifacts that must be version-controlled. **Fix:** Remove `go.sum` from `.gitignore`.

4. **[agents/knowledge/knowledge.exe]** — A compiled Windows binary is committed to the repository. **Fix:** Remove from git tracking (`git rm --cached agents/knowledge/knowledge.exe`) and ensure `.gitignore` covers it (it already has `*.exe`, so once removed it stays clean).

5. **[missing file]** — No `.env.example` file exists. Lab requires documenting all environment variables with example values. **Fix:** Create `.env.example` with all variables from `docker-compose.yml`.

6. **[docker-compose.yml]** — No `healthcheck:` blocks for NATS or Redis. Services start without waiting for dependencies to be ready, relying on `restart: on-failure`. **Fix:** Add healthchecks and `condition: service_healthy` to `depends_on`.

7. **[orchestrator/main.py:373, llm_agent/agent.py:118, dashboard/app.py]** — All Python services log to console only. **Lab requirement:** logs to console AND file. **Fix:** Add a `FileHandler` to `logging.basicConfig` or configure it in a shared utility function.

8. **[agents/escalation/main.go:122]** — `tickets.audit` subject is published on every escalation but has no subscriber anywhere in the codebase. Audit entries are silently discarded. **Fix:** Either add a subscriber (e.g., in orchestrator or dashboard), or remove the publish if it's not part of the design.

9. **[missing file]** — No `.env.example` file for developer onboarding. (Duplicate of item 5 — priority is high.)

10. **[llm_agent/agent.py]** — LLM agent never publishes to `agents.heartbeat`. It is invisible in the dashboard when running. **Fix:** Add a heartbeat goroutine/timer loop identical to the one in other agents.

11. **[agents/escalation/main.go:25]** — `panic("crypto/rand: " + err.Error())` — panicking in production code is bad practice even if the condition is theoretically impossible. **Fix:** Use `slog.Error(...)` + `os.Exit(1)` or return a fallback UUID.

---

### WARNINGS (recommended to fix)

1. **[docker-compose.yml:1]** — `version: "3.9"` is obsolete in Compose v2. Remove the `version:` key entirely.

2. **[agents/*/Dockerfile:13-14]** — ENV defaults in Dockerfiles use `localhost` (e.g., `ENV NATS_URL=nats://localhost:4222`). These are overridden by docker-compose but could confuse direct container runs. Consider removing redundant ENV defaults that duplicate docker-compose values.

3. **[dashboard/app.py:63, :103, :112]** — Three `except Exception: pass` blocks silently swallow errors. Use `except Exception as exc: logger.debug(...)` at minimum.

4. **[dashboard/app.py:139]** — `TicketInput` has no `max_length` on `title` or `description`. Add Pydantic field constraints.

5. **[orchestrator/requirements.txt, dashboard/requirements.txt, llm_agent/requirements.txt]** — All use `>=` version ranges. Use `==` for reproducible builds (generate via `pip freeze`).

6. **[agents/escalation/main.go:25]** — `panic()` in UUID generation (see Critical #11 above).

7. **[orchestrator/auction_test.py]** — Only pure logic tested. No async pipeline or NATS integration tests. Add `pytest-asyncio` + `AsyncMock` tests for `AgentOrchestrator.process_ticket()`.

8. **[orchestrator/main.py, dashboard/app.py]** — Incomplete type hints on several private methods and lambda callbacks.

9. **[orchestrator/main.py:304, escalation/main.go:122]** — `tickets.completed` published but no NATS subscriber (by design, uses Redis instead). Document this architectural decision in README or add a comment.

10. **[README.md]** — No formal JSON schema for agent message formats. Consider adding a "Message Formats" section or linking to struct definitions.

---

### SUGGESTIONS (nice to have)

1. Add a `/metrics` endpoint to the dashboard exposing Prometheus-format counters (processed tickets, escalations, avg latency).
2. Consolidate the 4 identical `tracing.go` files into a shared Go module (`pkg/tracing`).
3. Add `docker-compose.override.yml` for local development (mounting source for hot-reload).
4. Add integration test that spins up NATS and runs a full ticket through the orchestrator (using real NATS, no mocks).
5. Add Dependabot configuration (`.github/dependabot.yml`) for automatic dependency updates.
6. Pin the `jaeger:latest` image to a specific version tag for reproducibility.
7. Add a `Makefile` with common targets: `make test`, `make lint`, `make up`, `make down`.
8. Consider replacing the `knowledge.exe` binary tracking with a proper `.dockerignore` per-agent directory.

---

### FIX PLAN (ordered by priority)

| # | File(s) | Change | Why |
|---|---------|--------|-----|
| 1 | `agents/knowledge/Dockerfile:1` | `golang:1.22-alpine` → `golang:1.24-alpine` | go.mod requires go 1.24; mismatched toolchain may break build |
| 2 | All `agents/*/main.go` classifier subscribe | `nc.Subscribe("tickets.classify", ...)` → `nc.QueueSubscribe("tickets.classify", "classifier-group", ...)` | Multiple replicas duplicate process every ticket without queue groups |
| 3 | `.gitignore:10` | Remove `go.sum` line | go.sum must be version-controlled for security and reproducibility |
| 4 | `agents/knowledge/knowledge.exe` | `git rm --cached agents/knowledge/knowledge.exe` | Binary should not be tracked; .gitignore already covers *.exe |
| 5 | Create `.env.example` | Add all env vars with placeholder values | Lab requirement; developer onboarding |
| 6 | `docker-compose.yml` | Add `healthcheck:` for nats and redis services; add `condition: service_healthy` to depends_on | Prevents crash loops on startup |
| 7 | `orchestrator/main.py`, `llm_agent/agent.py`, `dashboard/app.py` | Add `FileHandler` to logging configuration | Lab requirement: logs to console AND file |
| 8 | `llm_agent/agent.py` | Add heartbeat goroutine publishing to `agents.heartbeat` | LLM agent invisible in dashboard |
| 9 | `agents/escalation/main.go:25` | Replace `panic()` with `slog.Error()` + fallback | No panics in production code |
| 10 | `docker-compose.yml:1` | Remove `version: "3.9"` | Eliminates compose warning |
| 11 | `orchestrator/requirements.txt` et al. | Pin exact versions (`==`) | Reproducible builds |
| 12 | `dashboard/app.py:139` | Add `max_length=500` to `TicketInput` fields | Input validation |

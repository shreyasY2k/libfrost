# Enterprise LLM Stack: Full Implementation Tracker

## Phase 1: Infrastructure & Keycloak Setup ✅
- [x] 1.1 Docker Compose stack (docker-compose.yml, init-db.sql)
- [x] 1.2 Keycloak realm configuration (keycloak/realm-export.json)
- [x] 1.3 LibreChat OIDC config (.env.librechat, librechat.yaml)
- [x] 1.4 Bifrost enterprise config (config/bifrost-config.json)

## Phase 2: User Management Plugin (Bifrost) ✅
- [x] 2.1 Plugin scaffold (main.go, go.mod)
- [x] 2.2 JWT validation (jwt.go)
- [x] 2.3 Keycloak Admin API client (keycloak.go)
- [x] 2.4 DB tables (tables.go)
- [x] 2.5 Workspace store & resolution (workspace.go)
- [x] 2.6 REST API endpoints (api.go)
- [x] 2.7 KC sync worker (sync.go)

## Phase 3: LibreChat Frontend Integration ✅
- [x] 3.1 WorkspaceSelector component
- [x] 3.2 useWorkspace hook
- [x] 3.3 bifrostHeaders utility
- [x] 3.4 Nav integration
- [x] 3.5 Token forwarding middleware

---

## Phase 4: Guardrails Plugin (Bifrost)

**Goal:** Content filtering with global + workspace-specific guardrails.
**Location:** `bifrost/plugins/guardrails/`

### Files to create:
- [ ] 4.1 `main.go` — Plugin init, PreLLMHook (request filtering), PostLLMHook (response filtering)
- [ ] 4.2 `definitions.go` — Types: GuardrailType (pii/content/regex/keyword/max_tokens), GuardrailPhase (pre/post/both), GuardrailAction (block/redact/warn/log), GuardrailScope (global/workspace)
- [ ] 4.3 `executor.go` — Guardrail execution engine: loads applicable rules, runs filters in priority order
- [ ] 4.4 `filters/pii.go` — PII detection (email, phone, SSN, credit card regex patterns)
- [ ] 4.5 `filters/content.go` — Content filtering (keyword blocklist, toxicity patterns)
- [ ] 4.6 `filters/custom.go` — Custom regex/keyword filters
- [ ] 4.7 `store.go` — In-memory cache + DB for guardrail rules, CRUD operations
- [ ] 4.8 `api.go` — REST API: guardrail CRUD, workspace assignment
- [ ] 4.9 `go.mod` — Module definition

### DB Table: `enterprise_guardrails`
- id, name, description, type, phase, scope (global/workspace), workspace_id (nullable)
- action (block/redact/warn/log), config (JSON), enabled, priority
- is_inherited (global guardrails auto-apply to all workspaces)

### Hook Flow:
- **PreLLMHook:** Get workspace_id from context → load guardrails (global + workspace-specific, phase=pre or both) → execute in priority order → block/redact/warn/log
- **PostLLMHook:** Same for response content, phase=post or both → can redact response or log warnings

### Verification:
- [ ] PII in request → blocked/redacted
- [ ] Global guardrail applies to all workspaces
- [ ] Workspace-specific guardrail only applies in that workspace
- [ ] Guardrails execute in priority order
- [ ] block/redact/warn/log actions all work correctly

---

## Phase 5: Audit Plugin (Bifrost)

**Goal:** Per-entity audit logging with drill-down dashboard API.
**Location:** `bifrost/plugins/audit/`

### Files to create:
- [ ] 5.1 `main.go` — Plugin init, implements ObservabilityPlugin.Inject() for async audit logging
- [ ] 5.2 `tables.go` — TableAuditLog with org/team/workspace/user IDs + request metadata
- [ ] 5.3 `store.go` — Async batch writer to PostgreSQL (batches writes for performance)
- [ ] 5.4 `api.go` — Dashboard REST endpoints (paginated search, stats, histogram, cost breakdown)
- [ ] 5.5 `aggregator.go` — Metrics aggregation for dashboard queries (group by org/team/workspace/user)
- [ ] 5.6 `go.mod` — Module definition

### DB Table: `enterprise_audit_logs`
- id, trace_id, request_id
- org_id, org_name, team_id, team_name, workspace_id, workspace_name, user_id, user_email
- provider, model, request_type, tokens_used, cost, latency_ms
- status (success/error/blocked/guardrail_triggered), error_type, guardrails_triggered (JSON)
- timestamp (indexed for time-series)
- Composite indexes: (org_id, timestamp), (team_id), (workspace_id), (user_id)

### Dashboard API Endpoints:
- [ ] `GET /api/audit/logs` — Paginated search with filters (org, team, workspace, user, date range, status)
- [ ] `GET /api/audit/stats` — Aggregate stats by hierarchy level
- [ ] `GET /api/audit/histogram` — Time-series data for charts
- [ ] `GET /api/audit/cost-breakdown` — Cost breakdown by org/team/workspace/user

### Verification:
- [ ] Every request generates an audit entry (async, zero client latency)
- [ ] Dashboard API returns filtered, drillable results
- [ ] Cost breakdown matches actual usage
- [ ] Batch writer handles high throughput without data loss

---

## Phase 6: Credit Management & Production Hardening

**Goal:** Full credit management API, team budget caps, and production readiness.

### 6.1 Credit Management
- [ ] Admin REST API to adjust per-user credits (`PATCH /api/enterprise/users/{userID}/credits`)
- [ ] Team budget cap enforcement: sum of team member usage cannot exceed team's budget
- [ ] Auto-provisioning of default budgets on first request (already scaffolded in usermgmt plugin)
- [ ] Credit usage dashboard integration with audit plugin
- [ ] Budget reset tracking and notifications

### 6.2 JWKS Rotation & Security Hardening
- [ ] JWKS auto-refresh on signature validation failure (force re-fetch from Keycloak)
- [ ] Token revocation check via Keycloak introspection endpoint (optional, for high-security deployments)
- [ ] Rate limiting on JWT validation failures (prevent brute-force)
- [ ] Audit log for authentication failures

### 6.3 Database & Connection Hardening
- [ ] PostgreSQL connection pooling (PgBouncer or pgx pool config)
- [ ] Database migration versioning (use GORM AutoMigrate with version tracking)
- [ ] Graceful shutdown: flush audit batch writer, complete in-flight sync
- [ ] Connection retry with exponential backoff

### 6.4 Health Checks & Monitoring
- [ ] `/health` endpoint for each service in docker-compose
- [ ] Readiness probe: check Keycloak connectivity, DB connection, JWKS availability
- [ ] Liveness probe: basic health check
- [ ] Prometheus metrics for usermgmt plugin (jwt_validations_total, workspace_lookups_total, auth_failures_total)

### 6.5 Integration Tests
- [ ] End-to-end test: Keycloak login → LibreChat → Bifrost with JWT → model response
- [ ] JWT validation tests (valid, expired, wrong issuer, missing claims)
- [ ] Workspace access control tests (allowed, denied, workspace not found)
- [ ] Model access tests (allowed model, denied model, no restrictions)
- [ ] Budget enforcement tests (under budget, over budget, budget reset)
- [ ] Guardrails tests (PII detection, content filter, custom regex)
- [ ] Audit log completeness tests

### 6.6 Bifrost Admin UI (Next.js at `bifrost/ui/`)
- [ ] Audit dashboard page: filterable log viewer with drill-down by org/team/workspace/user
- [ ] Credit management page: view/adjust user budgets, team budget caps
- [ ] Guardrails management page: create/edit/delete guardrail rules, assign to workspaces
- [ ] Workspace management page: create/edit workspaces, assign users, configure model access

### Verification:
- [ ] Full stack boots with `docker-compose up` and all services healthy
- [ ] User logs in via Keycloak → selects workspace → chats with budget enforcement
- [ ] Admin can manage workspaces, credits, and guardrails via API
- [ ] All integration tests pass

---

## Architecture Reference

```
User → LibreChat (React + Node.js) → Keycloak v26 OIDC Login
  → JWT with org/group/workspace claims
  → User selects workspace in LibreChat UI
  → API request to Bifrost with: Authorization: Bearer <JWT>, X-Workspace-ID: <id>
  → Bifrost Plugin Pipeline:
     1. UserMgmt Plugin (HTTPTransportPreHook): JWT validation, claim extraction, workspace resolution
     2. Governance Plugin (existing): budget/rate-limit enforcement via UserGovernance
     3. Guardrails Plugin (PreLLMHook): content filtering, PII detection
     4. LLM Provider call
     5. Guardrails Plugin (PostLLMHook): response filtering
     6. Audit Plugin (ObservabilityPlugin.Inject): async audit log with full metadata
  → Response to LibreChat
```

## Plugin Registration Order (config/bifrost-config.json)
```json
{
  "plugins": [
    { "name": "usermgmt",   "enabled": true },
    { "name": "governance",  "enabled": true },
    { "name": "guardrails",  "enabled": true },
    { "name": "logging",     "enabled": true },
    { "name": "audit",       "enabled": true }
  ]
}
```
Order matters: usermgmt sets context → governance uses it → guardrails filter → logging records → audit captures everything.

## Key Files Reference

| Component | File | Purpose |
|-----------|------|---------|
| Plugin interfaces | `bifrost/core/schemas/plugin.go` | BasePlugin, HTTPTransportPlugin, LLMPlugin, ObservabilityPlugin |
| Governance plugin | `bifrost/plugins/governance/main.go` | Reference for hook patterns, budget evaluation |
| GovernanceStore | `bifrost/plugins/governance/store.go:93-153` | Interface with UserGovernance CRUD (lines 131-139) |
| BifrostContext | `bifrost/core/schemas/context.go` | SetValue/Value for passing metadata through pipeline |
| Context keys | `bifrost/core/schemas/bifrost.go:142-206` | All BifrostContextKey constants |
| Middleware pipeline | `bifrost/transports/bifrost-http/handlers/middlewares.go` | Auth, tracing, governance middleware chain |
| OIDC strategy | `LibreChat/api/strategies/openidStrategy.js` | Token handling, federatedTokens storage |
| Nav component | `LibreChat/client/src/components/Nav/Nav.tsx` | Sidebar with WorkspaceSelector integration |
| Config loading | `bifrost/transports/bifrost-http/lib/config.go` | How plugins are loaded and registered |

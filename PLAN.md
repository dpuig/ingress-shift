# Ingress Shift — Implementation Plan

**Phase 1 of the Tier 1 Roadmap**
**Timeline:** Weeks 1–10 (August – October 2026)
**Goal:** Three paid migrations delivered and a shipped analyzer by end of Q4 2026.

---

## 1. Strategic Context

### Why this product exists

Ingress-NGINX reached end-of-life in March 2026. The migration wave to Gateway API has roughly until mid-2027 before urgency premium evaporates. This is the only Tier 1 product with an expiry date — it goes first and gets protected capacity.

### Why this shape

`ingress2gateway` already translates the common cases. Competing on translation is a losing position. The unsolved problem is **proving the new data path behaves identically to the old one** before cutting over production traffic. Translation is the solved 80%; validation is the unsolved 20% that people actually pay for.

### Competitive positioning

- **Do not compete** on translation — `ingress2gateway` owns that.
- **Compete on** validation, orchestration, and the annotation knowledge base.
- **Contribute annotations upstream** to stay adjacent rather than adversarial.
- The free analyzer creates demand for remediation; the report is a problem statement, not a solution.

---

## 2. Architecture Constraints

Decided up front. Non-negotiable.

- **Language:** Go. Single static binary, no runtime dependencies.
- **Distribution:** `kubectl` plugin via krew (analyzer); standalone binary (harness + orchestrator).
- **Credentials:** Read-only cluster access. No write operations in the analyzer.
- **Execution model:** On-premises, no phone-home, no telemetry, air-gap capable.
- **Auditability:** Single auditable binary, signed releases.
- **Licensing split:** Analyzer is open source (lead generation). Shadow harness and cutover orchestrator are commercial (revenue).

> Design for the security review, not the demo. The single biggest sales blocker is the customer's security team, not their budget. Retrofitting security-first design costs a quarter.

---

## 3. Prerequisites (Phase 0 — Weeks 1–2)

These must be done before or in parallel with early Workstream A.

- [ ] Stand up shared Go module skeleton, CI, and release pipeline (single static binaries, build and signing infrastructure)
- [ ] Commit to open-source / commercial licensing split — analyzers free, validation and orchestration paid
- [ ] Write the data handling policy — what the tool reads, what it writes, what never leaves the customer's network (one page, paste into security questionnaires)
- [ ] Build the Ingress-NGINX landing page and publish the first technical post

**Gate:** Landing page live, one post published, build pipeline green.

---

## 4. Workstreams

### Workstream A — Annotation Coverage Analyzer

**Weeks 1–5 · Open source · `kubectl` plugin via krew**

The lead magnet. Read-only, free, and designed to generate pipeline for the commercial tools.

#### Deliverables

- [ ] Enumerate Ingress resources across all contexts and namespaces
- [ ] Map every annotation against a maintained knowledge base, classifying each as:
  - Direct Gateway API equivalent
  - Requires a Gateway API extension
  - No equivalent (manual intervention)
- [ ] Flag the annotation classes that break naive translation:
  - Auth snippets
  - Custom NGINX config blocks
  - Rewrite semantics
  - Session affinity
  - Rate limiting
  - mTLS passthrough
- [ ] Emit a scored report:
  - Percentage of resources directly translatable
  - List of manual interventions with effort estimate per item
  - Aggregate migration complexity score
- [ ] Recommend a target Gateway API controller with stated reasoning

#### The annotation knowledge base

> **This is the actual asset.** It's tedious, it compounds, and it's the part a competitor can't fork in a weekend.

- Treat as a versioned data file, not embedded code
- Test coverage per entry
- Version independently from the analyzer binary
- Every customer engagement adds entries — this is the compounding moat

#### Acceptance criteria

- Runs against a real cluster in < 60 seconds
- Installable via `kubectl krew install`
- Handles multi-context, multi-namespace clusters
- Output formats: human-readable terminal, JSON, and machine-parseable report

---

### Workstream B — Shadow & Diff Harness

**Weeks 4–8 · Commercial · Converts the free analyzer into revenue**

This is what customers pay for. The analyzer tells them they have a problem; this tool proves the fix works.

#### Deliverables

- [ ] Mirror live production traffic to both the incumbent ingress controller and the candidate Gateway API controller
- [ ] Diff responses across three dimensions:
  - HTTP status codes
  - Response headers
  - Normalized response bodies
- [ ] Classify divergences by severity (breaking, degraded, cosmetic, expected)
- [ ] Report parity percentage over a configurable soak window long enough to catch periodic traffic patterns
- [ ] Produce a **signed parity report** — the artifact that lets someone approve the production cutover

#### Design notes

- Traffic mirroring must be non-intrusive to production (read-only shadow, not inline)
- Soak window should be configurable but default to capturing at least one full business cycle
- Body normalization must handle dynamic content (timestamps, request IDs, CSRF tokens)
- The signed parity report is the sales artifact — it needs to look good enough to present to a change advisory board

---

### Workstream C — Cutover Orchestrator

**Weeks 8–10 · Commercial**

The final step: safe, staged traffic migration with automated rollback.

#### Deliverables

- [ ] Weighted, staged traffic shift (e.g., 1% → 5% → 25% → 50% → 100%)
- [ ] Automated rollback triggered on:
  - Error-rate regression against defined SLOs
  - Latency regression against defined SLOs
  - Custom health check failures
- [ ] Decommission checklist for the old ingress controller
- [ ] Audit-facing remediation certificate documenting the completed migration

#### Design notes

- Weight progression and SLO thresholds must be customer-configurable
- Rollback must be automatic and fast — minutes, not hours
- The remediation certificate is a compliance artifact; treat its format seriously

---

## 5. Go-to-Market Schedule

Runs in parallel with engineering. Marketing starts before code.

| Week | Action |
|------|--------|
| 1–2 | Landing page live, first technical post published |
| 2–4 | Direct outreach to existing client base — start with people who already trust you |
| 4–6 | Publish the analyzer as free open source; it is a lead magnet, price it accordingly |
| 5–8 | Deliver paid engagements 1 and 2, building Workstream B inside them |
| 8–10 | Case study from engagement 1; conference talk or webinar submission |

> **Sell the service before you build the tool.** The first two paying engagements tell you which 20% of the work is repeated and worth automating. Build the tool on the third engagement, not the zeroth.

---

## 6. Resourcing

| Track | Owner | Allocation |
|-------|-------|------------|
| Engineering | Senior engineer | 60% (3 days/week) |
| Sales, pricing, design authority | Founder | 40% |
| Delivery of paid engagements | Whole team | Remainder |

### Capacity protection

- **Ring-fence the days.** Named engineer, fixed days, not "whatever's left after client work."
- **If the ring-fence breaks twice in a month,** the roadmap has failed — re-plan rather than pretend.
- **Bill the R&D.** Engineering happens *inside* paid engagements. The customer pays for a migration; you build the tooling while delivering it.

---

## 7. Risk Register

| Risk | Impact | Mitigation |
|------|--------|------------|
| Billable work consumes product capacity | High — the default failure mode | Ring-fenced days; build inside paid engagements; re-plan rather than silently slip |
| Ingress migration window closes before shipping | High | Hard 10-week cap; ship the analyzer even if incomplete |
| `ingress2gateway` absorbs the validation layer | Medium | Compete on validation and orchestration, not translation; contribute annotations upstream to stay adjacent |
| Free analyzer cannibalizes paid work | Low | The analyzer creates demand for remediation; the report is a problem statement, not a solution |
| Customer security team blocks deployment | High | Read-only credentials, on-premises execution, no phone-home, air-gap capable, single auditable binary — from commit one |

---

## 8. Metrics

### Leading indicators (track from Week 1)

- Analyzer installs (krew download count)
- Landing page conversion rate
- Inbound assessment requests
- Security questionnaires received (a buying signal, not an annoyance)

### Lagging indicators

- Paid engagements delivered
- Engineer-hours per delivery (must trend down — if it doesn't, the tool isn't earning its build cost)
- Gross margin per engagement
- Assessment-to-engagement conversion rate

### Health indicators

- Ring-fenced engineering days actually protected
- Annotation knowledge base coverage (entries with test coverage)
- Time from customer request to delivered report

---

## 9. Gate — End of Week 10

| Criterion | Target |
|-----------|--------|
| Paid migrations delivered | ≥ 3 |
| Analyzer published and installable via krew | Yes |
| Median analyzer runtime on a real cluster | < 60 seconds |
| Written case study | ≥ 1 |
| Repeatable engagement margin | ≥ 50% |

---

## 10. Kill Criterion

> Fewer than **two paid engagements by Week 10** means the urgency isn't converting in your market.

**If triggered:**
1. Stop further engineering investment
2. Harvest the annotation knowledge base as content marketing (blog posts, comparison tables, community contributions)
3. Move engineering capacity to Phase 2 (VMware Assess) early

---

*Source: [Tier 1 Implementation Roadmap](../tier1-implementation-roadmap.md)*

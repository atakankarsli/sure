# Sure

Sure checks source code against product expectations written in plain English.

A coding agent writes feature contracts: short Markdown files with stable check IDs such as `checkout.failure/CHK-001`. A later agent changes code and runs `sure check`. Sure picks the contracts touched by the change, collects their source files within a byte budget, asks the [Jev](https://docs.typesafe.ai) decision model one fixed question per check, and prints JSON the agent can act on: which expectation, which source, whether it is a violation, missing context or a service failure, and the safe next step.

`sure check` never builds or runs the target application, runs its tests, opens a browser, starts an agent or edits source. It does send the selected source to the configured provider.

**Status:** pre-code. The structure, decisions and plan are in place; the CLI is not implemented yet. See [docs/PLAN.md](docs/PLAN.md).

## Planned commands (v0)

```sh
sure init --repo .
sure contracts validate --repo . --output json
sure plan --repo . --changed --base main --output json
sure check --repo . --all --output json
sure check --repo . --changed --base main --output json
sure check --repo . --contract checkout.failure --output json
sure doctor --repo . --output json
```

## A contract

```markdown
---
schema_version: sure.contract/v1
id: checkout.failure
severity: error
evidence: code
scope:
  - src/features/checkout/**
context:
  - src/domain/orders.ts
---
# Checkout failure

## CHK-001 — Preserve the cart
Given a non-empty cart, when order submission resolves with an error,
existing cart items and quantities must remain unchanged.
```

## Provider

Jev through the Vercel AI Gateway (`AI_GATEWAY_API_KEY`) by default, with TypeSafe direct (`TYPESAFE_API_KEY`) as a second transport. Keys come only from the environment. A `replay` provider plays back exact recorded requests offline.

## Not in v0

Watch mode, LSP, MCP, hook installers, auto-fix, dashboards, language servers, vector search, and image judgments.

## Repository

| Path | What |
|---|---|
| `AGENTS.md` | Rules for agents working here |
| `docs/PLAN.md` | Milestones, current state, blockers |
| `docs/adr/` | Decisions |
| `docs/research/jev-wire.md` | Verified Jev wire format and open unknowns |
| `docs/handoff/` | The original brief (11 Oct 2026), kept verbatim |
| `schemas/` | Versioned output schemas (draft) |
| `eval/` | Evaluation module: fixture app, corpus, harness |

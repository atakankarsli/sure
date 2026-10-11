# Sure

Sure checks source code against product expectations written in plain English.

A coding agent writes feature contracts: short Markdown files with stable check IDs such as `checkout.failure/CHK-001`. A later agent changes code and runs `sure check`. Sure picks the contracts the change touches, collects their complete source files within a byte budget, asks a decision model one fixed question per check, and prints one JSON document the agent can act on: which expectation, which source, whether it is a violation, missing context or a service failure, and the safe next step.

`sure check` never builds or runs the target application, runs its tests, opens a browser, starts an agent or edits source. It does send the selected source to the configured provider.

**Status:** v0. The CLI is complete and tested offline. Live runs work with Claude Haiku 5.5; Jev through the Vercel AI Gateway is implemented but unverified live (the gateway needs billing set up). Accuracy has not been measured; that is the separate evaluation repository's job.

## Install (local)

```sh
make build          # ./sure, CGO-free
./sure --help
```

## Use

```sh
sure init --repo .                                   # scaffold .sure/ (never overwrites)
sure contracts validate --repo . --output json       # strict, offline
git add .sure && git commit -m "Add contracts"       # the trusted base
sure plan  --repo . --changed --base main --output json   # what would be sent, offline
sure check --repo . --changed --base main --output json   # the real check
sure check --repo . --contract checkout.failure --output json
sure doctor --repo . --live --output json            # setup check; --live sends no source
sure schema                                          # JSON schema of the check output
```

Exit codes: `0` clear, `1` error-severity violation, `2` invalid invocation or config, `3` incomplete (unknown, unmapped change, missing evidence, contract change to review), `10` provider failure, `11` rate limit, `12` deadline, `13` malformed decision, `130` canceled. Operational failures outrank violations, which outrank incompleteness.

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

Config lives in `.sure/config.yaml` (`sure init` writes a commented one). Contracts and config are read from the trusted base commit; working-tree edits to them are reported as `contract_changes` and never weaken a check.

## Providers

| `provider.name` | Transport | Key variable | Default model | Probabilities |
|---|---|---|---|---|
| `jev` | `gateway` (default): Vercel AI Gateway `/v1/evaluate` | `AI_GATEWAY_API_KEY` | `typesafe-ai/jev` | model distribution |
| `jev` | `typesafe`: TypeSafe `/v1/systemone` | `TYPESAFE_API_KEY` | `jev-1.13.0` | model distribution |
| `anthropic` | Messages API, structured output | `SURE_ANTHROPIC_API_KEY` | `claude-haiku-5-5` | self-reported |

Keys come only from the environment; `.env` files are never read. `--provider` overrides the config for one run. `--record <dir>` saves decisions; `--provider replay --recordings <dir>` plays them back offline and never falls back to a live call. Decisions are cached under the user cache directory (`SURE_CACHE_DIR` overrides; `--no-cache` disables).

A verdict is accepted only when the chosen label's probability is at least 0.90 and leads the runner-up by 0.20; anything weaker is `unknown`. These are unvalidated starting values.

## Not in v0

Watch mode, LSP, MCP, hook installers, auto-fix, dashboards, language servers, vector search, image judgments.

## Repository

| Path | What |
|---|---|
| `AGENTS.md` | Rules for agents working on Sure |
| `skills/sure/SKILL.md` | How an agent uses Sure in another repository |
| `docs/PLAN.md` | Milestones, state, blockers |
| `docs/adr/` | Decisions |
| `docs/research/jev-wire.md` | Verified Jev wire format and open unknowns |
| `docs/handoff/` | The original brief (11 Oct 2026), kept verbatim |
| `schemas/` | Published output schema |

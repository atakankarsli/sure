# Sure

How sure are we that the code still does what the product needs?

Sure checks code changes against your product's core flows, written in plain English, and prints how sure a decision model is that each flow still holds, with the file and lines when it does not. It reads source only: no tests to write or wait for, no build, nothing executed. Coding agents run it after every change; a second model can double-check the few things it flags.

```
$ sure
sure · 2 of 5 contracts · 1 changed file vs main · jev/typesafe-ai/jev

  ✗ checkout.failure     3%
      CHK-001  3%  Preserve the cart · violated
                   → src/features/checkout/finish.ts:6-16
      read  src/domain/cart.ts, src/domain/orders.ts, src/features/checkout/finish.ts
  ✓ cart.lines          96%

3% sure · 1 violated · 3 passed · 1.2s
Next: fix the violated behavior, then run `sure checkout.failure`
```

## Use

```sh
sure                                  # check the flows your changes touch
sure checkout                         # check named flows (id or prefix)
sure --expect "Failed payment keeps the cart"   # check one more thing, just for this run
sure --json                           # one JSON document, for agents and CI
sure init                             # create .sure/ with a config and a template
sure validate                         # check .sure/ for mistakes, offline
sure doctor --live                    # check setup and the model connection
```

Flows live in `.sure/contracts/*.md` and are read from the base branch, so a change cannot weaken the flows it is checked against. Ad-hoc checks (`--expect`, `--checks file.md`) read the changed files plus what they import and what imports them; they work in any Git repository, even without `.sure/`.

Exit codes: `0` pass, `1` violated, `2` usage or config error, `3` not sure, `10`–`13` provider failure, `130` canceled.

## A flow

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
When order submission fails, the cart's items and quantities stay unchanged.
```

## Decision models

| `provider.name` | Model | Key variable | Probabilities |
|---|---|---|---|
| `jev` (default) | `typesafe-ai/jev` via Vercel AI Gateway, or `jev-1.13.0` direct (`transport: typesafe`) | `AI_GATEWAY_API_KEY` / `TYPESAFE_API_KEY` | from the model |
| `anthropic` | `claude-haiku-5-5` | `SURE_ANTHROPIC_API_KEY` | self-reported |

A check passes when its chosen label reaches 90% and leads the runner-up by 20 points; anything weaker is "not sure". Keys come only from the environment. Decisions are cached by content; `--record` and `--replay` save and replay them offline.

## Agents

`skills/sure/SKILL.md` teaches an agent to set Sure up (read the code, ask the user a few questions, propose about 20 flows) and to use it after every change.

## Repository

| Path | What |
|---|---|
| `AGENTS.md` | Working on Sure itself |
| `docs/PLAN.md` | State, measurements, next steps |
| `docs/adr/` | Decisions |
| `docs/research/jev-wire.md` | Verified Jev wire formats |
| `docs/handoff/` | The original brief, verbatim |
| `schemas/` | Output schema (`sure schema`) |

The evaluation corpus lives in the separate `sure-eval` repository.

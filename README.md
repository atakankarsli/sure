<p align="center">
  <img src="docs/assets/sure-mark.svg" width="120" alt="">
</p>

<h1 align="center">sure</h1>

<p align="center">
  <a href="#limits"><img src="https://img.shields.io/badge/status-experimental-f97316?style=flat-square" alt="status: experimental"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.26"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-7cb342?style=flat-square" alt="MIT"></a>
  <a href="https://github.com/atakankarsli/sure/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/atakankarsli/sure/ci.yml?branch=main&style=flat-square&label=ci" alt="CI"></a>
  <a href="#decision-models"><img src="https://img.shields.io/badge/decision%20model-Jev-000000?style=flat-square" alt="decision model: Jev"></a>
  <a href="https://github.com/atakankarsli/sure/issues/new"><img src="https://img.shields.io/badge/found%20a%20miss%3F-open%20an%20issue-7c3aed?style=flat-square" alt="found a miss? open an issue"></a>
</p>

<p align="center"><strong>How sure are we that the code still does what the product needs?</strong></p>

Sure checks a code change against your product's core flows, written in plain English, and prints how sure a decision model is that each flow still holds, with the file and lines when it does not. It reads source only. Nothing is built, run or tested.

**Status: experimental.** It finds real bugs and it misses some. Read [Limits](#limits) before you trust a green run.

```sh
go install github.com/atakankarsli/sure@latest
export TYPESAFE_API_KEY=...
cd any-git-repo && sure --expect "Deleting a user keeps their invoices"
```

That works in any repository, with no setup: Sure reads the files your branch changed plus what they import, and answers in under a second.

```
$ sure
sure · 2 of 5 contracts · 1 changed file vs main · jev/jev-1.13.0

  ✗ checkout.failure     4%
      CHK-001  4%  Preserve the cart · violated
                   → src/features/checkout/checkout.ts:12-43
      read  src/domain/cart.ts, src/domain/orders.ts, src/features/checkout/checkout.ts, src/features/checkout/failure.ts
  ✓ checkout.pending   100%
  · 3 contracts not affected

4% sure · 1 violated · 3 passed · 600ms
Next: fix the violated behavior, then run `sure checkout.failure`
```

## Install

```sh
go install github.com/atakankarsli/sure@latest

export TYPESAFE_API_KEY=...         # Jev, direct (docs.typesafe.ai)
export AI_GATEWAY_API_KEY=...       # or Jev through the Vercel AI Gateway
export SURE_ANTHROPIC_API_KEY=...   # optional: Claude as the decision model
```

Keys come only from the environment. A repository cannot choose the key or the endpoint.

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

Exit codes: `0` pass, `1` violated, `2` usage or config error, `3` not sure, `10`–`13` provider failure, `130` canceled.

## How it works

1. **Flows** are Markdown files in `.sure/contracts/`, one per feature, with one plain-English check per outcome. Sure reads them from the base branch, so a change cannot weaken the flows it is checked against.
2. **Evidence** is the complete source files a flow names. Over budget means "not sure", never a cropped approval. Nothing else leaves the machine.
3. **Decision**: Sure asks a decision model fixed questions about each check and gets a probability distribution back, not prose. A check passes at 90% with a 20-point lead; weaker answers are "not sure". Locations map to real bytes. Messages are templates. Nothing is generated.
4. **Output** is a short human summary or one JSON document, with the file and lines to look at and the command to run again.

Ad-hoc checks (`--expect`, `--checks file.md`) work in any Git repository without `.sure/`: they read the changed files plus what they import and what imports them.

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

## CHK-002 — Failure keeps submitting false
When placeOrder resolves with ok: false, the state that submit stores has submitting: false.
```

Write the second kind of check too. A sentence that names the function, the input and the effect is answered decisively; a general requirement is where the model hedges.

## Decision models

| `provider.name` | Model | Key | Probabilities |
|---|---|---|---|
| `jev` (default) | `jev-1.13.0` direct, or `typesafe-ai/jev` through the Vercel AI Gateway | `TYPESAFE_API_KEY` / `AI_GATEWAY_API_KEY` | from the model |
| `anthropic` | `claude-haiku-5-5` | `SURE_ANTHROPIC_API_KEY` | self-reported |

Jev answers in under a second and costs $0.042 per million input tokens. `--long-context` raises its limit from 32k to 262k tokens per flow for evidence that does not fit; it is not a default, see below. Decisions are cached by content; `--record` and `--replay` save and replay them offline.

## What we measured

[`sure-eval`](https://github.com/atakankarsli/sure-eval): a small TypeScript shop, 5 flows, 12 checks, 40 controlled changes with independent oracles. One run each, 2026-10-18, not calibrated. Details and the raw numbers are in [`docs/research/`](docs/research/).

| Setup | Planted bugs caught | Cleared wrongly | Safe checks cleared | False alarms | Per case |
|---|---|---|---|---|---|
| Jev, flows as written | 13 / 18 | 2 | 88 / 95 | 0 | 0.8 s |
| Claude Haiku 5.5, same flows | 16 / 18 | 0 | 82 / 95 | 0 | 5.6 s |
| Jev, checks phrased as concrete facts (probe on 8 bugs and 5 safe cases) | 15 / 18 | 0 | 5 / 5 cases | 0 | 0.8 s |

What hurt: more evidence. With the whole app per check Jev clears one more bug it should not; on a 60-file C# assembly, 159k tokens of context cleared a planted bug that a 23k-token feature scope kept flagged. What is next: facts and anti-facts per check, movement against the base as a verdict, and a second model for what stays unsure ([ADR 0003](docs/adr/0003-decision-ladder.md)).

## Limits

- Sure sees the files a flow names. A bug outside them is invisible. Implicit state across files (a field kept by object spread, a helper that drops a parameter) is where Jev misses today.
- "Not sure" (exit 3) is a real outcome and it is where bugs hide. Treat it as work, not as a pass.
- A decision model can be confidently wrong. Two of eighteen planted bugs were cleared at 95–97%. Keep tests on money, auth and data paths.
- Evidence per flow must fit about 80 KB (27k tokens). Split big flows; `--long-context` as a last resort.
- Import following for ad-hoc checks covers TypeScript, JavaScript and Python. For other languages pass `--scope`.
- `.sure/` lives at the repository root, and flows are read from the base branch: use `--draft` before the first commit.
- Source goes to the provider you configure. Through the gateway, routing is pinned to TypeSafe's zero-data-retention upstream. The gateway needs a Vercel team with billing enabled.
- One run per number above. Nothing here is calibrated; the policy thresholds are starting points.

## Agents

`skills/sure/SKILL.md` teaches a coding agent to set Sure up (read the code, ask a few questions, propose flows) and to run it after every change.

```sh
npx skills add atakankarsli/sure
```

## Repository

| Path | What |
|---|---|
| `AGENTS.md` | Working on Sure itself |
| `docs/PLAN.md` | State, measurements, next steps |
| `docs/adr/` | Decisions |
| `docs/research/` | Jev wire facts, long-context and question-design measurements |
| `docs/handoff/` | The original brief, verbatim |
| `schemas/` | Output schema (`sure schema`) |

MIT. The CLI skeleton comes from the author's [adc](https://github.com/atakankarsli/adc).

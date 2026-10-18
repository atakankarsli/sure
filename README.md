<p align="center">
  <img src="docs/assets/sure-mark.svg" width="120" alt="">
</p>

<h1 align="center">sure</h1>

<p align="center">
  <a href="#where-it-breaks"><img src="https://img.shields.io/badge/status-experiment-f97316?style=flat-square" alt="status: experiment"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.26"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-7cb342?style=flat-square" alt="MIT"></a>
  <a href="https://github.com/atakankarsli/sure/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/atakankarsli/sure/ci.yml?branch=main&style=flat-square&label=ci" alt="CI"></a>
  <a href="#models"><img src="https://img.shields.io/badge/decision%20model-Jev-000000?style=flat-square" alt="decision model: Jev"></a>
  <a href="https://github.com/atakankarsli/sure/issues/new"><img src="https://img.shields.io/badge/found%20a%20miss%3F-tell%20me-7c3aed?style=flat-square" alt="found a miss? tell me"></a>
</p>

<p align="center"><strong>An experiment: can a decision model tell me I broke a flow, without tests?</strong></p>

Agents write most of my code now, and most of the output tokens go to writing and re-running tests that prove it still works. Sure tries the other way round. You write your product's core flows in plain English. Sure reads the source files behind each flow and asks a decision model, a model that only answers fixed questions with a probability, whether the flow still holds. No build, no run, no tests. Under a second per flow, well under a cent.

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

## Try it

```sh
go install github.com/atakankarsli/sure@latest
export TYPESAFE_API_KEY=...            # docs.typesafe.ai; or AI_GATEWAY_API_KEY, or SURE_ANTHROPIC_API_KEY
cd any-git-repo && sure --expect "Deleting a user keeps their invoices"
```

That needs no setup: Sure reads the files your branch changed plus what they import. For a repository, `sure init`, write flows in `.sure/contracts/`, commit them, then `sure` after every change. `sure --json` is for agents; `skills/sure/SKILL.md` teaches one to do the setup (`npx skills add atakankarsli/sure`).

## What happened

I built a small shop in TypeScript with 5 flows, 12 checks, and 40 controlled changes with independent tests that say what each one really breaks ([sure-eval](https://github.com/atakankarsli/sure-eval)). One run each, 2026-10-18, nothing calibrated:

| Setup | Bugs caught | Cleared wrongly | Safe checks cleared | False alarms | Per case |
|---|---|---|---|---|---|
| Jev, flows as written | 13 / 18 | 2 | 88 / 95 | 0 | 0.8 s |
| Claude Haiku 5.5, same flows | 16 / 18 | 0 | 82 / 95 | 0 | 5.6 s |
| Jev, checks written as concrete facts (probe, 8 bugs + 5 safe cases) | 15 / 18 | 0 | 5 / 5 cases | 0 | 0.8 s |

Three things I did not expect:

- **The threshold is not the problem.** Jev answers at 0.93 or better when it sees a bug and at 0.28 or lower when it does not. Lowering the bar buys nothing.
- **The question is.** "Record returns false before adding when the attempt id was already recorded" gets 0.96. The same thing as a requirement sentence gets 0.87, or clears the bug at 0.94. Name the function, the input and the effect.
- **More context makes it worse.** On a 60-file C# assembly, 159k tokens of source cleared a planted bug at 1.00 that a 23k-token feature scope kept flagged. 2.5 s, same price, wrong answer.

Everything, including what I plan to do about it, is in [`docs/research/`](docs/research/) and [ADR 0003](docs/adr/0003-decision-ladder.md).

## Where it breaks

- Sure sees the files a flow names. A bug outside them is invisible, and implicit state across files (a field kept by object spread) is where Jev misses.
- "Not sure" (exit 3) is a real answer and it is where the bugs hide. Two of eighteen were cleared at 95–97%. Keep tests on money, auth and data.
- Evidence per flow must fit about 80 KB. Split flows; `--long-context` exists but see above.
- Import following covers TypeScript, JavaScript and Python; elsewhere pass `--scope`.
- `.sure/` lives at the repository root and flows are read from the base branch, so `--draft` before the first commit.
- Source goes to the provider you configure. Through the Vercel AI Gateway, routing is pinned to TypeSafe's zero-data-retention upstream.

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

## CHK-002 — Failure path clears submitting
When placeOrder resolves with ok: false, the state that submit stores has submitting: false.
```

Flows are read from the base branch, so a change cannot weaken the flows it is checked against. A check passes at 90% with a 20-point lead; weaker answers are "not sure". Locations are real bytes, messages are templates, nothing is generated.

## Models

| `provider.name` | Model | Key | Probabilities |
|---|---|---|---|
| `jev` (default) | `jev-1.13.0` direct, or `typesafe-ai/jev` via the Vercel AI Gateway | `TYPESAFE_API_KEY` / `AI_GATEWAY_API_KEY` | from the model |
| `anthropic` | `claude-haiku-5-5` | `SURE_ANTHROPIC_API_KEY` | self-reported |

Exit codes: `0` pass, `1` violated, `2` usage or config, `3` not sure, `10`–`13` provider, `130` canceled. `AGENTS.md` is for working on Sure itself; `docs/PLAN.md` has the state and what is next. MIT. The CLI skeleton comes from my [adc](https://github.com/atakankarsli/adc).

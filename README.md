<p align="center">
  <img src="docs/assets/sure-mark.svg" width="120" alt="">
</p>

<h1 align="center">sure</h1>

<div align="center">

[![SPONSORED BY E2B FOR STARTUPS](https://img.shields.io/badge/SPONSORED%20BY-E2B%20FOR%20STARTUPS-ff3001?style=for-the-badge&labelColor=black)](https://e2b.dev/startups)

</div>

<p align="center">
  <a href="#where-it-breaks"><img src="https://img.shields.io/badge/status-experiment-f97316?style=flat-square" alt="status: experiment"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.26"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-7cb342?style=flat-square" alt="MIT"></a>
  <a href="https://github.com/atakankarsli/sure/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/atakankarsli/sure/ci.yml?branch=main&style=flat-square&label=ci" alt="CI"></a>
  <a href="#models"><img src="https://img.shields.io/badge/decision%20model-Jev-000000?style=flat-square" alt="decision model: Jev"></a>
  <a href="https://github.com/atakankarsli/sure/issues/new"><img src="https://img.shields.io/badge/found%20a%20miss%3F-tell%20me-7c3aed?style=flat-square" alt="found a miss? tell me"></a>
</p>

<p align="center"><strong>Sure is a Go CLI that checks AI agent code changes against natural-language product flows.</strong></p>

<p align="center"><em>An experiment: can a decision model tell me I broke a flow, without tests?</em></p>

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

## Why

Agents write most of my code now, and most of the output tokens go to writing and re-running tests that prove it still works. Sure tries the other way round. You write your product's core flows in plain English. Sure reads the source files behind each flow and asks a decision model, a model that only answers fixed questions with a probability, whether the flow still holds. No build, no run, no tests. Under a second per flow, well under a cent.

It answers with a number and a location, not prose: each check is supported, violated or "not sure", with the file and lines the model pointed at. Flows are read from the base branch, so the change being checked cannot weaken them.

## Install

Needs Go 1.26 and Git. There are no prebuilt releases yet.

```sh
go install github.com/atakankarsli/sure@latest

# or from a clone
git clone https://github.com/atakankarsli/sure && cd sure && make build   # ./sure
```

Then set one provider key ([Models](#models)) in your environment and check it:

```sh
export TYPESAFE_API_KEY=...            # docs.typesafe.ai; or AI_GATEWAY_API_KEY, or SURE_ANTHROPIC_API_KEY
sure doctor --live                     # one synthetic request, no source code sent
```

Keys are read only from the environment. Never commit one.

## Use it

**One sentence, no setup.** Sure reads the files your branch changed against the default branch, plus what they import and what imports them:

```sh
cd any-git-repo
sure --expect "Deleting a user keeps their invoices"
sure --checks expectations.md              # a Markdown list of expectations, or - for stdin
sure --expect "..." --scope 'src/billing/**'   # read these files instead of the changed ones
```

**Flows in a repository.** Write them once, commit them, and run `sure` after every change:

```sh
sure init                  # creates .sure/config.yaml, .sure/contracts/ and a contract template
# write flows in .sure/contracts/*.md (see "A flow" below)
sure validate              # offline check of config and contracts
git add .sure && git commit -m "Add Sure flows"
sure                       # checks the flows your changes touch
sure checkout.failure      # checks one flow, or an id prefix
sure --all                 # checks every flow
```

**Before you spend a request**, `--dry-run` shows what would be checked and how many tokens would be sent, without calling a model. `--draft` uses uncommitted flows while you write them.

**For agents and CI**, `sure --json` prints one JSON document (`sure schema` prints its JSON Schema) and the exit code carries the verdict. [`skills/sure/SKILL.md`](skills/sure/SKILL.md) teaches an agent to set Sure up and act on its answers (`npx skills add atakankarsli/sure`).

Other flags: `--base`, `--provider`, `--record`/`--replay`, `--no-cache`, `--contracts-ref`, `--assess-base`, `--long-context`, `--timeout`, `--verbose`. `sure --help` describes each.

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

Exit codes: `0` pass, `1` violated, `2` usage or config, `3` not sure, `10`–`13` provider, `130` canceled.

## In E2B sandboxes (planned)

> **Not built yet.** Nothing in this repository talks to E2B today. This is what I am building with E2B for Startups.

The plan is one isolated [E2B](https://e2b.dev) sandbox per agent verification run:

1. An agent finishes a change on a branch.
2. A fresh sandbox starts for that run alone, installs Sure and clones the repository at the agent's branch, with the base branch fetched so Sure can diff against it and read the trusted flows.
3. The provider key is passed into the sandbox as an environment variable, never written to disk.
4. The sandbox runs `sure --json` against the repository's flows (or `sure --expect "..."` for a one-off product flow) and returns the JSON document and exit code to the agent.
5. The sandbox is thrown away.

Sure already never builds or runs the code it checks, so the sandbox is not there to contain the app. It is there so parallel agent runs never share a checkout, a cache or a key, and so a run can be reproduced from its branch alone. Progress goes in [`docs/PLAN.md`](docs/PLAN.md).

## More

[`docs/PLAN.md`](docs/PLAN.md) has the state and what is next; [`docs/adr/`](docs/adr/) the decisions. MIT. The CLI skeleton comes from my [adc](https://github.com/atakankarsli/adc).

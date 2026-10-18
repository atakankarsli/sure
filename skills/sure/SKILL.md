---
name: sure
description: Check that code changes keep a product's core flows working, without writing or running tests. Use after editing code in a repository that has .sure/, when asked to set up Sure or write the flows it checks, or to confirm one expectation about a change ("make sure checkout still keeps the cart").
---

# sure

`sure` reads source code and asks a decision model whether it still does what plain-English flows say. It never builds, runs or tests the app. It is experimental: it finds real bugs and misses some, and "not sure" is a real answer. Run `sure --help` for the exact interface.

## Install

```sh
go install github.com/atakankarsli/sure@latest
export TYPESAFE_API_KEY=...        # Jev direct; or AI_GATEWAY_API_KEY for the Vercel AI Gateway
export SURE_ANTHROPIC_API_KEY=...  # optional: Claude as the decision model
sure doctor --live                 # confirms the key and the model
```

Keys come only from the environment. Never write one into a file in the repository.

## After every change

```sh
sure --json
```

It checks the flows your changes touch (against origin/HEAD, main or master) and returns one JSON document. Read `confidence.percent`, then `checks[]`:

| Exit | Meaning | Do |
|---|---|---|
| 0 | Every touched flow holds, at ≥90% by default | Continue |
| 1 | A flow is violated | Open each `locations[]` with `role: model_selected` (file and lines), fix the code, run the check's `recheck` argv |
| 3 | Not sure: a check is unknown, a changed file belongs to no flow, or `.sure/` changed | Follow `next_actions`; tell the user what stays unresolved. Do not treat it as a pass |
| 2 | Bad flags or config | Read `errors[]`; run `sure doctor` |
| 10–13 | The model call failed | Report it; do not loop until it passes |

Never edit `.sure/contracts/` to make a check pass. Contracts are read from the base branch, so the edit would not count anyway; it would only show up in `contract_changes`. Stop after three repair rounds on the same check and report instead.

## Check something specific

When you want to be sure of something the flows do not cover, say it:

```sh
sure --expect "When payment fails, the cart keeps its items" --json
sure --checks expectations.md --json            # '## Title' sections or '- ' bullets
```

Ad-hoc checks read the changed files plus what they import and what imports them (TypeScript, JavaScript, Python). For other languages, or when they come back unsure, point them at the code with `--scope 'src/checkout/**'`.

If a check reports `evidence_budget_exceeded`, narrow the scope. `--long-context` (Jev at TypeSafe only) raises the limit to 262k tokens, but wide evidence makes the model miss bugs it finds with a feature-sized scope.

## Write checks the model answers decisively

Measured on the evaluation corpus: a general requirement is where the model hedges or clears a bug; a sentence that names the function, the input and the effect is answered at 0.95 or better. For each outcome write both:

```markdown
## CHK-003 — Allow another attempt
After a failed submission settles, submitting must be false so the customer can submit again.

## CHK-003a — Failure path clears submitting
When placeOrder resolves with ok: false, the state that submit stores (the value returned by applyFailure) has submitting: false.
```

- One observable outcome per check; name the function and the input where you can.
- Add a check for the error and edge branches (a thrown call, an empty error string, a repeated id), not only the main path.
- Keep each flow's evidence feature-sized: the files that implement it plus the exact files it depends on. Do not widen a scope to "be safe".

## Set up a repository (once)

1. Run `sure init`. It creates `.sure/` at the repository root and never overwrites anything.
2. Read the codebase: entry points, routes and screens, money paths, writes to storage, auth, background jobs.
3. Ask the user once, in a single message, at most five questions: which journeys make money or keep users; what must never break; what is out of scope; which areas are fragile; anything already promised to users.
4. Propose about 20 flows as a numbered list: id (`checkout.failure`), the outcomes it guarantees, and the files that implement it. Wait for the user's approval.
5. Write each approved flow as `.sure/contracts/<id>.md`:

   ```markdown
   ---
   schema_version: sure.contract/v1
   id: checkout.failure
   severity: error
   evidence: code
   scope:
     - src/features/checkout/**
   context:
     - src/domain/cart.ts
   ---
   # Checkout failure

   ## CHK-001 — Preserve the cart
   When order submission fails, the cart's items and quantities stay unchanged.
   ```

   - Write from what the product must do, not from what the code does today, then add the fact-level checks above.
   - Never renumber check IDs. `scope` globs cover the implementation; `context` lists the exact files it depends on.
6. Run `sure validate`, then `sure --all --dry-run` to check that each flow fits the budget, then `sure --all --draft` (the flows are not committed yet). A flow that comes back unsure usually needs a narrower scope or a more concrete check.
7. Commit `.sure/`. From then on `sure` checks against these committed flows.

## Measure it on your own code

Clone [`sure-eval`](https://github.com/atakankarsli/sure-eval) to see the harness: a fixture app, flows, independent oracle tests and 40 controlled changes with frozen labels. Copy the shape for your repository: a case is a small edit plus the oracle that proves what it breaks. Never tune on the cases you report.

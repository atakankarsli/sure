---
name: sure
description: Check that code changes keep a product's core flows working, without writing or running tests. Use after editing code in a repository that has .sure/, when asked to set up or update the flows Sure checks, or when you want to confirm a specific expectation about a change ("make sure checkout still keeps the cart").
---

# sure

`sure` reads source code and asks a fast decision model whether it still does what plain-English flows say. It never builds, runs or tests the app. Run `sure --help` for the exact interface.

## After every change

```sh
sure --json
```

It checks the flows your changes touch (against origin/HEAD, main or master) and returns one JSON document. Read `confidence.percent`, then `checks[]`:

| Exit | Meaning | Do |
|---|---|---|
| 0 | Every touched flow holds, at ≥90% by default | Continue |
| 1 | A flow is violated | Open each `locations[]` with `role: model_selected` (file and lines), fix the code, run the check's `recheck` argv |
| 3 | Not sure: a check is unknown, a changed file belongs to no flow, or `.sure/` changed | Follow `next_actions`; tell the user what stays unresolved |
| 2 | Bad flags or config | Read `errors[]`; run `sure doctor` |
| 10–13 | The model call failed | Report it; do not loop until it passes |

Never edit `.sure/contracts/` to make a check pass. Contracts are read from the base branch, so the edit would not count anyway; it would only show up in `contract_changes`. Stop after three repair rounds on the same check and report instead.

## Check something specific

When you want to be sure of something the flows do not cover, say it:

```sh
sure --expect "When payment fails, the cart keeps its items" --json
sure --checks expectations.md --json            # '## Title' sections or '- ' bullets
```

These ad-hoc checks read the changed files plus what they import and what imports them. If they come back unsure, point them at the code with `--scope 'src/checkout/**'`. Each sentence should state one observable outcome.

## Set up a repository (once)

1. Run `sure init`. It creates `.sure/` and never overwrites anything.
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

   - Write from what the product must do, not from what the code does today.
   - One behavioral outcome per check. Never renumber check IDs.
   - `scope` globs cover the implementation. `context` lists the exact files it depends on, such as types and services.
6. Run `sure validate`, then `sure --all --dry-run` to check that each flow fits the budget, then `sure --all`. A flow that comes back unsure usually needs more `context`.
7. Commit `.sure/`. From then on `sure` checks against these committed flows.

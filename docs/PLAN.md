# Plan

Milestones follow brief §17, adjusted by `docs/adr/0001-foundation.md`. Update this file as work lands.

## M0 — Inspect and document ✅ (2026-10-11)

- [x] Handoff read in full; kept verbatim in `docs/handoff/`.
- [x] ADC template read at `origin/main` `0d954e7`: AGENTS.md, go.mod, main.go, cmd/, registry, Makefile, .goreleaser.yml.
- [x] Jev wire checked: TypeSafe docs, plus Vercel gateway catalog and probes (`docs/research/jev-wire.md`).
- [x] Decisions recorded in ADR 0001.
- [x] Repository created; private remote `atakankarsli/sure`.
- [ ] Gateway live response shape. Blocked until the Vercel team has a credit card.

## M1 — Offline runnable slice

- CLI shell adapted from ADC: `main.go`, `cmd/`, registry, shared output/errors, `cmdtest`, Makefile, pre-commit hook, CI.
- `sure --help`, `--version`, `init`, `contracts validate`.
- Strict contract reader: frontmatter validation, check headings, rejection of unknown and duplicate keys, bad globs, unsafe paths.
- Whole-file evidence collector with byte budgets, deterministic order, hashes, missing-file reporting.
- Fake provider, run-result schema validation, exit-code mapping.
- `sure check` against a tiny `testdata/` fixture with deterministic fake answers.
- A test proving the target application is never executed.

## M2 — Real Jev and safe snapshots

- `gateway` transport first, then `typesafe`. Strict response validation; missing fields are errors.
- Deadlines, bounded worker pool, shared backoff with `Retry-After`, no retries on 4xx validation or auth errors.
- Cache keyed by normalized request, endpoint, model, prompt and policy versions, contract hash and evidence; replay provider.
- Git snapshot of the worktree, `--changed` with `--base` merge base, renames and deletions, unmapped changes.
- Trusted contracts from the base ref; `contract_changes`.
- Request and response fixtures from the first live calls.

## M3 — Fixture app and corpus (`eval/`)

- `eval/app`: small Next.js shop from the official `with-vitest` example, exact pinned dependencies and lockfile, pinned Node and package manager.
- Plain TypeScript domain (catalog, cart, checkout, orders) with injected time and services.
- Independent Vitest oracles; optional small Playwright suite.
- 10–12 contracts accepted and committed before any mutation.
- 40 cases from `docs/handoff/CASE_MATRIX.md`, opaque IDs, frozen dev/holdout splits.

## M4 — Measurement and repair demo

- Pilot run with the live key and an agreed budget, then 5 uncached repetitions on the frozen semantic split.
- Inspect dev-split errors, freeze prompts, then run holdout once.
- One preserved-behavior refactor and one bug → Sure finding → agent repair → oracle-confirmed recovery episode, at most 3 iterations.
- Honest report, including poor results.

## M5 — Release-ready development artifacts

README, skill, schemas, CI, CGO-free build check, changelog, local GoReleaser snapshot. Nothing is published without the owner.

## Blockers and open questions

- **Live calls:** the Vercel team needs a credit card on file. The gateway returns `403 customer_verification_required` until then.
- **Live budget:** not agreed yet. At $0.042 per million input tokens the full corpus with 5 repetitions is estimated well under $1. Proposed cap: $5.
- **Upstream pinning:** route gateway calls only to the `typesafe-ai` upstream (ZDR) before sending real source.
- **Model pinning through the gateway:** unknown; see `docs/research/jev-wire.md`.
- **Markdown parsing:** a restricted grammar or a parser; decide in M1.

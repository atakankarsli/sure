# Plan

Milestones follow brief §17, adjusted by `docs/adr/0001-foundation.md`. The evaluation milestones (fixture app, 40-case corpus, measurement, repair episodes) belong to the separate evaluation repository.

## M0 — Inspect and document ✅ (2026-10-11)

Handoff read and kept in `docs/handoff/`; ADC template read at `origin/main` `0d954e7`; Jev wire checked against TypeSafe docs and the gateway; ADR 0001; private remote.

## M1 — Offline vertical slice ✅ (2026-10-11)

- CLI adapted from ADC: `init`, `contracts validate`, `plan`, `check`, `doctor`, `schema`, `version`; grouped help; injected writers and dependencies.
- Strict contract grammar and config validation; trusted-vs-working-tree diff.
- Snapshot capture of allowlisted bytes, whole-file evidence bundles, regions for localization, byte and token budgets.
- Fake provider; JSON output validated against `schemas/run-result.schema.json`; exit-code precedence.
- Tests prove the target is never executed (scripts, Makefile, fsmonitor and diff drivers stay untouched).

## M2 — Real providers and safe snapshots ✅ (2026-10-11), live Jev pending

- Jev over the Vercel gateway (`/v1/evaluate`, upstream pinned to `typesafe-ai`) and TypeSafe direct; Claude (`claude-haiku-5-5`) through the official Go SDK with structured output.
- Strict response validation, retries with `Retry-After` and a shared backoff gate, deadlines, redirects refused.
- Content-addressed cache, `--record` / `--provider replay`.
- `--changed` with merge base, renames, deletions, untracked files, unmapped changes, plan invalidation by dependency manifests and contract edits; `--assess-base` regression labelling.
- Visual manifest parsing and the `unsupported_evidence_kind` outcome.
- Live trial (Claude Haiku 5.5, 2026-10-11, demo repo in a scratch folder): the clean baseline gave 1 supported and 2 unknown (`low_confidence`, p≈0.80–0.85); a cart-clearing bug in the failure branch came back `violated` (p=0.93) on the right function, exit 1. Two runs, about 9.7k input tokens in total. This is a smoke test, not a measurement.

## M5 — Release-ready development artifacts ✅ (local only)

README, AGENTS.md, skill, schema, Makefile, pre-commit hook, CI (format, vet, race tests, CGO-free cross-build), GoReleaser with releases disabled.

## Next

- **Live Jev:** add a credit card to the Vercel team, then run `sure doctor --live`. Confirm the gateway's response shape, whether `providerOptions` pinning is honored, and the returned model; record request/response fixtures.
- **Evaluation repository:** fixture shop, independent oracles, 40 cases, frozen splits, measured precision/recall per provider. Only then tune the policy thresholds.
- **Claude calibration:** Haiku's self-reported probabilities sit just under the 0.90 threshold on correct code. Do not lower the threshold before the evaluation repository measures it; try `effort: medium` first.

## Open questions

- Live budget for the evaluation runs (Jev is $0.042 per million input tokens; Haiku 5.5 is $0.10/$0.50).
- Whether a Jev version can be pinned through the gateway.

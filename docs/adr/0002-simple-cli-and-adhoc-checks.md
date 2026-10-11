# ADR 0002 — A Vercel-style CLI, confidence, and ad-hoc checks

Date: 2026-10-11. Status: accepted by the owner.

## Context

The owner wants Sure to replace much of the test writing in fast-moving, agent-built projects: an agent changes code, runs one command, and gets a percentage plus the file and lines when something broke. The interface should feel like Vercel's tools: one obvious command, few flags, short output. `eve` was the reference for tone.

## Decisions

1. **Bare `sure` is the check.** It selects the flows touched by changes against the default branch (origin/HEAD, main, master), or every flow when nothing changed. Positional arguments name flows by id or prefix. Visible commands: `sure`, `init`, `validate`, `doctor`. `plan` became `--dry-run`; `contracts validate` became `validate`; `--output json` became `--json`. `check`, `schema` and `version` still work but are not listed.
2. **Confidence is the model's number.** Each check's confidence is P(supported) from the decision model. A flow shows its weakest check, and the run shows its weakest flow. A check the model never decided counts as 0; a provider failure leaves no number. The pass bar stays the acceptance policy (90%, 20-point margin) in the trusted config.
3. **Ad-hoc checks.** `--expect` sentences and `--checks` Markdown (light lists or full contracts) run for one invocation. They are untrusted by design and labelled `adhoc`. Without `--scope` they read the changed files plus their imports and importers, within three quarters of the state budget. They work without `.sure/`, using default settings.
4. **Trust in every mode.** Trusted contracts come from the merge base with the default branch whatever the selection, and from HEAD only when HEAD is the default branch. This closes a hole the evaluation found in `sure <contract>`.
5. **Gateway route.** Jev goes through `/v4/ai/decision-model` (AI SDK decision model spec v4) instead of `/v1/evaluate`, so the response shape is the one the SDK validates.
6. **Self-reported probabilities are normalized.** Claude often omits options or drifts from a total of 1. Omitted options count as 0, totals within 0.15 of 1 are rescaled, and a repeated option keeps its larger value. Jev answers stay strictly validated.

# Plan

## Done (2026-10-11)

- **M0–M2 of the brief:** contracts, trusted base, snapshots, evidence, Jev (gateway and direct), Claude, cache, replay, visual manifest, exit codes, schema. Offline tests across every package, race-clean, CGO-free cross-builds.
- **Vercel-style CLI:** bare `sure` checks the flows a change touches and prints how sure the model is (`3% sure`, per flow and per check, with file:lines). Four visible commands: `sure`, `init`, `validate`, `doctor`.
- **Ad-hoc checks:** `--expect`, `--checks file.md|-`, `--scope`. They read the changed files plus their imports and importers (TS/JS, Python), and work without `.sure/`.
- **Gateway:** Jev is called through `POST /v4/ai/decision-model`, the route the AI SDK uses. Refusals are handled.
- **Speed:** unchanged tracked files are identified by Git blob IDs and read only when they become evidence; the region table goes once per request.
- **Evaluation repo** (`sure-eval`): fixture shop, 19 independent Vitest oracles, 40 cases validated without any model, and a harness with a contract track and an ad-hoc track.

## Measured (one run each, 2026-10-11; not calibrated)

Jev `jev-1.13.0` called directly through TypeSafe, against Claude Haiku 5.5 at effort low:

| Track | Provider | Violations caught | Wrongly cleared | Safe checks cleared | False alarms | Time |
|---|---|---|---|---|---|---|
| contracts, dev + holdout | Jev | 13/18 | **2** | 88/95 | 0 | 40 cases 7.4 s, p50 0.9 s/case |
| contracts, dev + holdout | Haiku | 16/18 | 0 | 82/95 | 0 | 30 cases 33 s, p50 5.6 s/case |
| ad-hoc (diff + imports), dev | Jev | 7/12 | 1 | 39/59 | 0 | 3.2 s |
| ad-hoc (diff + imports), holdout | Jev | 1/6 | 1 | 16/36 | 0 | 2.0 s |
| ad-hoc (diff + imports), dev | Haiku | 8/12 | 0 | 33/59 | 0 | 24.4 s |
| software (incomplete + trust) | either | — | — | 27/27 assertions | — | under 7 s |

Jev is about six times faster and clears more safe code. Its misses are confident, though. It cleared both CHK-003 mutants (C003, C015, P(supported) 0.94–0.95): a removed `submitting: false` that object spread keeps true. In the ad-hoc track it also cleared C010 (stock taken inside the validation loop, 0.98). TypeSafe's jaggedness notes list indirection as a known weakness. Haiku's misses were all "not sure" and in the right direction (0.85–0.88 on contracts), but its numbers are self-reported and cluster at round values. Holdout results were not used to tune anything.

The eval found and fixed one real bug: `sure <contract>` used to trust HEAD, so a contract weakened on a feature branch was believed. Trusted contracts now come from the merge base in every mode.

## Next

- **Jev through the gateway:** works directly with `TYPESAFE_API_KEY` today; the gateway route still needs a card on the Vercel team.
- **Indirection misses:** on the dev split only, try question wording that asks the model to follow each field to its final value on the failure path, and measure CHK-003 again. Consider a second opinion (Claude) for checks Jev clears below 0.97.
- **Policy:** after the Jev run, decide the threshold per provider from the dev split only.
- **Evidence:** import following covers TS/JS and Python; add Go and Swift if those repos need ad-hoc checks.
- **Repair episodes:** the brief's agent repair demo (bug → Sure finding → agent fix → oracle) is not built yet.

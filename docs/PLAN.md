# Plan

## Done (2026-10-11)

- **M0–M2 of the brief:** contracts, trusted base, snapshots, evidence, Jev (gateway and direct), Claude, cache, replay, visual manifest, exit codes, schema. Offline tests across every package, race-clean, CGO-free cross-builds.
- **Vercel-style CLI:** bare `sure` checks the flows a change touches and prints how sure the model is (`3% sure`, per flow and per check, with file:lines). Four visible commands: `sure`, `init`, `validate`, `doctor`.
- **Ad-hoc checks:** `--expect`, `--checks file.md|-`, `--scope`. They read the changed files plus their imports and importers (TS/JS, Python), and work without `.sure/`.
- **Gateway:** Jev is called through `POST /v4/ai/decision-model`, the route the AI SDK uses. Refusals are handled.
- **Speed:** unchanged tracked files are identified by Git blob IDs and read only when they become evidence; the region table goes once per request.
- **Evaluation repo** (`sure-eval`): fixture shop, 19 independent Vitest oracles, 40 cases validated without any model, and a harness with a contract track and an ad-hoc track.

## Measured (Claude Haiku 5.5, effort low, one run each; not calibrated, not Jev)

| Track | Split | Violations caught | Wrongly cleared | Safe checks cleared | False alarms | Time |
|---|---|---|---|---|---|---|
| contracts | dev (20) | 11/12 | 0 | 49/59 | 0 | 20.6 s, p50 5.6 s/case |
| contracts | holdout (10) | 5/6 | 0 | 33/36 | 0 | 12.1 s |
| ad-hoc, diff only | dev (20) | 6/12 | 0 | 13/59 | 0 | 21.9 s |
| ad-hoc, diff + imports | dev (20) | 8/12 | 0 | 33/59 | 0 | 24.4 s |
| software (incomplete + trust) | 10 | — | — | 27/27 assertions | — | 6.7 s |

Every miss so far is "not sure", never a wrong answer. Haiku's self-reported probabilities sit just under 90% on correct code. One holdout case failed on a format error (a repeated option), since fixed. Holdout results were not used to tune anything.

The eval found and fixed one real bug: `sure <contract>` used to trust HEAD, so a contract weakened on a feature branch was believed. Trusted contracts now come from the merge base in every mode.

## Next

- **Jev live:** add a card to the Vercel team, then `sure doctor --live` and `npm run eval -- --split all --provider jev` in `sure-eval`. Compare against the table above.
- **Policy:** after the Jev run, decide the threshold per provider from the dev split only.
- **Evidence:** import following covers TS/JS and Python; add Go and Swift if those repos need ad-hoc checks.
- **Repair episodes:** the brief's agent repair demo (bug → Sure finding → agent fix → oracle) is not built yet.

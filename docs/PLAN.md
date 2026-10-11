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
| ad-hoc (diff + imports), dev | Jev | 11/12 | 0 | 49/59 | 0 | 3.1 s |
| ad-hoc (diff + imports), holdout | Jev | 2/6 | 1 | 30/36 | 0 | 2.0 s |
| ad-hoc (diff + imports), dev | Haiku | 11/12 | 0 | 50/59 | 0 | 19.0 s |
| ad-hoc (diff + imports), holdout | Haiku | 6/6 | 0 | 36/36 | 0 | 11.4 s |
| software (incomplete + trust) | either | — | — | 27/27 assertions | — | under 7 s |

Ad-hoc rows were rerun after a harness bug: requirements were cut at their first line break, so the model judged half sentences. That bug, not Jev, caused the earlier ad-hoc C010 miss.

Jev is about six times faster. Its one real weakness so far is confident: it cleared both CHK-003 mutants (C003 and C015, P(supported) 0.92–0.95). There a removed `submitting: false` is kept true by object spread, across two files. In a probe on C003, the contract wording gave 0.79 supported. A one-path fact ("when placeOrder resolves with ok: false, the returned state has submitting false") gave 0.81 violated. Stating the hidden input ("applyFailure is called while submitting is true") gave 0.95 violated. Jev sees the bug once the question points at the path and the incoming value; it does not trace implicit state across files on its own. TypeSafe's jaggedness notes list indirection as a known weakness. Haiku's misses were all "not sure" and in the right direction (0.85–0.88 on contracts), but its numbers are self-reported and cluster at round values. Holdout results were not used to tune anything.

The eval found and fixed one real bug: `sure <contract>` used to trust HEAD, so a contract weakened on a feature branch was believed. Trusted contracts now come from the merge base in every mode.

## Next

- **Jev through the gateway:** works directly with `TYPESAFE_API_KEY` today; the gateway route still needs a card on the Vercel team.
- **Indirection misses:** on the dev split only, try question wording that asks the model to follow each field to its final value on the failure path, and measure CHK-003 again. Consider a second opinion (Claude) for checks Jev clears below 0.97.
- **Policy:** after the Jev run, decide the threshold per provider from the dev split only.
- **Evidence:** import following covers TS/JS and Python; add Go and Swift if those repos need ad-hoc checks.
- **Repair episodes:** the brief's agent repair demo (bug → Sure finding → agent fix → oracle) is not built yet.

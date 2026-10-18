# Jev long context: does more evidence help?

Date: 2026-10-18. Model `jev-1.13.0` through TypeSafe directly, one run per cell, `--no-cache`. Not calibrated; the
numbers are what one run returned.

## What long context is

TypeSafe raises both the per-request and the per-question limit from 64k/32k to 262k tokens when a request carries
`TypeSafe-Danger-Zone-Long-Context: true`. There is no separate model ID, the price is the same, and TypeSafe
documents it as slower and less accurate. Sure sends the header with `--long-context` (or `provider.long_context: true`
in the trusted config), TypeSafe transport only, and raises the evidence floors to 640 KiB of state, 768 KiB per request
and 240k estimated tokens. Cache keys change with it (wire format `jev-typesafe-v1-long`).

The synthetic `sure doctor --live --long-context` probe was accepted: same answer and the same 347 input tokens as the
standard probe.

## TypeScript fixture (`sure-eval`, 40 cases, 113 labelled checks)

"Wide" is a copy of the corpus where every contract's scope is `src/**` and the `context` lists are dropped, so each
check reads the whole 9.5 KB app instead of its feature.

| Evidence | Header | Violations caught | Wrongly cleared | Safe cleared | Unsure | p50 / case |
|---|---|---|---|---|---|---|
| scoped (as committed) | off | 13/18 | 2 (C003, C015) | 88/95 | 10 | 0.8 s |
| scoped | on | 13/18 | 2 (C003, C015) | 87/95 | 11 | 0.8 s |
| wide | off | 12/18 | 1 (C016) | 87/90 | 8 | 0.8 s |
| wide | on | 12/18 | 2 (C015, C016) | 87/90 | 7 | 0.8 s |

- The scoped standard run reproduces the 11 October numbers exactly.
- The header alone moves probabilities by about ±0.05. Three checks crossed the 0.90 bar in either direction
  (C005 CAT-003 0.91→0.85, C008 CHK-001 0.89→0.95, C025 CAT-003 0.95→0.88). Noise, not a trend.
- Wide evidence clears more safe checks that scoped evidence left unsure (C002, C006, C012, C016, C023 CAT-003),
  turns the known C003 miss into unsure (0.97→0.64), and creates a new confident miss: C016 (a serialization helper
  drops the filter, page code unchanged) goes from unsure 0.70 to **supported 0.93**. One catch is lost (C004 or C006,
  depending on the header).
- Software cases C031–C033 and the trust edits of C027/C038 do not apply to the wide copy (no `context`, `src/**`
  maps every file, case edits target the original contract text). Under the header alone, C032 (budget exceeded) now
  fits and passes; that is the flag doing its job, not a regression.

## A Unity game's core assembly (C#, ad-hoc checks)

The owner's game in development: `Game.Core`, 60 pure-C# files, 558 KB. Eight `--checks` sentences on the real
checkout, and on a copy with two planted bugs: the venue ledger no longer ignores a repeated attempt id (check 3),
and a world day set backwards reopens the attention budgets (check 4). Checks 7 and 8 are deliberately false, to see
confident `violated` answers.

| Evidence | Tokens sent | Time | Clean checkout | With the two bugs |
|---|---|---|---|---|
| Venues + Economy (standard) | 16k | 0.7 s | 3 supported, 3 unsure (no attention code), 2 violated | — |
| Venues + Economy + Attention (long) | 23k | 0.8 s | 5 supported, 1 unsure, 2 violated | check 3 **unsure, P(violated) 0.73**; check 4 unsure, P(supported) 0.87 |
| changed files only (standard, the ad-hoc default) | 7k | 0.4 s | — | check 3 unsure 0.74 violated; check 4 unsure 0.85 supported; 4 checks unknown for lack of evidence |
| all of Core, 60 files (long) | **159k** | 2.5 s | 5 supported, 1 unsure, 2 violated | check 3 unsure, P(violated) **0.34**; check 4 **supported 1.00** |

- 159k tokens answered in 2.5 s. "Slower" is real but not a problem; cost is $0.007 per request.
- Sure's estimate (bytes/3) was 187.7k against 158.8k actual on C#: about 18% conservative.
- On clean code, wide and narrow evidence agree on 7 of 8 checks; the eighth (analytics event names) drifts from
  0.73 to 0.46 supported with the extra 500 KB.
- On buggy code, wide evidence is worse at both bugs: the ledger bug drops from 0.73 to 0.34 violated, and the
  day-rollover bug is **cleared with P(supported) 1.00**, where narrow evidence at least refused to clear it. The
  60-file state contains both bugs; the model stops seeing them.
- Neither bug was confirmed `violated` under the 0.90 policy even with narrow evidence, but both moved the
  distribution a long way from the clean run (0.09→0.73 and 1.00→0.87). Comparing base and current distributions
  (`--assess-base`) would surface that movement to the agent.
- The ad-hoc default (changed files plus imports) reads only the two changed C# files because `using` is not
  followed: half the checks come back unknown for lack of evidence. C# needs import following or contract scopes.

## Conclusions

1. Long context works and is cheap and fast enough. It is a fallback for evidence that does not fit, not a way to
   skip evidence selection.
2. More context hurts bug detection: on the one real C# bug pair, 159k tokens cleared a bug that 23k tokens kept
   unsure. TypeSafe's "less accurate" warning is about dilution, and it shows at 160k tokens in one run. The slicing
   work in `docs/research/evidence-slicing.md` stays the main road.
3. The default 64 KiB state cap bites before the 32k-token limit on C# (73 KB of three Core folders is 25k tokens).
   Raising the default toward about 90 KiB would let a feature-sized C# scope fit without the header.
4. The probabilities carry more signal than the verdicts: a check whose P(violated) jumps from 0.09 to 0.73 after a
   change is worth a pointer even when it stays `unknown`.

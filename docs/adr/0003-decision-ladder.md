# ADR 0003 — Decisive verdicts: facts, movement and a second opinion

Date: 2026-10-18. Status: proposed; waits for the owner and for a fresh holdout.

## Context

The owner's requirement: Sure must find real bugs and must not answer "not sure" where a bug is there. Measurements
in `docs/research/decisive-questions.md` and `jev-long-context.md` show where the hedging comes from:

- Jev is bimodal. When it sees a bug it answers at ≥ 0.93; when it does not, no threshold helps.
- It sees a bug when the question names the function, the input and the effect. A requirement sentence leaves
  indirection bugs (state kept by object spread, a helper that drops a field, a loop that checks one line) at
  "not sure" or, worse, cleared at 0.95.
- More evidence makes this worse, not better: 159k tokens cleared a planted bug that 23k tokens kept unsure.
- Claude catches what Jev leaves unsure, at ten times the latency and a self-reported probability.

## Decisions

1. **A check carries facts.** Next to its requirement sentence, a check may list `facts` (what the code does on the
   path that satisfies the requirement, naming the function and the input) and `anti_facts` (the behaviour a bug
   would show, stated as a claim). The contract author writes them with the code in view; `sure init` and the skill
   show the shape. A check without facts still works and is marked `facts: none` in the output.
2. **One request, union rule.** Sure asks the requirement, every fact and every anti-fact as independent questions
   over the same feature-sized evidence. A check is `violated` when any fact is violated at the policy bar or any
   anti-fact is supported at it, unless its paired fact is also supported at the bar (a contradiction, reported as
   `inconsistent`). It is `supported` when the requirement and every fact are supported at the bar and every
   anti-fact is violated at it. Anything else is `unsure` and goes to the next step. The bar stays 0.90 / 0.20.
3. **Movement is a verdict.** With `--assess-base` (default on for change checks), the same questions are asked on
   the base. A fact that was supported at the bar on the base and is at or below 0.40 now makes the check
   `regressed`, a failing verdict with the changed hunk as location. The brief's rule stands: this is not a proof of
   runtime behaviour; it is a bounded, byte-backed pointer to what the change did to the model's belief.
4. **A second opinion closes the ladder.** Checks still unsure go to the `anthropic` provider with the same evidence
   and questions when `SURE_ANTHROPIC_API_KEY` is set. Its verdict counts under its own bar, labelled
   `second_opinion`. Without the key the check stays `unsure` and the run is incomplete, as today.
5. **Evidence stays feature-sized.** Long context is a fallback for evidence that does not fit, never a default.
   Ad-hoc checks on C# follow `using` directives the way TS imports are followed today.
6. **Numbers are claimed only from a fresh holdout.** The current holdout was used to probe fact wording. New cases
   are written and frozen before the ladder's numbers go in the README.

## Consequences

- Authoring gets heavier: a good contract is the requirement plus two to four facts per check. That is the work the
  owner wanted the contract-writing agent to do; the skill guides it.
- Cost per check rises from one to three questions on one request (still one Jev call), plus a base call on change
  checks and a Claude call only for unsure checks. On the corpus: about 0.8 s per feature for the first two steps,
  7 s for a check that reaches the third.
- The output gains verdicts `regressed`, `inconsistent` and a `decided_by` field (`jev`, `movement`,
  `second_opinion`), all in the schema; `unsure` remains possible but is the end of a ladder, not the first answer.
- Cache keys include the question set, so adding facts re-asks only the checks that changed.

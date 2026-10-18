# Decisive questions: why Jev says "not sure", and what makes it sure

Date: 2026-10-18. `jev-1.13.0` direct, one run per cell, no cache. Companion to `jev-long-context.md`.
**Caveat:** the probes below touched holdout cases (C014–C017). Fact wording was written from the clean fixture
before any case edit was read, but the holdout can no longer be called untouched for the facts design; a fresh
holdout is needed before any number here is claimed.

## 1. The threshold is not the problem

On the scoped corpus run (113 labelled checks), Jev's answers are bimodal. Every caught violation sits at
P(violated) ≥ 0.93; every missed one at ≤ 0.28:

| P(violated) ≥ t | caught / 18 | false alarms / 95 |
|---|---|---|
| 0.90 | 13 | 0 |
| 0.50 | 13 | 0 |
| 0.30 | 13 | 1 |

Lowering the bar buys nothing. The five misses (C003, C015, C014, C016, C017) are questions the model does not see
the bug through, not answers it hedged.

## 2. The question is the problem

The Unity game's core assembly from `jev-long-context.md`, two planted bugs, two to four files of evidence (3–4k
tokens, 0.3 s per request):

| Question | Ledger bug (repeat attempt id recorded again) | Day bug (earlier day reopens budgets) |
|---|---|---|
| Requirement sentence | violated 0.87 | **supported 0.94** (wrong) |
| Fact naming the path ("Record returns false before adding… when attemptId is in recentAttemptIds") | **violated 0.96** | violated 0.75 |
| Anti-fact (the buggy behaviour stated positively) | supported 0.94 | **supported 0.98** |
| BASE vs CURRENT ("the change makes X true") | **yes 0.98** | yes 0.49 / no 0.50 |
| Same fact + anti-fact on the clean base | supported 1.00 / violated 1.00 | — |

A fact that names the function, the input and the expected effect is answered decisively. The anti-fact (what the
bug does, stated as a claim) catches the case the fact alone leaves at 0.75. The diff question is decisive for a
removed guard and a coin flip for a subtle rewrite.

## 3. Facts on the corpus

Fact and anti-fact questions for CHK-002, CHK-003, CAT-001 and ORD-001, written from the clean code, asked with the
contract's own evidence set and Sure's prompt wording. Rule: **bug** when any fact is violated ≥ 0.90 or any
anti-fact is supported ≥ 0.90; **clean** when every fact is supported ≥ 0.90 and every anti-fact violated ≥ 0.90;
otherwise unsure.

| Case | Before (requirement only) | With facts | Note |
|---|---|---|---|
| baseline, C020, C029, C023, C022, C026 (clean / safe) | cleared | **clean**, lowest 0.92 | no false alarm |
| C002, C005 (caught before) | caught | **bug** 0.97 / 1.00 | kept |
| C010 (caught before, 0.98) | caught | unsure (fact 0.74) | the requirement sentence sees it, the fact does not |
| C016 (holdout, unsure 0.70) | unsure | **bug** 1.00 | serialization helper drops the filter |
| C017 (holdout, unsure 0.73) | unsure | **bug** 1.00 + anti 0.94 | stock checked for the first line only |
| C003 (wrongly cleared 0.97) | unsafe clear | unsure: fact 0.72 violated, P(supported) 0.97 → 0.27 | spread keeps `submitting` true |
| C015 (wrongly cleared 0.95) | unsafe clear | unsure: fact 0.62 violated, 0.95 → 0.37 | same bug, two files |
| C014 (holdout, unsure 0.74) | unsure | unsure (0.88 / 0.84) | empty error string treated as success |

Taking the union of the requirement sentence and the facts: **15 of 18 caught, 0 wrongly cleared, 0 false alarms**,
3 unsure (C003, C015, C014), against 13 / 2 / 0 / 5 with requirement sentences alone.

## 4. Movement against the base

The same fact on the clean base and on the change: CHK-003.f2 goes 1.00 → 0.27 (C003) and 1.00 → 0.37 (C015);
on the safe edits C020 and C029 it stays at 0.99–1.00. A rule "a fact that was ≥ 0.90 on the base and is ≤ 0.40
now is a regression" catches both spread bugs with no movement on the safe cases. C014 moves only 0.99 → 0.88.

## 5. A second opinion on what is left

Claude Haiku 5.5 at effort low, Sure's contracts track, on the five misses and five safe controls:

| | Caught | Wrongly cleared | Safe cleared | False alarms | p50 |
|---|---|---|---|---|---|
| Haiku on C003, C014, C015, C016, C017 + C020, C022, C023, C026, C029 | **5/5** (P(supported) 0.02–0.05) | 0 | 29/30 | 1 (C014 CHK-001) | 6.8 s |

The one labelled false alarm deserves a look at the label: the C014 edit makes a failed result with an empty error
take the success path, which also clears the cart, so CHK-001 may be violated in fact and the oracle may test only a
non-empty error.

## 6. What this means for the verdict

A ladder, each step only for checks the previous one left unsure:

1. Jev, feature-sized evidence, requirement + facts + anti-facts in one request (0.8 s, under a cent): 15/18 here.
2. Movement against the base for the same questions (`--assess-base`, one more Jev request): +2 (C003, C015).
3. Claude on the remaining unsure checks with the same evidence (7 s each): +1 (C014).

Across this corpus that is 18/18 with 0 wrongly cleared and 0–1 labelled false alarms, in one run, with a holdout
that these probes have now touched. Treat it as the design hypothesis to confirm on a fresh holdout, not as a result.

Fact sentences used, for reproduction:

- CHK-002: submit sets status 'success' only where result.ok is true; when ok is false the status becomes 'failed' whatever the error string, including empty · when placeOrder throws, status becomes 'failed' and the cart is not cleared · anti: an ok:false result with an empty error leads submit to set 'success' or clear the cart.
- CHK-003: when placeOrder throws, the stored state has submitting false · when placeOrder resolves ok:false, the state submit stores (returned by applyFailure) has submitting false · anti: after an ok:false result the stored state keeps submitting true so a later submit returns early.
- CAT-001: nextPage keeps the category and increments the page · serializeQuery includes category whenever it is a non-empty string, for every page · createCatalogApi passes the parsed category and page to listProducts · anti: for some page or category the next-page request omits or changes the category.
- ORD-001: validateOrder compares every line with inventory.available before ok:true; one short line makes it ok:false · placeOrder calls inventory.take only after ok:true · anti: stock is reduced before validation finishes, or only the first line is checked.

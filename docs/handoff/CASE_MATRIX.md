# Initial 40-case evaluation matrix

These are **case specifications to implement**, not measured results. Titles and expected outcomes belong only to the harness. Use opaque case IDs in the materialized target workspace. The cases must be reviewed against the final English contracts before labels are frozen.

Each semantic mutant must typecheck and fail an independent behavioral oracle for its stated reason. Each safe change must preserve the oracle. Run all other relevant oracles too, so an accidental unrelated defect does not become a mislabeled control.

## Contract families

- `checkout.failure/CHK-001`: a failed submission preserves cart items and quantities.
- `checkout.failure/CHK-002`: failed submission cannot transition to success.
- `checkout.failure/CHK-003`: submitting clears after a failed attempt settles.
- `checkout.pending/CHK-004`: a second submit while pending does not start another order request.
- `catalog.query/CAT-001`: pagination retains the selected filter.
- `catalog.query/CAT-002`: changing the filter returns pagination to the first page.
- `catalog.query/CAT-003`: an earlier request cannot overwrite results of a newer query.
- `cart.lines/CART-001`: removing a line preserves other lines and their quantities.
- `cart.lines/CART-002`: adding an existing SKU increments rather than replaces its quantity.
- `orders.validation/ORD-001`: insufficient stock causes rejection without inventory changes.
- `orders.validation/ORD-002`: the committed price derives from the service catalog, not a client-supplied price.
- `orders.validation/ORD-003`: validation rejection creates no committed order.

The implementation agent should write these as real Markdown contracts, with actual file scopes and dependencies. Do not add mutation-specific clues to the model prompts.

## Semantic cases: 18 violations, 12 safe changes

| ID | Split | Proposed change | Expected target |
|---|---|---|---|
| C001 | dev | Clear cart in the failed-order branch. | violated CHK-001 |
| C002 | dev | Transition to success after either service result. | violated CHK-002 |
| C003 | dev | Remove failure-path cleanup of submitting state. | violated CHK-003 |
| C004 | dev | Remove the already-submitting guard. | violated CHK-004 |
| C005 | dev | Drop the filter when constructing the next-page request. | violated CAT-001 |
| C006 | dev | Retain the old page when changing filters. | violated CAT-002 |
| C007 | dev | Remove the request-generation check before updating results. | violated CAT-003 |
| C008 | dev | Remove a cart line by clearing the entire cart. | violated CART-001 |
| C009 | dev | Replace existing quantity with the newly requested quantity. | violated CART-002 |
| C010 | dev | Mutate inventory before the insufficient-stock rejection path. | violated ORD-001 |
| C011 | dev | Use client price when constructing a committed order. | violated ORD-002 |
| C012 | dev | Persist an order before completing validation. | violated ORD-003 |
| C013 | holdout | A separately referenced failure helper now clears the cart; caller unchanged. | violated CHK-001 |
| C014 | holdout | Treat a resolved `{ok:false}` service result as successful resolution. | violated CHK-002 |
| C015 | holdout | Move submitting reset into the success-only branch during extraction. | violated CHK-003 |
| C016 | holdout | A query serialization helper loses a filter; page code unchanged. | violated CAT-001 |
| C017 | holdout | Check stock only for the first cart line; a later line is insufficient. | violated ORD-001 |
| C018 | holdout | Removal predicate also drops a different SKU with a shared prefix. | violated CART-001 |
| C019 | dev | Rename private identifiers without changing behavior. | supported affected checks |
| C020 | dev | Extract the correct failure cleanup into a helper. | supported affected checks |
| C021 | dev | Replace a nested condition with an equivalent early return. | supported affected checks |
| C022 | dev | Add an explicit return type to the order result. | supported affected checks |
| C023 | dev | Reorder pure object fields in a query request. | supported affected checks |
| C024 | dev | Change a nonsemantic local variable name in the cart operation. | supported affected checks |
| C025 | dev | Correct a comment without changing executable behavior. | supported affected checks |
| C026 | dev | Extract a catalog-price lookup while preserving the data source. | supported affected checks |
| C027 | holdout | Move a source file and update all imports plus accepted mapping in a new trusted baseline. | supported; no phantom absence |
| C028 | holdout | Replace a safe list operation with an equivalent implementation. | supported affected checks |
| C029 | holdout | Introduce a wrapper that preserves result/error propagation. | supported affected checks |
| C030 | holdout | Change formatting and line placement around a relevant operation. | supported; current line locations |

C027 has a reviewed alternate baseline/mapping because changing trusted scope is a separate guardrail, not something a candidate can authorize. Alternatively preserve the original path through a re-export and use unchanged trusted scope; document which construction is chosen before freezing the split.

## Incomplete evidence and coverage cases

| ID | Condition | Expected handling |
|---|---|---|
| C031 | A required explicit dependency file is unavailable. | unknown, missing_context, nonzero incomplete |
| C032 | Complete feature evidence exceeds the configured source budget. | unknown, evidence_budget_exceeded; no silent truncation pass |
| C033 | A newly added allowed source file is outside all declared scopes/context. | unmapped_changes reported; incomplete |
| C034 | A trusted contract's source glob matches no current code. | unknown/empty_scope, never supported |
| C035 | A separately trusted visual contract has an artifact, but configured provider is Jev. | unknown, unsupported_evidence_kind; no image judgment |
| C036 | An explicit evidence artifact's source digest no longer matches current worktree. | unknown, stale_evidence; no reuse |

## Contract and trust-boundary cases

| ID | Attempt | Expected handling |
|---|---|---|
| C037 | Delete a check from the candidate contract file. | trusted check remains; contract change review required |
| C038 | Narrow scope to exclude changed problematic source. | use trusted scope; report review-required change |
| C039 | Lower severity or decision threshold in the candidate. | trusted gate policy remains; report attempted change |
| C040 | Insert a repository setting that redirects the provider to a new host. | never connect; reject invalid/untrusted policy and report it |

C035/C036 and trust cases need deterministic software tests in addition to corpus episodes. API 401/429/529, malformed responses, timeout, replay misses, and cancellation belong to the Go transport/CLI test suite; do not count them as semantic quality evidence.

## Scoring rules

Score each case/check pair against frozen labels. Use `violated` as the positive class. `unknown`, `error`, and `not_checked` are not true-positive detections of a labeled violation. Report them separately and include them among undetected violations. `supported` on a labeled violation is the unsafe-clear category.

Measure precision/recall on the 30 semantic cases and their check-level outcomes. Report incomplete and trust cases in their own pass/fail tables, not in a blended accuracy percentage. For repeats, show per-case agreement and raw distributions; do not use cache hits as independent repetitions.

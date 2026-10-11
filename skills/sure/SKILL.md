---
name: sure
description: Write feature contracts in plain English and check code changes against them with the `sure` CLI. Use after editing code in a repository that has a .sure/ directory, when asked to add or review product requirements as contracts, or when a `sure check` result needs to be acted on.
---

# Sure

`sure` checks source code against feature contracts: short Markdown files in `.sure/contracts/` whose checks state, in English, what the product must do. It reads source; it never builds, runs or tests the application.

## After a batch of edits

```sh
sure check --repo . --changed --base main --output json
```

Read the single JSON document on stdout. Progress lines go to stderr.

| Exit | Meaning | What to do |
|---|---|---|
| 0 | No violation in the selected checks, or nothing applicable changed | Continue. `no_changes` and `not_checked` are not passes of anything. |
| 1 | An error-severity check is `violated` | Fix the behavior, then run the check's `recheck` argv. |
| 3 | Incomplete: `unknown` checks, changed source no contract covers, or contract changes needing review | Follow each `next_actions` entry; report what you could not resolve. |
| 2 | Invalid flags or config | Read `errors[]`; run `sure doctor --output json`. |
| 10–13, 130 | Provider or run failure | Not a verdict. Report it; do not retry until green. |

For each check in `checks[]`:

- `requirement` is the immutable expectation. Never edit a contract to make a check pass; contract edits are evaluated from the trusted base anyway and reported in `contract_changes`.
- `locations` with `role: model_selected` is the region the model picked; `inspected` only means the file was read. Neither is an execution trace.
- `next_actions` are typed: `inspect_source`, `supply_context`, `narrow_scope`, `provide_visual_evidence`, `review_contract_change`, `map_changed_source`, `recheck`. Run `recheck` exactly as given; it keeps the trusted contracts ref and provider.
- `unknown` is never a pass. `low_confidence` and `model_unknown` usually mean the contract's scope or context misses a file the behavior depends on.

Stop after three repair rounds on the same check and report instead of looping.

## Writing contracts

```markdown
---
schema_version: sure.contract/v1
id: checkout.failure
severity: error
evidence: code
scope:
  - src/features/checkout/**
context:
  - src/domain/orders.ts
---
# Checkout failure

## CHK-001 — Preserve the cart
Given a non-empty cart, when order submission resolves with an error,
existing cart items and quantities must remain unchanged.
```

- Write from accepted product intent, not from what the code does today.
- One observable behavioral outcome per check. No "everything works" checks and no implementation trivia.
- `scope` globs select the code that implements the feature; `context` lists exact extra files it depends on (types, services). Both trigger the contract when they change.
- Every `##` heading is a check: `## <ID>-<number> — <title>`. Check IDs are stable; never renumber.
- `evidence: image` checks stay `unknown` until an image-capable provider exists.
- Run `sure contracts validate --repo . --output json`, then commit `.sure/`. Checks use committed contracts as the trusted base; before the first commit use `--draft`.

## Other commands

- `sure plan --repo . --changed --output json`: what would be selected and sent, offline.
- `sure doctor --repo . --output json`: Git, config, trusted contracts, key presence, cache. `--live` sends one synthetic question with no source.
- `sure schema`: the JSON schema of the check output.

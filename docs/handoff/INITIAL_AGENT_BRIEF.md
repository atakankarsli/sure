# Sure — initial implementation brief

**Research snapshot:** 11 October 2026.  
**Deliverable:** a working Go CLI plus an independently reproducible evaluation repository.  
**Project name and executable:** `sure`. Naming is settled.  
**Important:** This is an implementation specification, not a claim that Sure or its benchmark already exists. Commands, thresholds, corpus sizes, and example findings below are proposed interfaces and design choices.

## 1. Build the right product

Sure checks **source code against versioned product expectations written in English**. A coding agent authors the expectations. A later agent changes code and runs Sure. Sure collects a bounded set of relevant source files, asks a decision model fixed questions, and returns machine-readable diagnostics.

The primary command must NOT build or execute the target application, run its tests, open a browser, invoke a coding agent, or modify its source. The application under inspection does not need to start. Hosted Jev evaluation does send selected source to the configured provider; “source-only” does not mean “no network.”

Three distinct components:

1. **Author:** the user's existing coding agent writes contracts and context mappings.
2. **Checker:** Sure collects evidence and invokes Jev; it does not rewrite requirements.
3. **Evaluator:** a separate development harness creates controlled code changes and runs independent application tests to measure whether Sure was right. These tests are NOT part of `sure check`.

v0 is a scoped source-reading verifier, not a sound whole-program proof system. `supported` means the supplied source supports the requirement under the recorded scope and assumptions. It does not mean the entire product was executed or proven correct.

## 2. Workspace and existing Go reference

Use the owner's supplied Go CLI template when it is present in the workspace. Inspect it before choosing dependencies or command layout. The accessible reference researched for this brief is **`atakankarsli/adc`**; it has not been confirmed to be the exact template the owner meant. Prefer a differently supplied local template rather than silently assuming ADC is authoritative.

Files actually reviewed in ADC:

- `AGENTS.md`: ffcli, one command package per command, a central registry, noninteractive commands, Markdown/JSON/table reporting, sentinel errors, make targets, and CLI tests.
- `go.mod`: `github.com/peterbourgon/ff/v3 v3.4.0`; the file currently declares Go 1.23.0.
- `cmd/run.go`: parsing, signal cancellation, exit mapping; it currently writes directly to process streams.
- `internal/cli/shared/output.go`: shared output rendering.
- `.goreleaser.yml`: `CGO_ENABLED=0`, macOS/Linux, amd64/arm64, version metadata injection.

**Reuse the architecture, not the whole product.** Do not carry Apple APIs, WWDC data, scraping, user-agent rotation, anti-detection behavior, Apple URLs, or ADC-specific release destinations into Sure. Preserve required license notices for any reused source. Do not publish private template history or unrelated files.

Recommended adaptation: keep ffcli and the command registry; inject `io.Writer`, a provider interface, clock, sleeper, and Git reader for tests. Keep `os.Exit` in `main` only. Construct fresh command state for each invocation, avoiding package-global flag state. Keep `--output`, matching the existing CLI idiom, rather than inventing a parallel `--format` option.

The Go 1.23 declaration is an observed template detail, not a recommendation to use an old toolchain. Select a currently supported Go release, validate dependencies, and record the selected version. Preserve a CGO-free default build for v0.

Work in new local directories. Do not modify ADC. Do not push, create remote repositories, publish releases, or modify a Homebrew tap unless separately authorized.

## 3. Two repositories, deliberately separated

### `sure` — product repository

```text
sure/
  main.go
  cmd/{run,root,root_usage,exit_codes}.go
  internal/
    cli/
      registry/
      shared/
      initcmd/
      contracts/
      plan/
      check/
      doctor/
    contracts/             # strict Markdown/frontmatter reader and validation
    gitview/               # base/current snapshot and changed-path abstraction
    evidence/              # scoped, bounded, read-only source collection
    engine/                # planning, evaluation, decisions, coverage accounting
    provider/              # interface and typed result models
      jev/                 # direct TypeSafe HTTP adapter
      replay/              # explicit offline playback; never network fallback
    cache/
    report/
  schemas/
  skills/sure/SKILL.md
  testdata/
  tools/eval/              # developer harness, NOT invoked by sure check
  docs/
  Makefile
  .goreleaser.yml
```

Honor the actual template's layout where equivalent. Avoid a framework of dozens of tiny packages. Do not add Cobra, a web service, or a Node runtime to the Go binary merely for convenience.

### `sure-eval` — controlled application and corpus

```text
sure-eval/
  src/
    app/                   # small Next.js App Router UI
    features/{catalog,cart,checkout}/
    domain/                # plain TypeScript product behavior
    server/                # local/in-memory services, no external accounts
  .sure/
    config.yaml
    contracts/
  oracle/
    unit/                  # independent Vitest expectations
    browser/               # small optional Playwright oracle/screenshot suite
  eval/
    cases.jsonl
    patches/               # controlled, individually replayable edits
    splits.json
  package.json
  lockfile                 # use the actual lockfile for the chosen package manager
  README.md
```

Use the official Next.js `with-vitest` example as a setup reference, not a large SaaS starter. Its published package file contains floating dependencies; resolve a compatible set and commit the lockfile before constructing the corpus. Do not perpetuate `next: latest` in a reproducibility fixture. Pin Node/package-manager versions too.

Use local catalog data and a controllable in-memory order service. No database server, payment processor, login service, Docker requirement, remote fonts, analytics, or hosted account is necessary. Inject time, service results, and deferred promises where needed. Reset state between oracle tests.

Keep business logic in plain TypeScript modules. Next's current testing guide notes that Vitest does not support async Server Components; do not make those components the center of the first oracle. Small browser tests may cover actual rendering separately.

## 4. Contract v1: readable English with stable IDs

One feature document contains several independently assessed checks. Proposed format:

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
  - src/domain/cart.ts
---
# Checkout failure

## CHK-001 — Preserve the cart
Given a non-empty cart, when order submission resolves with an error,
existing cart items and quantities must remain unchanged.

## CHK-002 — Do not show success
When order submission fails, the application must not transition to
the order-success state.

## CHK-003 — Allow another attempt
After a failed submission settles, submitting must become false so
the customer can retry.
```

Canonical check IDs are `<document-id>/<local-check-id>`. The English text is the contract, not a model-generated summary of it. Include the actual requirement in each model question: Jev question IDs are correlation keys, not a substitute for model-visible instructions.

Use strict frontmatter validation and a Markdown parser or deliberately restricted, well-tested grammar. Ignore headings inside fenced code. Reject unknown metadata keys, duplicate YAML keys, duplicate document/check IDs, empty checks, bad globs, invalid evidence kinds, unsafe paths, and contradictory configuration. Required explicit context files must exist; a missing one is incomplete evidence, not a reason to drop it silently.

`scope` and `context` are **both** evidence inputs and change triggers. Scope selects a code area; explicit context makes cross-file dependencies practical before a semantic index exists. The engine collects all matching allowed files, not only changed hunks.

v0 assessment support: `code`. Reserve `image` as a known evidence kind, but return `unknown / unsupported_evidence_kind` when only Jev is configured. A visual check must not turn green because an image exists.

The agent should draft contracts from product intent and accepted requirements, then inspect the implementation. Do not assume every current behavior is desirable. Avoid “everything works” checks and implementation-style trivia unless that is the actual product requirement. One behavioral outcome per check.

Do not require every user to build a runtime test for every contract. Valid/invalid fixtures and independent oracle tests are required for the **Sure evaluation corpus**, not as an artificial prerequisite for using a source-reading product.

## 5. CLI v0

Proposed commands:

```sh
sure --help
sure --version
sure init --repo .
sure contracts validate --repo . --output json
sure plan --repo . --changed --base main --output json
sure check --repo . --all --output json
sure check --repo . --changed --base main --output json
sure check --repo . --contract checkout.failure --output json
sure doctor --repo . --output json
```

`init` only scaffolds files and refuses to overwrite existing user content. It does not call Jev or invent product requirements. `contracts validate`, `plan`, and the default `doctor` are offline. An explicit doctor live-probe flag may check credentials against a tiny synthetic input, never source.

`check` supports `--no-cache`, `--provider jev|replay`, `--timeout`, and `--contracts-ref <ref>`. Targeted contract checks use the same context collector as full checks. Do not add `watch`, LSP, MCP, hooks installers, auto-fix, or a hosted dashboard before the core evaluation is working. Agent hooks can initially invoke the ordinary CLI once after an edit batch.

One JSON document on stdout in JSON mode. Progress, retry notices, and human logs go to stderr. No banners, ANSI escape sequences, markdown fences, or trailing status lines on stdout. Capture errors in the JSON envelope whenever JSON output mode was successfully selected. Help is a documented exception.

Commands are noninteractive. Read `TYPESAFE_API_KEY` from the environment. Never request a key in chat, commit it, include it in reports, or silently load repository `.env` files.

## 6. Snapshot semantics and coverage — get these right before optimization

For `--changed`, default base is `HEAD` when no base is supplied. With `--base main`, resolve the merge base with current HEAD and use it as the baseline. The current target is the working-tree snapshot, including committed branch changes, staged and unstaged modifications, deletions, renames, and permitted untracked source files. Capture bytes once before model calls. Never claim the worktree is represented solely by its commit SHA.

Read-only Git commands must disable external diff and textconv execution. Use argument arrays, not a shell; bound output. Respect the repository boundary and do not follow symlinks outside it. A `--staged` index-only mode is a later feature unless implemented fully and tested; never implement it by listing staged paths and then reading working-tree bytes.

Initial selection policy:

- `--all`: evaluate all declared checks.
- `--changed`: select every contract whose scope OR context intersects changed paths, using both old and new paths for renames and deletions.
- Config, dependency manifests/lockfiles, compiler configuration, and contract changes invalidate relevant planning; conservatively select all contracts until a narrower policy is tested.
- A changed source file covered by no contract must appear as `unmapped_changes`. It produces an incomplete run, not a reassuring clean result.
- No changed files is `no_changes`, with zero evaluated checks. This is not full-product verification.
- No matching code or a missing required file yields unknown/incomplete; it never yields supported.
- An excluded contract is `not_checked`, not passed. Preserve total/selected/evaluated counts.

In v0, do not claim an inferred cross-repository or compiler-accurate call graph. Explicit scope/context is the honest baseline. A future adapter may add import/symbol relationships; v0 should leave an interface for it, not delay the first experiment to build all language servers.

### Trusted contract baseline

For protected checks, read contracts and policy from the resolved trusted base (`--contracts-ref`, otherwise the comparison base or HEAD). The working branch cannot lower thresholds, redirect the endpoint, delete a requirement, or remove a required evidence kind to make a failure disappear.

Compare candidate contracts to the trusted version and report changes separately. Evaluations still use the trusted wording. Treat removals, scope changes, severity changes, criteria changes, and evidence changes as `contract_changed` / review-required. Do not ask the model whether weakening is acceptable.

Bootstrap before the first contract commit must be explicit draft mode, labeled `trust_mode=draft`; it is not a protected gate. For the experiment, commit the initial accepted contracts before applying mutations.

A result can be labeled an introduced regression only when a compatible baseline assessment was supported and the current assessment is violated. Otherwise call it a contract finding or unknown, not a newly proven regression. Assess base and current separately under identical contracts, model, and prompt policy; do not infer runtime behavior from a probability delta.

## 7. Evidence collector v0

Start with a pure-Go, file-scoped reader. For each contract, assemble complete source files selected by scope/context in deterministic order, with stable IDs, paths, line ranges, hashes, and explicit missing-file information. This supports language-neutral source reading without pretending to implement Swift/Kotlin type resolution.

Default allowlisted input root for the demo is `src/`. Exclude tests, oracle data, patches, expected results, Git metadata, dependencies, generated output, caches, `.env*`, credentials, binary files, and recordings **before** building any provider request. In normal user repos, tests may become an explicit future evidence source; in the clean source-only benchmark they are excluded to prevent answer leakage.

Apply configurable byte budgets and conservative token estimates. Record estimates separately from provider-reported usage. Small example defaults such as 16 KiB of state and 24 KiB of serialized request are starting policies to evaluate, not a tokenizer guarantee. Enforce the provider's limits too. Over-budget evidence yields unknown with the list of omitted files. Do not crop away a branch and then approve the requirement. Do not independently approve slices and combine them into a whole-program approval.

Whole files are acceptable for the first small demo; the evaluation should expose when this becomes too broad or too expensive. Tree-sitter is a later optimization. It provides incremental syntax parsing and has a C runtime; adding it must deliberately revisit the template's CGO-free cross-build requirements.

For localization, enumerate actual candidate source regions with IDs, plus `NONE`. Ask a separate Choice question to select the relevant region on a violation. Map the returned ID to existing bytes in Go. If localization is uncertain, report a file- or contract-level finding. Never fabricate a line, causal path, or quote.

Source comments, strings, filenames, and documents are untrusted data, not instructions. Explicit prompts and adversarial cases help; they are not a proof of prompt-injection immunity.

## 8. Jev adapter: direct HTTP, narrowly typed

Verified documentation at the research date describes:

- `POST https://api.typesafe.ai/v1/systemone`, bearer authorization.
- Top-level `model`, `state`, and `questions` (a map, not a chat message list).
- Stable model ID `jev-1.13.0`; prefer it to a moving alias for evaluation.
- `answers` keyed by the submitted IDs, plus returned model identity and token usage.
- Noul returns `noul`; Choice returns `choice`, `probabilities`, and `confidence`.
- Jev is text/JSON only and is not a text-generation model.

Implement the narrow adapter using Go's `net/http`; the product does not need Python or JavaScript SDKs. Do not treat Jev as an OpenAI-compatible chat API. v0 needs Choice for three-way assessment and optional Noul for auxiliary questions, not a universal SDK.

The model documentation lists 64k tokens for a whole request and 32k for state plus the longest question. Jevlint has useful batching code, but do not copy its concurrency/rate defaults blindly: provider limits are dynamic. Use a small bounded worker pool, shared rate-limit backoff, and request/run deadlines. On 429/529 and selected transient 5xx responses, honor `Retry-After` with bounded exponential backoff and jitter. Do not retry malformed requests or bad credentials indefinitely.

Use a configured, trusted HTTPS endpoint. A repository cannot redirect secrets/source to a different endpoint. Reject cross-origin redirects. Test HTTP servers must be injectable through test dependencies, not an insecure production default.

Validate responses strictly: exact required question coverage; expected type; allowed Choice value; complete, finite probabilities in [0,1]; a plausible normalized distribution; required fields present; nonnegative usage; and compatible model identity. A missing `noul` must NOT decode into Go's zero-value float and become a false pass. Use pointers or `json.RawMessage` to distinguish missing/null/zero. Preserve response errors; malformed decisions are operational failures.

### Assessment strategy

Use fixed engine prompts with three options:

- `supported`: the supplied current implementation supports the precise requirement in the recorded scope/assumptions.
- `violated`: supplied executable source indicates a concrete conflicting behavior under the precondition.
- `unknown`: evidence is missing, unresolved, ambiguous, or inadequate for that claim.

Include the full English requirement in each question, and name the relevant source region directly where known. Questions in one call are independent; one cannot use another answer from that call. Batch only questions sharing useful evidence, not every feature into one giant state.

Starting experimental policy: accept supported/violated only when the chosen label's probability is at least 0.90 and its margin over the runner-up is at least 0.20; otherwise map to unknown. These numbers are **unvalidated development defaults**, not published accuracy guarantees. Version the policy and tune it only on the development split. Jev's Choice `confidence` is distribution-derived; it is not the same field as the selected label's probability.

Keep arithmetic and exact source/file checks in ordinary code. Do not use `1 - P(violation)` as evidence that the entire contract is satisfied. An absent implementation can look nonviolating unless incompleteness is handled explicitly.

Jev does not generate a diagnosis paragraph. v0's message is a deterministic template containing the original requirement, verdict, and actual inspected/selected regions. `message_origin` must say `template`. A future explanatory generative layer is optional and cannot override the underlying decision.

## 9. Cache and provenance

Content-address decisions by the actual normalized request, endpoint identity, pinned model, prompt/policy versions, contract hash, evidence contents, and collector version. Cache invalidation must cover added/deleted files and context dependencies, not just the edited file. Rebuild plan manifests against the current path inventory before reusing decisions.

Do not place timestamps, run IDs, random temporary paths, or commit subjects inside the model-visible state. They prevent reusable requests and can leak scenario clues. Namespace caches by repository/trust context. Separate model-decision cache from display locations so stale line numbers cannot be reused.

Use atomic writes and restrictive permissions. Never cache network failures as valid decisions. Replay mode returns only exact recorded requests and never falls back to a live provider. Distinguish `live`, `cache`, `replay`, and test-only fake results in reporting. Cached consistency is not model reliability.

Store current source digests, contract hashes, actual returned model, prompt version, and selected/omitted evidence metadata in the report. Full source request recording is opt-in and local; never commit private source recordings. Example numeric answers in this handoff are synthetic examples, not observations.

## 10. Agent-facing output and exit behavior

Use `schemas/run-result.schema.json` and `examples/agent-result.json` in this handoff as starting contracts. Keep result schema versioning independent of the binary's version.

Essential fields: run ID, source snapshot digest, trust mode, scope, total/selected/evaluated counts, all selected check outcomes, operational errors, unmapped changes, provider usage, and agent next actions.

A finding should tell an agent:

1. Which immutable expectation is at issue.
2. Which actual source to inspect and whether it was merely inspected or selected as relevant.
3. Whether this is a violation, missing context, unsupported visual input, or service failure.
4. Which safe next step is appropriate.

`next_actions` uses typed data such as `inspect_source`, `supply_context`, `provide_visual_evidence`, `review_contract_change`, and `recheck`. Recheck arguments are generated by Sure and preserve original repo, baseline, contract trust, and provider flags. Do not emit executable fixes produced by the model. Evidence locations are not an independently verified execution trace.

Recommended exit codes extending ADC's pattern:

| Exit | Meaning |
|---|---|
| 0 | Scoped check complete, no violation; or explicitly reported no-changes/no-applicable-checks state |
| 1 | At least one error-severity contract violation |
| 2 | Invalid invocation or configuration |
| 3 | Incomplete: unknown, unmapped source change, missing evidence, or contract review required |
| 10 | Network/provider transport failure |
| 11 | Rate-limit/overload exhausted |
| 12 | Deadline exceeded |
| 13 | Malformed/incompatible provider decision |
| 130 | User cancellation |

Do not return zero for a selected check that was never evaluated. Operational failure takes precedence over a violation exit, then violation over incomplete; the JSON retains every outcome and all counts. For multiple operational errors, use a documented fixed priority. Configuration errors discovered before evaluation use 2. Nonzero check exits are normal evaluation outputs, not automatically failures of the corpus harness.

## 11. The clean demo application

Build a small local shop with four behavior areas:

- Catalog filtering and pagination: selected filter survives page changes; changing filter resets pagination; old responses do not replace a newer query.
- Cart: add/remove items and preserve existing quantities during a failed checkout.
- Checkout: submitting state, single in-flight request, failure handling, successful completion.
- Order service: validate current stock and use server-side catalog prices; rejected orders produce no committed order or inventory change.

Do not add auth, payments, or a backend server solely to make the fixture appear realistic. The order service can be deterministic TypeScript with injected dependencies; a thin Next route may exercise wiring where useful. Prefer 10–12 clear contracts and a small number of short files over a broad product.

The baseline must build and pass its independent oracle. A failing case must compile/typecheck and fail the relevant behavioral oracle for the intended reason. Syntax errors, broken imports, timeouts, and unavailable dependencies are separate invalid/infra cases, not impressive semantic detections.

## 12. Corpus and independent ground truth

Create **40 initial cases**:

- 20 development cases: 12 real violations + 8 safe changes.
- 10 held-out semantic cases: 6 violations + 4 safe changes.
- 6 insufficient-evidence/coverage cases.
- 4 contract/trust-boundary cases.

`CASE_MATRIX.md` specifies the cases. The initial agent may implement the corpus, but must not tune prompts on held-out cases after inspecting their results. This synthetic holdout is an engineering check, not proof of generalization to arbitrary companies. Later add independent real-world changes selected outside the implementation agent.

Each manifest records an opaque case ID, split, baseline tree/commit identity, patch hash, target contract/check IDs, and oracle mapping. Ground-truth labels and explanations stay in the harness workspace, not in the candidate source workspace. The model sees no expected verdicts, case titles, commit messages describing the bug, mutation operator names, oracle tests, or previous failing test logs.

For each case:

1. Materialize an isolated candidate from a pinned clean baseline; never reset the user's working tree.
2. Apply only the controlled application patch. Keep contracts/config/oracle under trusted harness control except intentional tamper fixtures.
3. Capture a candidate source digest and run Sure. Source roots and evidence allowlists are explicit.
4. Store the raw Sure JSON and exit status.
5. Run the independent application oracle outside Sure and score against pre-reviewed expectations.
6. Reject invalid mutants and unexpected extra failures; do not silently count them as true positives.
7. Destroy only the isolated candidate created for this case.

Oracle tests must test real domain behavior rather than assert on mocks returning what the test configured. Fixing the fixture or test is allowed during corpus development, but regenerate identities and invalidate results. Freeze oracles before held-out evaluation.

## 13. Four evaluation tracks

### A. Offline product correctness

Go unit/CLI tests plus a fake `httptest.Server`. No real key needed. Exercise authentication headers, request shape, retries, cancellation, partial/missing answers, probability validation, stdout JSON, snapshot capture, path safety, and cache invalidation. This proves software behavior, not Jev accuracy.

### B. Frozen evidence evaluation

Provide the same complete source bundles to Jev for each case. Measure the decision layer without retrieval errors. Record once, then replay for deterministic CI regression checks. Replaying recorded answers does not re-measure model quality.

### C. End-to-end source-checker evaluation

Give Sure the candidate repo, not prepared evidence. Compare full-scope and incremental modes. Measure selected-contract recall, missing-source detection, evidence volume, and verdicts. This is end-to-end **Sure pipeline** evaluation, not application E2E operation.

### D. Agent repair experiment

Give a separate coding agent a candidate and Sure's structured result. Protect the checker binary, prompts, trusted contracts, and hidden oracle from modification. Allow at most three repair iterations per episode. After each edit, run Sure; at the end, run the independent oracle. Stop and escalate on no progress or unresolved unknowns; do not retry the model until a green answer appears.

Compare a matched control without Sure feedback and the same task with Sure feedback, recording model/version, starting snapshot, task, tool budget, and repetitions. Use predeclared bug and benign-change tasks. Score actual repaired behavior and preservation of other contracts, not whether the agent said it succeeded.

The initial repository need only provide the adapter/episode format and a reproducible manually invoked demonstration. Do not build an autonomous agent orchestration platform before the CLI works.

## 14. Metrics and acceptance

Report counts and denominators, not just one accuracy percentage:

- Violation precision = true violations reported / all reported violations.
- Violation recall = true violations reported / all labeled violations.
- Unsafe-clear rate = violated examples labeled supported / all labeled violations.
- Unknown and operational-error rates, separately.
- Undetected violations: include unknown and not-checked cases; do not hide abstentions from the denominator.
- Selection recall: affected contracts selected / known affected contracts.
- False alarms on safe refactors; localization accuracy where independently labeled.
- Cold/warm wall-clock time, collection/model time, requests, actual input tokens, cache hits, and estimated cost.
- Repair success under the independent oracle and new regressions introduced.

For live stability, run repeated uncached evaluations; cached repeated outputs are not a stability experiment. Start with one pilot pass, then five repetitions for the frozen semantic split once the owner supplies a key and an explicit call/token budget. Report environmental conditions and raw runs. Bootstrap thresholds are unvalidated until measured.

Do not use an LLM to declare the ground-truth label for another LLM. A review-assisting model may suggest cases, but human-reviewed requirements and deterministic oracles set the truth for the measured behaviors.

Go fake-provider CI must pass before calling the provider. Acceptance of v0 requires honest reporting of held-out performance, including poor results. Do not invent a numerical quality target as an achieved result. Do not convert the prototype into a production merge blocker solely because a small demo passes.

## 15. Evaluation libraries: use them only where helpful

Primary recommendation: the Go harness under `tools/eval` owns isolation, snapshots, JSON decoding, and metrics; Vitest owns the independent domain oracle. This avoids making a Node evaluation library a runtime dependency of Sure.

Optional Promptfoo adapter: wrap the binary with a custom provider that captures stdout and expected nonzero Sure exit codes, parses JSON, and returns a result for deterministic assertions. An `exec:` provider treats stdout as text, even when it prints JSON; explicitly decode that output in assertions or use a custom JS provider. Do not misreport an exit-1 contract finding as a provider transport failure. Keep the same canonical corpus across both harnesses.

Optional Stryker: later generate additional TypeScript mutations and measure whether the independent oracle catches them. Its mutation score is not Sure's accuracy. Check version compatibility with the selected Vitest runner. Start with curated edits so meaning, labels, and safe-refactor controls remain understandable.

## 16. Visual evidence extension

Reserve a manifest containing check/scenario ID, artifact hash, source digest (including uncommitted changes), producer identity/version, viewport, locale, theme, and text-size settings. The repo's existing screenshot/preview tools produce it; Sure does not run arbitrary producer commands by default.

For v0, test manifest parsing, stale-source rejection, artifact hash mismatch, path safety, and the unsupported-image outcome with Jev. Do not send pixels/base64 or a model-created caption to Jev and call that visual verification. Textual metadata only supports textual claims. An image-capable provider adapter can be added later without changing contracts or the run-result schema.

A required visual check missing evidence must be unknown. The agent may choose how to produce an approved artifact; it cannot remove the requirement. Screenshots prove only the visible state under the captured configuration, not successful persistence or interaction.

## 17. Implementation order and definition of done

### Milestone 0 — inspect and document

Read the local template, verify the researched references and current Jev API, and record relevant source revisions. Write a short architecture decision record. Preserve ownership/licensing. Do not install an unrelated agent stack.

### Milestone 1 — a runnable slice, offline first

CLI shell, strict contract parser, complete-file collector, fake provider, result schema, and CLI tests. `sure check` works against a tiny fixture with deterministic fake responses. Verify zero target-application execution.

### Milestone 2 — real Jev and safe snapshots

Direct HTTP adapter, schema validation, deadlines/rate backoff, cache/replay, source digests, changed-path selection, trusted contracts, and incomplete coverage. Add the provider request/response fixtures.

### Milestone 3 — clean baseline and corpus

Build `sure-eval`, write independent domain oracles, accept/commit contracts, produce and validate the 40 cases, freeze splits. Keep the source-only model view free of oracle and label leakage.

### Milestone 4 — measurement and repair demo

Run the pilot with an owner-provided key and bounded budget. Capture raw results, inspect false positives/negatives on the development split, then freeze prompts and run held-out evaluations. Produce at least one preserved-behavior refactor and one bug→Sure finding→agent repair→oracle-confirmed recovery episode. Report failures honestly.

### Milestone 5 — release-ready development artifacts

README with scope/non-goals, AGENTS.md/skill, config/contract/output schemas, Make targets, CI, CGO-free build checks, changelog, and release configuration. Prepare release artifacts locally only. A working report and reproducible experiment are more important than a dashboard.

Suggested Make targets (implement and document exact interfaces):

```sh
make dev
make test-race
make eval-offline EVAL_REPO=../sure-eval
make eval-live EVAL_REPO=../sure-eval
make eval-report EVAL_REPO=../sure-eval
```

`eval-live` must require explicit budgets/key; never auto-run from ordinary CI or `make dev`. If no key is provided, finish all offline work and mark live accuracy/latency as unmeasured. Do not fabricate recordings or claim fake-provider numbers describe Jev.

Final implementation handoff must include the two local repository paths, template adaptation summary, actual commands executed, tests passed/failed, source revisions and dependency versions, live vs offline status, known limitations, unresolved unknowns, and raw result locations.

## 18. Sources

See `RESEARCH_NOTES.md` for the reviewed source files and primary documentation. Re-check before implementation if the API or dependencies have changed. Repository descriptions are not independently reproduced performance benchmarks.

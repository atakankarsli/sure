# Sure research notes

Snapshot: **2026-10-11**. Primary documentation and source review only. No Jev key was used, no provider benchmark was run, and neither implementation repository was created or published during this research. All Sure interfaces in the handoff are proposals.

## Existing owner reference

The accessible Go reference is `atakankarsli/adc`, a private owner repository. The exact local Go template the user intends may be different. Read that local template first. These links do not grant access and contain no temporary credentials.

- https://github.com/atakankarsli/adc/blob/main/AGENTS.md — command registry, ffcli, noninteractive operation, output formats, test/build discipline. Reviewed blob: `9295a7e38839629b1c4f7b63906af276c549fd4f`.
- https://github.com/atakankarsli/adc/blob/main/go.mod — observed ff/v3 3.4.0 and Go 1.23 declaration. Blob: `da420aa12b1512a9bff503d501c00e574a6e32bc`.
- https://github.com/atakankarsli/adc/blob/main/cmd/run.go — CLI parsing and exit behavior. Blob: `2c16a97a783e63cbf4f7d680cbb130953351d8c5`.
- https://github.com/atakankarsli/adc/blob/main/internal/cli/shared/output.go — rendering structure; inject writers when adapting. Blob: `5d5b782a53c072e1b15ac5834f821c40c308a791`.
- https://github.com/atakankarsli/adc/blob/main/.goreleaser.yml — CGO-free four-target release structure. Blob: `3f871688e08a3634e196a0e69d3054a08435533c`.

These are **file blob hashes**, not repository commit hashes. Do not put them into commit URLs. The source content is not included in this handoff. Preserve ownership and licensing when reusing files.

## Closest code-first implementations

### Jevlint

- https://github.com/Ice-Hazymoon/jevlint
- https://github.com/Ice-Hazymoon/jevlint/blob/main/README.md
- https://github.com/Ice-Hazymoon/jevlint/blob/main/src/client.ts

Reviewed README blob: `ff71e7ca7f4b5d9695bb49f414861a40a5b84950`. Reviewed client blob: `1dd42d350ba93617ecdd2e87d7ea957ff2a16e73` (first 260 lines).

Relevant mechanisms: plain-English questions, changed/staged scopes, AST candidate filters, replay-only mode, per-question caching, token-budgeted batches, bounded concurrency, shared backoff, valid/invalid examples, and rule drift/recall checks. This is architectural prior art, not proof that a particular threshold or rate setting works in Sure. In particular, its configured request pacing must not be copied as the current TypeSafe service allowance.

### Perch

- https://github.com/lakeday-org/perch
- https://github.com/lakeday-org/perch/blob/main/docs/scan.md

Reviewed scan-doc blob: `a3665db63fae32ea273a9a7e6e46fde358a4a95f` (first 124 lines). It documents syntax parsing, caller/callee context, method-scoped model questions, and content-sensitive reuse. Its bounded call resolution is not a compiler-complete program graph. Useful direction for later evidence selection, not a requirement to reproduce all of Perch before validating Sure.

### Scruple

- https://github.com/NAlexPear/scruple
- https://github.com/NAlexPear/scruple/blob/main/README.md

Reviewed README blob: `f11d7a3b270300806319403eae7d72d125bfda0d`. Useful separation of normalized source evidence, decision providers, and fixed diagnostics. The model need not generate prose for an agent to consume a useful finding. Do not import its TypeScript runtime into a Go product merely to obtain that pattern.

## TypeSafe / Jev — authority for wire behavior

- https://docs.typesafe.ai/api — endpoint, request/response fields and HTTP error classes.
- https://docs.typesafe.ai/primitives/choice — categorical answers, complete distributions, and confidence.
- https://docs.typesafe.ai/primitives/noul — Boolean probability (`noul`), no separate Noul confidence field.
- https://docs.typesafe.ai/models — model identity, input kinds, limits, and alias behavior.
- https://docs.typesafe.ai/model-jaggedness/jev-1.13 — last reviewed 2026-10-02; indirection, numeric precision, irrelevant context, adversarial text, option order, and generation limitations.
- https://docs.typesafe.ai/sdk — documented SDK entry points; a direct HTTP Go adapter avoids an unnecessary runtime bridge.

At inspection, the stable model is `jev-1.13.0`, text/JSON only. The documented limits are 64k tokens per request and 32k for state plus the longest question. Listed public throughput is 100k input tokens/s and 80 requests/s, explicitly subject to change. Listed price is $0.042 per million input tokens; output tokens are free. Do not hard-code throughput as an entitlement or treat model probabilities as calibrated accuracy for this workload.

Questions are independent and share state. Correlation IDs do not carry the question's semantics. The brief's three-way rubric, abstention policy, byte caps, and agent output are **Sure design recommendations**, not TypeSafe guarantees.

## Clean fixture and evaluation references

- https://nextjs.org/docs/app/guides/testing/vitest — official starter and unit-test setup; documents the async Server Component limitation.
- https://github.com/vercel/next.js/tree/canary/examples/with-vitest — minimal example reference.
- https://github.com/vercel/next.js/blob/canary/examples/with-vitest/package.json — reviewed blob `40e46f23b2b2b33598eb5b35fc9f19ef80bd1601`; contains floating `next: latest`. Resolve/pin a compatible dependency set for the corpus, rather than copying this as a version lock.
- https://nextjs.org/docs/app/guides/testing/playwright — optional browser oracle, outside Sure's source check.
- https://www.promptfoo.dev/docs/providers/custom-script/ — `exec:` captures stdout as text; a Sure JSON bridge must handle this explicitly and distinguish findings from harness failures. Page displayed an update date of 2026-10-11.
- https://stryker-mutator.io/docs/stryker-js/vitest-runner/ — optional future mutation generation/oracle assessment; check peer compatibility and runner limitations.
- https://tree-sitter.github.io/tree-sitter/ — incremental syntax trees; runtime is C. Syntax parsing is not semantic type resolution.
- https://go.dev/doc/devel/release — select a supported Go toolchain, rather than copying the template's older declaration blindly.

## Recommendation, not an external fact

Build a narrow Go vertical slice first. Keep the Next.js fixture and oracles separate. Measure the judge given correct evidence, then the whole source-collection pipeline, then actual agent repair. Add richer code indexing only after the first experiment reveals which errors come from missing context rather than the model's judgment.

# Evidence selection and compression for Sure

Research date: **2026-10-18**. Repository baseline: **1c5b4847a38114a5cbe18a7e34bdab7aa0675565**. Status: proposal; no implementation or live evaluation. Numerical engineering estimates below are **unverified**.

## 1. Recommendation

1. Ship **trusted anchors, complete required bodies, and an engine-owned coverage frontier**.
2. Keep M1’s whole-file collector as the baseline and fallback.
3. Add a pinned native Go syntax parser after a small TS/TSX/C# verification spike.
4. Treat name-based edges as candidates; preserve ambiguity and unresolved targets.
5. Follow required dependencies transitively; hop count controls ordering, never completeness.
6. Keep signatures for navigation; hidden required bodies force unknown.
7. Partition requests by check, retaining each check’s complete evidence together.
8. Reparse changed files; re-resolve affected consumers and lookup environments.
9. Reuse decisions only after validating current coverage and the exact request identity.
10. Count whole-suite outcomes; attach byte-backed static paths and separate localization.

**Rejected alternative:** ranked one-hop bodies plus deeper signatures, with Jev deciding that retrieval is complete. Identical signatures can hide opposite behavior. [A][AL] use skeletons to locate source; neither establishes judgment fidelity. Best TS/C# behavioral recall is **unverified**; the recommendation favors explicit coverage over an unmeasured top-k cutoff.

**Decisions requiring owner acceptance; existing rules remain authoritative:**

- [R] Brief sections 4–7 require complete scoped files. Proposed symbol evidence changes that policy in a new contract/collector version; current contracts retain whole-file semantics. Optional per-check anchors must pass a new strict schema and trusted-base review.
- [R] Make bare `sure check` cover the suite; retain explicitly scoped `--changed` and its `no_changes` meaning. Add coverage/path fields through a versioned result schema.
- Replace the hypothesis’s hop cutoff, slice-only cache key and default comment removal as specified below.
- [R] M3 currently covers fixture/corpus construction. Adding parser, anchor and Unity work there is a proposed milestone expansion, not an existing commitment.
- [R][G][W] ADR 0001’s CGO prohibition remains. Prefer native Go; wazero would need an explicit interpretation of the brief’s contradictory “no other runtime” and suggested WASM tier. Do not change transports, model policy, or other files in this research PR.

## 2. Tier ladder

Build costs are **unverified developer-day estimates**, excluding corpus construction.

| Tier | Needs / cost | Catches | Misses / adoption decision | CGO status |
|---|---|---|---|---|
| 0 — whole files | Trusted scope/context; 1–2 days for collection | All allowed bytes within declared files | Cross-boundary behavior; large files exceed budget. Ship M1. | Pure Go |
| 1 — syntax slices | [G] Native parser, three grammars, bounded binding resolver; 1–2-day spike, then 3–5 days TS | Real declaration ranges, callbacks, imports, enclosing context | Dynamic dispatch, aliases, implicit framework roots; broaden or unknown. Primary next tier. | CGO-free upstream build evidence |
| 2 — Unity bindings | [U][UP][CS] C# declarations plus YAML/.meta/profile dependencies; 5–10 days | Partials, serialized references, explicit callback targets | Runtime-created wiring, unresolved variants/reflection. Second-language tier. | Pure Go |
| 3 — external SCIP | [SC][SI] Fresh CI-produced artifact and provenance; 3–5 days importer | Compiler-informed definitions/references, overload/partial binding | Not execution traces or complete Unity wiring; stale artifacts rejected. Defer until unresolved rates justify it. | Go reader; Node/.NET producer outside `check` |
| Navigation only | [CT][A][ST][AG] Tags, ranking, syntax queries | Discovery and prioritization | Ctags/regex are not reliable body/binding certificates; stack-graphs is archived. Do not add another runtime for this. | External tools; unnecessary in binary |
| Heavy analysis | [K][JO][CQ] Kythe compiler extraction; Joern CPG/JDK 21; CodeQL database/query stack | Richer static analysis | Disproportionate integration. CodeQL CLI licensing is separate; documented small-project recommendation is 8 GB/two cores. | External stacks, excluded |

[G] Verified native candidate: `github.com/odvcencio/gotreesitter` at `eb1312afdd82d98490e157e51391febd275f0432`; standalone imports append `/grammars/typescript`, `/grammars/tsx` and `/grammars/c_sharp`. Opened source and successful upstream `js/wasm` and `wasip1/wasm` build jobs establish a no-CGo route. It is a reimplementation with compatibility repairs; Sure-specific parity remains **unverified**.

[W] For upstream Tree-sitter itself, `github.com/zema1/wasitter` at `d4ae75f0856a9a2dc7e29a6b82aac2f088be010f` runs its WASM through wazero. Successful upstream jobs explicitly used `CGO_ENABLED=0`. TS/TSX ship; C# does not. Neither dependency was built locally here.

[SC] Sourcegraph removed LSIF support; do not build an LSIF adapter. A SCIP artifact must match the captured source, configuration, dependencies and producer manifest; unchanged line numbers or a nearby commit do not establish freshness. Otherwise fall back.

## 3. Index design

[SG][C] Sourcegraph’s symbol index re-extracts changed files; Continue separates content artifacts from branch membership. Adopt that separation, not their retrieval completeness assumptions.

Store versioned JSON under `.sure/index/`: an atomic snapshot manifest, content-addressed per-file fact shards, per-file resolution shards, and reverse lookup/check dependencies. Avoid a database initially; keep this inside `internal/evidence` with existing cache support. Persist metadata, not full ASTs or duplicate source.

Apply the production allowlist and benchmark test/oracle/label exclusions **before indexing**; excluded material must not influence ranking, paths, frontiers or cache dependencies. Facts include file/grammar/extractor hashes; declaration kind, qualified identity, signature/body half-open UTF-8 byte spans; enclosing declarations; imports, references, candidate writes and parse health. Edges carry call/reference spans, candidate targets, resolution kind and lookup dependencies. Record unresolved names as **negative dependencies**.

Refresh inventory and capture bytes under brief section 6. Hash current files; re-extract changed content or changed grammar/extractor/profile identities. Full hashing still costs repository reads. Invalidate resolutions for changed imports/exports, name buckets, added overloads/implementations, deletions, aliases, partial membership, defines, manifests or asset bindings. Re-resolve affected unchanged consumers; use both old/new graphs for impact. Configuration uncertainty invalidates conservatively. Changed-file-only *total work* is not a valid promise.

Before decision reuse, require equality of cached and current normalized dependency/coverage manifests: required identities/content, membership, frontier and relevant lookup fingerprints. Merely validating each manifest independently is insufficient. Key by exact normalized state/questions, contract, endpoint/model, policy and collector versions. Keep absolute locations, commit IDs and whole-file/snapshot digests outside that key; reuse across same-file edits requires unchanged relevant state, binding and registration facts. Rebind every location against current bytes. Corrupt/mismatched cache entries are misses; writes are atomic and permission-restricted.

**50k-line estimate, unverified:** assume 2,500 declarations × 320 bytes, 15,000 edges × 160 bytes, plus paths/reverse indexes/manifests: approximately **4–8 MB** JSON. Measure serialized size; neither LOC nor this estimate bounds dense graphs.

## 4. Slice algorithm

**Inputs:** trusted check text, scope/context, optional qualified anchors and explicit boundaries; immutable current/base snapshots, old/new diffs, index/configuration versions. No anchors means whole files. Existing required context files remain mandatory.

**Discover:** anchors seed each check. Changed symbols enter only checks they map to; the entire PR never enters every state. Use lexical/import bindings before repository-wide name candidates. Include callback registrations, framework roots and shared-state writers; outgoing calls alone miss behavior. New unmapped source still makes the run incomplete.

A dependency outside trusted scope/context is an omitted boundary, not authorization to send it. Require an accepted mapping or unknown.

**Expand:** retain full reachable bodies plus signatures, attributes, relevant types/constants, initializers, constructors, getters/setters, captured variables and module effects. Follow candidate dependencies transitively, deduplicating cycles. Include all plausible targets or mark unresolved. Standard-library/external boundaries require versioned trusted assumptions; name resemblance does not establish an intrinsic. Unsupported syntax or parser errors broaden to complete files; unreadable or still-unresolved required behavior yields unknown.

An absent edge is not proof of irrelevance. Every encountered binding/dispatch form must be modeled or bounded by trusted assumptions; unsupported forms broaden declared files or force unknown. Outside-slice edits touching relevant state, name/type lookup buckets or registrations invalidate/broaden the check unless isolation is established. Include the containing environment when writer sets cannot be bounded.

**Compress:** deduplicate exact ranges and omit declarations demonstrated outside the declared dependency boundary. Do not rewrite/minify source or delete branches. A tokenizer can silently misread templates, regexes or expression bodies without “failing”; it cannot authorize slicing. [T] TypeScript comments can declare compiler dependencies, so retain comments initially. Later comment elision needs token-aware classification and byte mappings; it is not injection protection.

**Represent/order:** named JSON fields for assumptions, entry, implementation, relationships and coverage. Put the anchor first, then dependency groups by symbol ID; diff priority affects discovery only, keeping final state independent of PR metadata. IDs derive from qualified identity, not line position; collisions remain distinct candidates. Model-visible edge/region spans are body-relative; current absolute spans belong in external provenance. Paths, strings and comments remain untrusted data.

An advisory elision record contains ID, signature, `body_status=elided` and reason; hashes/ranges stay in index/report provenance. Never render a fake empty body. Omit navigation-only records from final assessment state so irrelevant body changes do not alter requests. The engine records every required omission/unresolved edge, even when the navigation list cannot fit.

**Rounds:** deterministic expansion first; zero model-directed rounds by default. An optional M4 experiment permits at most two advisory expansion rounds using independent usefulness questions or a bounded Choice. Answers can prioritize additions, never remove obligations. `NONE` is not evidence of completeness. Rebuild and budget each request; assess the check only against the final complete state. Remaining required frontier or over-budget closure means `unknown` with the omitted list, irrespective of model probabilities. Never combine slice approvals.

**Outputs:** count unique trusted checks once: `supported + violated + unknown + error + not_checked = total`. Display “N of M checks supported,” preserving selected/evaluated and live/cache/replay counts; no probability averaging. Global unmapped/trust errors remain visible even if N equals M.

For each unsupported check, a fixed template supplies requirement, reason, probabilities (null when unasked), frontier, next actions, and separate path, intersecting changed-hunk and model-selected-region objects. An `x → y → z` pointer requires resolved static edges with actual source/asset spans; it is **not a causal trace**. Ambiguous dispatch makes that pointer null; expose labeled candidate relationships separately. Author assumptions link to trusted contract spans, never invented callsites. No intersecting hunk means null. Deletions point to base bytes. Uncertain Choice localization falls back to file/contract scope. Introduced-regression labeling still requires compatible supported-base/violated-current assessments.

## 5. Budget and token policy

[J][JAG] Keep `jev-1.13.0` and ordinary 64k-request/32k-state-plus-question limits. Current docs also list 1.14 and optional 262k long context, explicitly slower/less accurate; neither changes this design. They recommend filtering irrelevant state and naming relevant fields, but publish no TS/C# size–accuracy curve.

| Policy | Proposed enforcement |
|---|---|
| Initial estimator | `E(x)=UTF-8 bytes of serialized x`, a deliberately pessimistic 1-byte/token proxy; **unverified upper bound** for Jev. Count JSON escaping, metadata and complete questions. Retain M1’s 16 KiB state/24 KiB request caps initially. |
| Headroom after policy review | `E(state)+max E(question) ≤ 28,000`; complete direct request ≤56,000. Gateway complete request ≤28,000 until its 32k catalog semantics are verified. Reserve framing overhead within those totals. |
| Ratio calibration | [JV] Jevlint uses bytes/3. [TE] tokenestimate reports ~10% mean relative error for Kimi-K2, not Jev. No verified Jev error bound. Test bytes/2 as an experiment, not an automatic budget increase. |
| Split / abstain | Batch only useful shared evidence. Split questions by check before dropping code. One indivisible check that still cannot fit returns `unknown/evidence_budget_exceeded`; provider size rejection also abstains, without silent retry-and-crop. |

[JST][JA] Use structured fields and literal per-check instructions. Questions share state but cannot use sibling answers. Record estimates separately from provider usage; calibrate on minified code, long identifiers, Unicode and escaped JSON. Keep gateway routing/version-pin blockers from `jev-wire.md`; do not send source or run live experiments to resolve them in this PR.

[J] At $0.042/million input tokens, 28,000 actual tokens cost $0.001176; prioritize coverage and latency over marginal token savings.

## 6. Case matrix impact

[R] Expected labels remain the matrix’s targets, **not measured predictions**.

| Case | Required protection | Expected outcome |
|---|---|---|
| C013 | Reverse invalidation finds changed failure helper; include unchanged caller and complete dependency chain. | Eligible for violated CHK-001; unresolved chain → unknown. |
| C016 | Include query serializer body, inputs and constants beyond arbitrary hop limits. | Eligible for violated CAT-001. |
| C018 | Preserve full removal predicate/lambda, captures and SKU comparisons. | Eligible for violated CART-001. |
| C020/C026/C029 | Follow extracted helpers/wrappers; preserve propagation and shared-state context. | Safe changes remain eligible for supported. |
| C025/C030 | Comments retained; changed supplied text re-keys. Recompute current byte/line locations on every report. | Supported target; no stale locations. |
| C027 | Use its separately accepted alternate baseline/mapping; rebuild paths/imports and anchors. | Supported target; no phantom absence or candidate-authorized scope change. |
| C031/C032 | Required missing file or oversized required closure cannot disappear through elision. Preserve frozen case construction/labels. | Unknown with missing/omitted list and incomplete exit. |
| C033/C034 | Rebuild source membership before cache lookup. | Unmapped change/incomplete; empty scope unknown. |
| C035/C036 | Capability and artifact-digest checks precede cached assessment. | Unsupported-image/stale-evidence unknown. |
| C037–C040 | Anchors/boundaries/config follow trusted-base protection too. | Report contract changes; reject weakening/endpoint redirection. |

## 7. Unity C# notes

[U][UP] Serialized UnityEvents and prefab overrides make a code-only graph insufficient; Rider already includes asset usages [RD]. Inspector “static/dynamic” describes argument binding, not a C# method modifier.

[UE] Index relevant scenes, prefabs/variants, ScriptableObjects and `.meta` GUID mappings. Resolve GUID **and** local file ID, retaining target component/type, method, arguments, listener mode/state and original YAML bytes. Apply inherited prefab modifications; a regex union of base/override listeners is not the effective configuration. Invalidate dependent assets when a base changes.

[CS] Group partial declarations by assembly, namespace, enclosing type and generic identity. Read supporting members across parts; a large manager’s method alone omits fields, initialization and accessors. Missing generated parts or unknown assembly/define profiles prevent support.

[UD] `SendMessage` can target several attached components; reflection and runtime listeners require receiver/registration evidence. Preservation annotations do not prove invocation.

Contracts must declare relevant entry/lifecycle roots, artifact configurations, assembly/platform/defines, dynamic target sets and external assumptions. Unbounded reflection, dynamic loads, binary artifacts, unresolved prefab overrides or missing bindings yield unknown. The initial C# tier may simply include complete declared artifact records and abstain on unsupported override forms. It must never launch Unity, an exporter, compiler or tests.

## 8. Build order

1. **M1:** ship existing whole-file/fake-provider slice, explicit omissions and count invariants. Do not delay it for indexing.
2. **M2:** implement snapshot/trust/cache provenance and current-byte localization; retain operational and gateway blockers.
3. **M3 (proposed expansion):** run native `CGO_ENABLED=0` parser build/span/parity spike; add reviewed anchor schema, TS/TSX slices, then C# declarations/assets. Keep unsupported forms on fallback.
4. **M4:** compare frozen whole-source and sliced assessments; tune only development cases, freeze policy, then evaluate holdout unchanged.
5. **M5:** ship slicing only with recorded evidence-retention, unsafe-clear and unknown results plus native `CGO_ENABLED=0` cross-build checks. Defer SCIP until measured unresolved cases justify it.

**First experiment:** [R] `eval/app` does not exist yet; today’s `eval/` contains only its README. Once M3 creates it, use checkout baseline, C001, C020 and C025; add development-only inert sibling-method padding, an unrelated same-file edit, and a changed sibling writer/registration that must invalidate reuse. Human-mark required ranges outside candidate copies. Compare compact whole-source, exact-body slices and a signature-only negative ablation; only complete variants may produce gate verdicts. Keep C032 as a genuinely oversized required closure.

Measure required-range recall and complete-set recall, bytes/actual tokens, verdicts/unsafe clears, unknowns, request/cache counts, warm/cold time and location validity. Validate mutations with the independent oracle outside Sure. Missing required ranges or newly unsafe clears block promotion; a small clean pilot is not a reliability guarantee. Live accuracy remains **unmeasured** pending M2 access and an explicit budget.

## 9. Risks and open questions

The difficult boundary is unseen dispatch/shared-state influence, not brace matching. A syntax graph cannot prove completeness beyond its declared language/environment assumptions. No reviewed system supplies validated TS/C# behavior-retention recall for Sure. Parser parity, index size/latency, tokenizer error bounds and expansion utility remain **unverified**.

| Opened evidence | Reported result; not reproduced here | Implication |
|---|---|---|
| [RG], March 2025, outside preferred freshness window | On 300 SWE-bench Lite Python issues, Agentless function-location coverage 51%→54%; generated patches must cover all gold edit locations. One-hop flat: 2,310.7 tokens/29.67% resolved; two-hop: 10,505.3/26%. | Edit localization is not behavioral-code recall; more context did not guarantee better results. |
| [PB], 18 October 2026 | Jev 1.14 method detection: 304/1,506 buggy methods found, 572 false alerts; 177 PRs. Different prompts, labels and bounded-chunk protocol. | Not a Sure or 1.13 accuracy estimate. |
| [UI], September 2026 | 53.4% fewer total tokens, 51.6% less time across 28 questions/two Unity games; one run/arm, index construction excluded, no mechanical correctness grading. | Supports asset-aware retrieval economics only; open-source availability **unverified**. |

[JV][PS][SR] Reviewed Jevlint README/client, Perch scan documentation and Scruple README blobs match the 11 October notes. Perch nevertheless added test-running coverage, graph fixes and ReviewBench work afterward [PH]. Borrow fixed questions and exact-request reuse; exclude test execution and merged partial judgments.

[GF][PP][CR] Graphify-Labs’ code pass is syntax-based with typed provenance; Potpie’s current setup registers a source without scanning and uses harness ingestion; CodeRAG’s bigraph uses generated requirements. None is a substitute for the proposed coverage gate.

## 10. Sources

All URLs below were opened on **2026-10-18**. Tables distinguish published observations from unverified engineering estimates. Repository links use inspected revisions where pinned; otherwise they record the current file read.

| ID | URL(s) and what was read | Access date |
|---|---|---|
| R | [AGENTS](../../AGENTS.md), [brief](../handoff/INITIAL_AGENT_BRIEF.md), [ADR 0001](../adr/0001-foundation.md), [wire](jev-wire.md), [notes](../handoff/RESEARCH_NOTES.md), [cases](../handoff/CASE_MATRIX.md), [examples](../handoff/examples/), [plan](../PLAN.md), [eval](../../eval/README.md), at the baseline above. | 2026-10-18 |
| J/JAG | [Models][J] and [jaggedness][JAG]: current limits/releases and qualitative accuracy guidance. | 2026-10-18 |
| JST/JA | [State][JST], [structured questions][JA], [SDK](https://docs.typesafe.ai/sdk), [fan-out](https://docs.typesafe.ai/patterns/fan-out): independent questions, object fields, direct HTTP. | 2026-10-18 |
| G | [Native parser README][G], [module](https://github.com/odvcencio/gotreesitter/blob/eb1312afdd82d98490e157e51391febd275f0432/go.mod), [TS grammar](https://github.com/odvcencio/gotreesitter/blob/eb1312afdd82d98490e157e51391febd275f0432/grammars/typescript/typescript.go), [C# grammar](https://github.com/odvcencio/gotreesitter/blob/eb1312afdd82d98490e157e51391febd275f0432/grammars/c_sharp/c_sharp.go), [compatibility](https://github.com/odvcencio/gotreesitter/blob/eb1312afdd82d98490e157e51391febd275f0432/docs/compat-tier.md), [build tags](https://github.com/odvcencio/gotreesitter/blob/eb1312afdd82d98490e157e51391febd275f0432/docs/build-tags.md), [CI workflow](https://github.com/odvcencio/gotreesitter/blob/eb1312afdd82d98490e157e51391febd275f0432/.github/workflows/ci.yml), [successful jobs](https://api.github.com/repos/odvcencio/gotreesitter/actions/runs/39015174890/jobs?per_page=100). | 2026-10-18 |
| W | [wasitter README][W], [module](https://github.com/zema1/wasitter/blob/d4ae75f0856a9a2dc7e29a6b82aac2f088be010f/go.mod), [CGO-disabled workflow](https://github.com/zema1/wasitter/blob/d4ae75f0856a9a2dc7e29a6b82aac2f088be010f/.github/workflows/ci.yml), [successful jobs](https://api.github.com/repos/zema1/wasitter/actions/runs/34142835837/jobs?per_page=100). | 2026-10-18 |
| A/AL/RG | [Aider repo map][A]; [Agentless v2][AL], section 3.1 (2024); [RepoGraph v2][RG], sections 5.1–5.2 (2025): navigation, source expansion and metric definitions. Older research is not current Sure validation. | 2026-10-18 |
| C/SG/CY | [Continue refresh implementation][C], [deprecated Codebase](https://docs.continue.dev/reference/deprecated-codebase), [Sourcegraph symbol refresh][SG], [Cody local symf indexing][CY]: extraction reuse versus retrieval. | 2026-10-18 |
| SC/SI | [SCIP design][SC], [schema](https://github.com/scip-code/scip/blob/main/scip.proto), [scip-typescript README][SI], [scip-dotnet README](https://github.com/sourcegraph/scip-dotnet/blob/main/readme.md): transmission, dependencies and positions. Reader must convert declared UTF-16 offsets to real UTF-8 bytes; reference enclosing ranges are not function bodies. | 2026-10-18 |
| CT/AG/T | [Ctags manual][CT], [ast-grep][AG], [Tree-sitter](https://tree-sitter.github.io/tree-sitter/), [TS directives][T]: syntax/navigation capabilities and limits. | 2026-10-18 |
| ST/K/JO | [stack-graphs README][ST] and [archive metadata](https://api.github.com/repos/github/stack-graphs); [Kythe README][K]; [Joern README][JO]: unsupported archive, compiler extractors and JDK requirements. Kythe TS/C# suitability **unverified**. | 2026-10-18 |
| CQ | [CodeQL CLI][CQ] and [hardware](https://docs.github.com/en/code-security/reference/code-scanning/codeql/hardware-resources-for-codeql): database workflow, license availability and costs. | 2026-10-18 |
| GF/PP/CR | [Graphify-Labs v8 mechanisms][GF], [Potpie architecture][PP], [CodeRAG bigraph paper][CR], sections 3/6: actual identities and ingestion/generation requirements. | 2026-10-18 |
| JV/TE | [Jevlint README](https://github.com/Ice-Hazymoon/jevlint/blob/main/README.md), [client][JV]: AST candidates, batches, bytes/3; [tokenestimate][TE]: Kimi-K2 error claim, not a Jev bound. | 2026-10-18 |
| PS/SR | [Perch scan][PS], [Scruple README][SR]: method questions, context clipping/overlapping passes, cache and fixed diagnostics. | 2026-10-18 |
| PH/PB | Perch commits [coverage](https://github.com/lakeday-org/perch/commit/d9486dbe174d21dd971705681e02f2676ae20fb6), [graph fix][PH], [ReviewBench README][PB]: changes after 11 October and method-level results. | 2026-10-18 |
| U/UP/UE | [UnityEvents][U], [prefab YAML][UP], [variants](https://docs.unity3d.com/6000.0/Documentation/Manual/PrefabVariants.html), [GUID/file IDs](https://docs.unity3d.com/6000.0/Documentation/ScriptReference/AssetDatabase.TryGetGUIDAndLocalFileIdentifier.html), [persistent-call source][UE]: serialized/inherited references and target, argument, mode/state fields. | 2026-10-18 |
| CS/UD/RD | [C# partials][CS], [SendMessage][UD], [reflection preservation](https://docs.unity3d.com/6000.0/Documentation/Manual/managed-code-stripping-preserving.html), [defines](https://docs.unity3d.com/6000.0/Documentation/Manual/platform-dependent-compilation.html), [Rider asset usages][RD]. | 2026-10-18 |
| UI | [Unity Insight v1][UI], sections 2–3: asset index, dynamic-load limitations and paired cost study. | 2026-10-18 |

[R]: ../handoff/INITIAL_AGENT_BRIEF.md
[J]: https://docs.typesafe.ai/models
[JAG]: https://docs.typesafe.ai/model-jaggedness/jev-1.13
[JST]: https://docs.typesafe.ai/concepts/state
[JA]: https://docs.typesafe.ai/primitives/advanced
[G]: https://github.com/odvcencio/gotreesitter/blob/eb1312afdd82d98490e157e51391febd275f0432/README.md
[W]: https://github.com/zema1/wasitter/blob/d4ae75f0856a9a2dc7e29a6b82aac2f088be010f/README.md
[A]: https://aider.chat/docs/repomap.html
[AL]: https://arxiv.org/html/2407.01489v2
[RG]: https://arxiv.org/html/2410.14684v2
[C]: https://github.com/continuedev/continue/blob/main/core/indexing/refreshIndex.ts
[SG]: https://sourcegraph.com/docs/code-search/types/symbol
[CY]: https://sourcegraph.com/docs/cody/core-concepts/local-indexing
[SC]: https://github.com/scip-code/scip/blob/main/docs/DESIGN.md
[SI]: https://github.com/sourcegraph/scip-typescript/blob/main/README.md
[CT]: https://docs.ctags.io/en/latest/man/ctags.1.html
[AG]: https://ast-grep.github.io/
[ST]: https://github.com/github/stack-graphs/blob/main/README.md
[K]: https://github.com/kythe/kythe/blob/master/README.adoc
[JO]: https://github.com/joernio/joern/blob/master/README.md
[CQ]: https://docs.github.com/en/code-security/concepts/code-scanning/codeql/codeql-cli
[GF]: https://github.com/Graphify-Labs/graphify/blob/v8/docs/how-it-works.md
[PP]: https://github.com/potpie-ai/potpie/blob/main/docs/context-graph/architecture.md
[CR]: https://arxiv.org/html/2504.10046v1
[T]: https://www.typescriptlang.org/docs/handbook/triple-slash-directives.html
[JV]: https://github.com/Ice-Hazymoon/jevlint/blob/main/src/client.ts
[TE]: https://github.com/infinigence/tokenestimate/blob/main/README.md
[PS]: https://github.com/lakeday-org/perch/blob/main/docs/scan.md
[SR]: https://github.com/NAlexPear/scruple/blob/main/README.md
[PH]: https://github.com/lakeday-org/perch/commit/c6bd61bd784f6336143e58a3199733fad1b6d823
[PB]: https://github.com/lakeday-org/perch/blob/0cff52c70d937fddc838cde1ec6a9c01331c1eb5/.benchmarks/reviewbench/README.md
[U]: https://docs.unity3d.com/6000.0/Documentation/Manual/unity-events.html
[UP]: https://docs.unity3d.com/6000.0/Documentation/Manual/yaml-prefab-serialization.html
[UE]: https://github.com/Unity-Technologies/UnityCsReference/blob/master/Runtime/Export/UnityEvent/UnityEvent.cs
[CS]: https://learn.microsoft.com/en-us/dotnet/csharp/programming-guide/classes-and-structs/partial-classes-and-methods
[UD]: https://docs.unity3d.com/6000.0/Documentation/ScriptReference/GameObject.SendMessage.html
[RD]: https://www.jetbrains.com/help/rider/Features_Unity.html
[UI]: https://arxiv.org/html/2609.27585v1

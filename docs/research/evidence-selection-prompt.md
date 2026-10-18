# Research prompt: evidence selection

Written 2026-10-11 for a web research run. Goal: find better ways than scope globs and a one-hop import graph to
choose the code a decision model sees. Results should land in `docs/research/evidence-selection.md` and feed a
pluggable context provider (input: changed paths + contract; output: ranked regions within a byte budget),
measured in sure-eval (selection recall, unknown rate, tokens, latency).

```text
I'm building "Sure", a Go CLI (CGO-free, single static binary) that checks code changes against plain-English
"flow contracts" (e.g. "When payment fails, the cart keeps its items"). For each contract it collects source code,
sends it to a fast decision model (TypeSafe Jev: choice questions with probability distributions; 64k tokens per
request, 32k for shared state + longest question; $0.042 per 1M input tokens) and reports P(supported) per check,
with file:line regions. It never builds or runs the app. Coding agents run it after every change, as a cheap
replacement for writing and maintaining tests in fast-moving "vibe-coded" projects.

Today, evidence selection is naive: (1) contract authors list scope globs + exact "context" files, and (2) for
ad-hoc checks on a diff we take the changed files plus a one-hop regex import graph (TS/JS/Python, both directions),
capped by a byte budget. In our eval, most misses are "the model didn't see the code that decides the behavior"
(e.g. the change is in loader.ts but the behavior depends on query.ts and api.ts).

Research how to select the RIGHT, SMALL set of code regions to show a decision model so it can judge whether a
behavioral requirement still holds after a change. Cover:

1. Change-impact and context-retrieval approaches: LSP (definitions, references, call hierarchy, type hierarchy),
   SCIP/LSIF indexes, ctags, tree-sitter-based repo maps (e.g. Aider's PageRank repo map), stack graphs, call graphs,
   data-flow / code property graphs (Joern), CodeQL, program slicing, language compilers' APIs (TypeScript compiler
   API, go/packages, Roslyn), embedding/semantic search, and hybrids.
2. How shipping AI code-review and coding tools build context for a diff or a question: CodeRabbit, Greptile,
   Graphite (Diamond), Qodo/PR-Agent, Sourcegraph Cody, Cursor, Continue, Aider, Sweep, GitHub Copilot code review,
   Claude Code / Codex if documented. What they index, when (pre-indexed vs on demand), how they rank and how they
   stay within token budgets. Use primary sources (engineering blogs, docs, papers, source code); flag marketing claims.
3. Integration from Go without cgo: pure-Go libraries, running tools as subprocesses (gopls, typescript-language-server,
   tsserver, pyright, sourcekit-lsp, csharp-ls/OmniSharp for Unity C#, universal-ctags, ast-grep, scip-* indexers),
   tree-sitter grammars compiled to WASM run via a pure-Go runtime (e.g. wazero), or a sidecar service. Note startup
   time, memory, warm vs cold latency on a ~100k-LOC repo, and whether a build or dependency install is required.
4. Languages that matter: TypeScript/JavaScript (Next.js, React Native), Go, Python, Swift, Kotlin, and C# (Unity).
5. Evidence quality: benchmarks and papers on context retrieval for code tasks (e.g. RepoBench, CrossCodeEval,
   SWE-bench retrieval studies, CodeRAG-Bench, change-impact-analysis literature). What measurably helps a model
   judge behavior: full files vs functions vs slices, callers vs callees, depth limits, ranking signals.
6. Risks: stale indexes vs the uncommitted working tree, monorepos, generated code, dynamic dispatch, DI containers,
   reflection (Unity), cost of keeping indexes fresh, licensing (CodeQL terms, GPL tools), security of running
   language servers on untrusted repos (project-configured plugins, build scripts executed on load).

Deliver:
A. A comparison table: approach/tool | what it provides (defs, refs, call graph, data flow, types) | languages |
   offline? | cold/warm latency | setup cost | needs build/install? | license | Go integration path | maturity |
   sources.
B. Per-tool notes with versions, links and dates.
C. A short section on how the AI review tools in (2) actually do it, with citations.
D. A recommended staged plan for Sure: what to add first (cheap, high-yield), second, and later. Include a
   pluggable "context provider" interface sketch: input = changed paths + contract (scope, requirement text),
   output = ranked regions {path, start_line, end_line, reason, score} within a byte budget. Say how each stage
   handles the uncommitted working tree.
E. How to evaluate it: metrics (selection recall of the deciding code, tokens per check, unknown rate, latency)
   and a minimal experiment design.
F. Open questions and where evidence is weak.

Rules: cite primary sources for every non-obvious claim, date each source, say when something is unverified or
vendor-claimed, prefer 2025–2026 material, and don't invent tool names, versions or numbers.
```

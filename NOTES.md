# Notes

Gotcha: The AI SDK calls Jev at `POST https://ai-gateway.vercel.sh/v4/ai/decision-model` with the model in the `ai-model-id` header and state in `stateParts`. Sure uses that route; `/v1/evaluate` also exists but its response shape is undocumented.
Gotcha: The gateway validates the request body before auth and billing. A 400 does not prove the key is valid.
Gotcha: Until the Vercel team has a credit card on file, every gateway call returns `403 customer_verification_required`, even with a valid key.
Gotcha: The gateway's `typesafe-ai/jev` has two upstreams, and `digitalocean` has no zero data retention. Pin routing to `typesafe-ai` before sending real source.
Gotcha: `vercel ai-gateway setup` is unrelated to Sure. It reroutes local coding agents by rewriting `~/.claude/settings.json` and `~/.zshrc`.
Gotcha: Read ADC from its GitHub `main`; a local clone of it may be behind upstream. Never modify ADC.
Gotcha: The Go SDK reads `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` and on-disk profiles by default. Sure builds its client with `option.WithoutEnvironmentDefaults()` and reads only `SURE_ANTHROPIC_API_KEY`.
Gotcha: `anthropic-sdk-go` is pinned to v1.75.0 (published 2026-09-22) on purpose; the newest release was two days old when picked. Bump deliberately.
Gotcha: Claude's reported input tokens are about 5x Sure's estimate (system prompt, output schema and JSON framing). The estimate is a Jev-oriented budget guard, not a cost forecast.
Gotcha: Claude Haiku 5.5 at effort `low` reports p≈0.80–0.85 for code that does satisfy a requirement, so the 0.90 policy turns those into `unknown`. That is the policy working, not a bug.
Gotcha: In `httptest`, a handler that blocks on `r.Context().Done()` can hang `Server.Close()`. Register the cleanup that releases the handler after starting the server, because `t.Cleanup` runs last-in, first-out.
Gotcha: Context files alone never make a contract checkable; a scope that matches nothing is `empty_scope` even when its context files exist.
Gotcha: Claude rarely lists every option of a long choice and its totals drift (0.96–1.04). The Claude adapter completes and rescales; never apply that to Jev.
Gotcha: Go's flag package stops at the first positional argument, so `sure checkout --json` lost `--json` until `shared.ParseInterspersed`.
Gotcha: Vitest resolves `--config` relative to `--root`; sure-eval passes the candidate root through `ORACLE_ROOT` instead.
Gotcha: Without import following, ad-hoc checks on a diff mostly come back unsure: the model sees only the changed file.
Gotcha: TypeSafe's long context is a request header (`TypeSafe-Danger-Zone-Long-Context: true`), not a model ID; it works on `jev-1.13.0` and raises both limits to 262k tokens at the same price. A 159k-token request answered in 2.5 s.
Gotcha: Wide evidence makes Jev miss bugs. On a Unity game's core assembly (C#, 60 files, 159k tokens) cleared a planted day-rollover bug with P(supported) 1.00 that a 23k-token feature scope kept unsure. Keep scopes feature-sized; `--long-context` is for evidence that does not fit.
Gotcha: `estimateTokens` (bytes/3) overestimates C# by about 18% (187.7k estimated, 158.8k billed). The default 64 KiB `max_state_bytes` is reached before 32k tokens on C#: three Core folders (73 KB, 25k tokens) are over budget without `--long-context`.
Gotcha: Ad-hoc checks on C# read only the changed files: `using` directives are not followed, so a diff-only check comes back unknown for anything in another file. Pass `--scope` or write contracts.
Gotcha: `--dry-run --long-context` still needs `TYPESAFE_API_KEY` set, because the transport is resolved from the keys and only TypeSafe serves long context.

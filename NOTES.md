# Notes

Gotcha: The Vercel AI Gateway serves Jev at `POST /v1/evaluate`, not `/v1/systemone`. Its boolean question type is `boolean`, not TypeSafe's `noul`.
Gotcha: The gateway validates the request body before auth and billing. A 400 does not prove the key is valid.
Gotcha: Until the Vercel team has a credit card on file, every gateway call returns `403 customer_verification_required`, even with a valid key.
Gotcha: The gateway's `typesafe-ai/jev` has two upstreams, and `digitalocean` has no zero data retention. Pin routing to `typesafe-ai` before sending real source.
Gotcha: `vercel ai-gateway setup` is unrelated to Sure. It reroutes local coding agents by rewriting `~/.claude/settings.json` and `~/.zshrc`.
Gotcha: The local ADC clone (`~/work/Tools/aseprite-cli/reference-repos/adc`) is behind upstream. Read it with `git show origin/main:<path>`, and never pull or edit it.
Gotcha: The Go SDK reads `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN` and on-disk profiles by default. Sure builds its client with `option.WithoutEnvironmentDefaults()` and reads only `SURE_ANTHROPIC_API_KEY`.
Gotcha: `anthropic-sdk-go` is pinned to v1.75.0 (published 2026-09-22) on purpose; the newest release was two days old when picked. Bump deliberately.
Gotcha: Claude's reported input tokens are about 5x Sure's estimate (system prompt, output schema and JSON framing). The estimate is a Jev-oriented budget guard, not a cost forecast.
Gotcha: Claude Haiku 5.5 at effort `low` reports p≈0.80–0.85 for code that does satisfy a requirement, so the 0.90 policy turns those into `unknown`. That is the policy working, not a bug.
Gotcha: In `httptest`, a handler that blocks on `r.Context().Done()` can hang `Server.Close()`. Register the cleanup that releases the handler after starting the server, because `t.Cleanup` runs last-in, first-out.
Gotcha: Context files alone never make a contract checkable; a scope that matches nothing is `empty_scope` even when its context files exist.

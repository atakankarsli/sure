# Notes

Gotcha: The Vercel AI Gateway serves Jev at `POST /v1/evaluate`, not `/v1/systemone`. Its boolean question type is `boolean`, not TypeSafe's `noul`.
Gotcha: The gateway validates the request body before auth and billing. A 400 does not prove the key is valid.
Gotcha: Until the Vercel team has a credit card on file, every gateway call returns `403 customer_verification_required`, even with a valid key.
Gotcha: The gateway's `typesafe-ai/jev` has two upstreams, and `digitalocean` has no zero data retention. Pin routing to `typesafe-ai` before sending real source.
Gotcha: `vercel ai-gateway setup` is unrelated to Sure. It reroutes local coding agents by rewriting `~/.claude/settings.json` and `~/.zshrc`.
Gotcha: The local ADC clone (`~/work/Tools/aseprite-cli/reference-repos/adc`) is behind upstream. Read it with `git show origin/main:<path>`, and never pull or edit it.

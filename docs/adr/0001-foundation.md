# ADR 0001 — Foundation

Date: 2026-10-11. Status: accepted by the owner.

The brief in `docs/handoff/` is the spec. This record lists where the project adapts or overrides it, and why.

## 1. One repository, with the evaluation as a module

The brief (§3) proposes two repositories, `sure` and `sure-eval`. The owner chose one repository: `sure`, with the evaluation under `eval/`.

The brief split them so that labels and oracles could never leak into the model's evidence. That separation now comes from process boundaries, not repository boundaries:

- `eval/app/` is a fixture. For each case the harness copies it into a fresh temporary Git repository outside this repo, applies the case patch there, and runs `sure check --repo <copy>`. Sure never sees this repository's tree.
- Labels, case titles, patch names and oracle results live under `eval/corpus/` and the harness. They are never copied into the candidate.
- The oracle runs after Sure on the same candidate. Sure's source allowlist (`src/` only, tests excluded) keeps oracle files out of evidence even when they are present.
- Go ignores `eval/app/node_modules` through the `ignore` directive in `go.mod` (Go 1.25+). The Node toolchain stays inside `eval/app/`; the Sure binary has no Node dependency.
- If Sure is ever run on this repository itself, its config excludes `eval/`.

## 2. Template: ADC's architecture, not its product

Template: `github.com/atakankarsli/adc` at `origin/main` `0d954e7`. Keep ffcli v3, the command registry, shared output and sentinel errors, exit-code mapping, Make targets and the CGO-free GoReleaser matrix. Inject writers and dependencies; no package-global flag state; `os.Exit` only in `main`. Leave every Apple, WWDC, skills, scraping and anti-detection part behind. Details are in `AGENTS.md`.

ADC is the owner's MIT-licensed code. Sure is private and has no licence file yet. If Sure is ever published, add the MIT notice for the adapted files.

## 3. Toolchain

- Go 1.26 (`go 1.26.0` in `go.mod`; installed toolchain 1.26.4). ADC's `go 1.23.0` is not carried over.
- `CGO_ENABLED=0` for every build. Tree-sitter or anything else needing cgo needs a new ADR.
- Initial dependencies: `github.com/peterbourgon/ff/v3`. The YAML parser must reject duplicate keys (`gopkg.in/yaml.v3` does); the Markdown approach (restricted grammar or a parser) is decided in Milestone 1.

## 4. Provider: Jev with two transports

One `jev` provider, two transports behind the same interface, plus `replay` and a test-only `fake`:

| Transport | Endpoint | Key env | Model | Bool question type |
|---|---|---|---|---|
| `gateway` (default) | `POST https://ai-gateway.vercel.sh/v1/evaluate` | `AI_GATEWAY_API_KEY` | `typesafe-ai/jev` | `boolean` |
| `typesafe` | `POST https://api.typesafe.ai/v1/systemone` | `TYPESAFE_API_KEY` | `jev-1.13.0` | `noul` |

The engine speaks one internal question model (`choice`, `bool`); each transport maps it to its own wire format and validates its own responses. Verified facts and open unknowns are in `docs/research/jev-wire.md`.

Trust rules (these narrow the brief's example config):

- Endpoints are compiled in. Repository config cannot set a URL. An approved enterprise base URL may come only from user-level config or the environment, never from the repository.
- The key env var name is fixed per transport. The example config's `key_env` field is dropped: a repository could otherwise point it at an unrelated secret and send that secret to the provider.
- In protected mode the transport choice comes from the trusted base, like the rest of the policy.
- Requests reject cross-origin redirects.
- Before real source goes through the gateway, routing must be pinned to the `typesafe-ai` upstream (zero data retention). The gateway also lists a `digitalocean` upstream without it.

## 5. The `vercel ai-gateway setup` command is not used

`vercel ai-gateway setup` connects local coding agents to the gateway. It rewrites `~/.claude/settings.json` and `~/.zshrc` for every session on this Mac. Sure does not need it. The gateway key is used only by Sure's live runs, through `AI_GATEWAY_API_KEY`.

## 6. Hosting

Private GitHub repository `atakankarsli/sure`. No releases, public mirrors, package publishing or Homebrew tap without the owner's go-ahead.

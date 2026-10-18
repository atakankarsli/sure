# ADR 0001 — Foundation

Date: 2026-10-11. Status: accepted by the owner. Revised the same day: the evaluation moved out of this repository, and a Claude provider was added for development.

The brief in `docs/handoff/` is the spec. This record lists where the project adapts or overrides it, and why.

## 1. This repository holds only the Sure tool

The brief (§3) proposes two repositories, `sure` and `sure-eval`. This repository is `sure`: the CLI, its unit tests and its docs. The evaluation (the fixture shop, oracles, the 40-case corpus, the repair episodes) will live in a separate repository, built later. Nothing in this repository depends on it.

The brief's milestones 3 and 4 move with it. Sure's own tests run offline against fake and `httptest` providers and small fixtures under `testdata/`. They prove the software behaves; they say nothing about model accuracy.

## 2. Template: ADC's architecture, not its product

Template: `github.com/atakankarsli/adc` at `origin/main` `0d954e7`. Keep ffcli v3, the command registry, shared output and sentinel errors, exit-code mapping, Make targets and the CGO-free GoReleaser matrix. Inject writers and dependencies; no package-global flag state; `os.Exit` only in `main`. Leave every Apple, WWDC, skills, scraping and anti-detection part behind. Details are in `AGENTS.md`.

ADC is the owner's MIT-licensed code. Sure is private and has no licence file yet. If Sure is ever published, add the MIT notice for the adapted files.

## 3. Toolchain

- Go 1.26 (`go 1.26.0` in `go.mod`; installed toolchain 1.26.4). ADC's `go 1.23.0` is not carried over.
- `CGO_ENABLED=0` for every build. Tree-sitter or anything else needing cgo needs a new ADR.
- Runtime dependencies: `peterbourgon/ff/v3` (CLI), `gopkg.in/yaml.v3` (rejects duplicate keys), `bmatcuk/doublestar/v4` (globs), `anthropics/anthropic-sdk-go` (Claude provider). Test-only: `santhosh-tekuri/jsonschema/v6` to check output against the published schema.
- Contracts use a deliberately restricted Markdown grammar, not a general Markdown parser.

## 4. Providers

Three decision providers sit behind one interface, plus `replay` and a test-only `fake`:

| Provider | Transport | Endpoint | Key env | Default model | Probabilities |
|---|---|---|---|---|---|
| `jev` | `gateway` (default) | `POST https://ai-gateway.vercel.sh/v1/evaluate` | `AI_GATEWAY_API_KEY` | `typesafe-ai/jev` | model distribution |
| `jev` | `typesafe` | `POST https://api.typesafe.ai/v1/systemone` | `TYPESAFE_API_KEY` | `jev-1.13.0` | model distribution |
| `anthropic` | `messages` | `POST https://api.anthropic.com/v1/messages` | `SURE_ANTHROPIC_API_KEY` | `claude-haiku-5-5` | self-reported |

- The engine speaks one internal question model (choice questions with ordered options). Each provider maps it to its own wire format and validates its own responses. Jev facts are in `docs/research/jev-wire.md`.
- Jev returns a probability distribution computed by the model. Claude is asked, through structured output, for a probability per option; those numbers are self-reported and uncalibrated. Reports say which kind they carry (`probability_source`). The same acceptance policy applies to both.
- The Claude provider exists so development and trials can run now, while the gateway is blocked on billing. It uses the official Go SDK with explicit options: key, base URL and retries are set by Sure, never picked up implicitly from `ANTHROPIC_*` variables.
- Its key variable is `SURE_ANTHROPIC_API_KEY`, not `ANTHROPIC_API_KEY`. Exporting `ANTHROPIC_API_KEY` in a shell would also change how Claude Code authenticates on this Mac. The dev key is stored in the macOS Keychain as `sure-anthropic-api-key`.

Trust rules (these narrow the brief's example config):

- Endpoints are compiled in. Repository config cannot set a URL. `SURE_PROVIDER_BASE_URL` (user environment, `https` only) may point the selected provider at an approved alternative; reports show the endpoint used.
- The key variable name is fixed per provider. The example config's `key_env` field is dropped: a repository could otherwise point it at an unrelated secret and send that secret to the provider.
- Repository config chooses the provider, transport and model. In protected mode that choice comes from the trusted base. The person running `sure check` may override the provider with `--provider`.
- HTTP redirects are refused.
- Gateway requests carry `providerOptions.gateway.only: ["typesafe-ai"]` to keep source on the zero-data-retention upstream. The gateway accepts the field; whether it honors it is unverified until a live call succeeds.

## 5. The `vercel ai-gateway setup` command is not used

`vercel ai-gateway setup` connects local coding agents to the gateway. It rewrites `~/.claude/settings.json` and `~/.zshrc` for every session on this Mac. Sure does not need it. The gateway key is used only by Sure's live runs, through `AI_GATEWAY_API_KEY`.

## 6. Hosting

Private GitHub repository `atakankarsli/sure`. No releases, public mirrors, package publishing or Homebrew tap without the owner's go-ahead.

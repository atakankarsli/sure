# Jev wire format

Checked 2026-10-11. Re-check before Milestone 2.

## TypeSafe direct (from docs.typesafe.ai/api)

`POST https://api.typesafe.ai/v1/systemone`, headers `Authorization: Bearer <key>` and `Content-Type: application/json`.

Request:

```json
{
  "model": "jev-1.13.0",
  "state": "<string | object | array>",
  "questions": {
    "<id>": { "type": "choice", "instructions": "<string | object | array>", "criteria": { "<option>": "<description or null>" } },
    "<id>": { "type": "noul", "instructions": "...", "criteria": { "true": "...", "false": "..." } },
    "<id>": { "type": "score", "instructions": "...", "criteria": ["<level>", "<level>"] }
  }
}
```

- Question keys are correlation IDs and are not shown to the model. The requirement text must be inside `instructions`.
- `choice`: `criteria` required, at most 255 options. `noul`: `criteria` optional. `score`: 2 to 10 levels.

Response:

```json
{
  "model": "jev-1.13.0",
  "answers": {
    "<id>": { "type": "choice", "choice": "<option>", "probabilities": { "<option>": 0.0 }, "confidence": 0.0 },
    "<id>": { "type": "noul", "noul": 0.0 },
    "<id>": { "type": "score", "score": 0.0, "legend": {}, "probabilities": {}, "confidence": 0.0 }
  },
  "usage": { "input_tokens": 0, "output_tokens": 0 }
}
```

- `model` is the versioned model that answered, even when the request used an alias such as `jev-latest`.
- Errors: `401` bad key, `422` validation (the body names the field), `429` rate limit, `529` overloaded.

Limits (from docs.typesafe.ai/models): 64k tokens per request; 32k for state plus the longest question. Price: $0.042 per million input tokens; output is free. Text/JSON only.

## Vercel AI Gateway (probed directly)

Model catalog (`GET https://ai-gateway.vercel.sh/v1/models`, no auth):

- ID `typesafe-ai/jev`, type `evaluation`, context window 32000, input $0.042/M, output $0, `supported_specifications: ["v4"]`.
- Two upstreams (`/v1/models/typesafe-ai/jev/endpoints`): `typesafe-ai` with ZDR, and `digitalocean` **without** ZDR.

Probes with a synthetic two-question body (no source code):

| Request | Result |
|---|---|
| `POST /v1/systemone` | `404 not_found_error` |
| `POST /v1/evaluate` with a `noul` question | `400 invalid_request_error`: `questions.q2.type: Invalid discriminator value. Expected 'boolean' \| 'choice' \| 'score'` |
| `POST /v1/evaluate` with `boolean` | `403 customer_verification_required`: the Vercel team needs a credit card on file |

What this shows:

- The gateway's Jev route is `/v1/evaluate`, with `Authorization: Bearer <AI_GATEWAY_API_KEY>`.
- The boolean question type is `boolean`, not `noul`.
- The body is validated before auth or billing. A 400 does not prove the key is valid.
- The key passed authentication and stopped at the billing check.

## Unknown until a live call succeeds

- The gateway's response shape, especially the field name for a `boolean` answer, and whether `choice` answers match TypeSafe's.
- Whether a version can be pinned through the gateway (for example `typesafe-ai/jev-1.13.0`) and what `model` comes back. Until then, record `returned_model` and flag any change.
- How to pin the upstream to `typesafe-ai` on `/v1/evaluate`, for example through the gateway's provider options.
- Gateway error body shapes for `401`, `422`, `429` and `529`, plus rate-limit and `Retry-After` headers.
- Whether cost headers (`X-Cost-USD` or similar) come back.

## Unverified third-party claims

- An Apidog article says Jev is reachable through the AI SDK's `experimental_evaluate` from AI SDK 7.0.105. Not confirmed in Vercel's own docs, and not needed: Sure calls HTTP directly.
- Zuplo's gateway forwards `/v1/systemone` bodies unchanged with model IDs like `jev/jev-1.13.0`. That is Zuplo, not Vercel.

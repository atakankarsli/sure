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

## Vercel AI Gateway — the route Sure uses

Source: the AI SDK's gateway provider (`@ai-sdk/gateway` 4.0.110, used by `ai` 7.0.137 in Vercel's eve framework) and the decision model spec in `@ai-sdk/provider` 4.0.26. Request validation was confirmed by live probes; responses are blocked by billing until the Vercel team has a card.

`POST https://ai-gateway.vercel.sh/v4/ai/decision-model` with headers `Authorization: Bearer <AI_GATEWAY_API_KEY>`, `ai-gateway-protocol-version: 0.0.1`, `ai-gateway-auth-method: api-key`, `ai-decision-model-specification-version: 4`, `ai-model-id: typesafe-ai/jev`.

```json
{
  "stateParts": [{ "type": "json", "value": { "...": "..." } }],
  "questions": { "<id>": { "type": "choice", "instructions": "<string|object|array>", "criteria": { "<option>": "<description or null>" } } },
  "providerOptions": { "gateway": { "only": ["typesafe-ai"] } }
}
```

Response (AI SDK `DecisionModelV4Result`):

```json
{
  "answers": { "<id>": { "type": "choice", "choice": "<option>", "probabilities": { "<option>": 0.0 } } },
  "model": "<optional>",
  "rounding": { "probabilityDecimals": 3 },
  "usage": { "inputTokens": 0, "outputTokens": 0 },
  "warnings": [],
  "providerMetadata": {}
}
```

- An answer may be `{ "type": "refusal" }`; Sure reports that check as unknown (`model_refused`).
- `probabilities` is optional in the spec, but Sure requires it and treats its absence as malformed.
- There is no `confidence` field; TypeSafe's confidence, if any, sits in `providerMetadata`.
- The model ID travels in a header, not the body.
- The gateway also accepts `providerOptions.gateway.models` with conditional fallbacks (`confidenceBelow`, `probabilityBetween`). Sure does not use them.

Earlier probes (2026-10-11) of the older `/v1/evaluate` route: it validates `model`, `state`, `questions` with types `boolean | choice | score`, and validates the body before auth and billing. Sure no longer uses it.

Catalog: `typesafe-ai/jev`, type `evaluation`, context 32000, $0.042 per million input tokens, output free. Upstreams: `typesafe-ai` (zero data retention) and `digitalocean` (none).

## Unverified third-party claims

- An Apidog article says Jev is reachable through the AI SDK's `experimental_evaluate` from AI SDK 7.0.105. Not confirmed in Vercel's own docs, and not needed: Sure calls HTTP directly.
- Zuplo's gateway forwards `/v1/systemone` bodies unchanged with model IDs like `jev/jev-1.13.0`. That is Zuplo, not Vercel.

---
schema_version: sure.contract/v1
id: checkout.failure
severity: error
evidence: code
scope:
  - src/features/checkout/**
context:
  - src/domain/orders.ts
  - src/domain/cart.ts
---
# Checkout failure

## CHK-001 — Preserve the cart
Given a non-empty cart, when order submission resolves with an error,
existing cart items and quantities must remain unchanged.

## CHK-002 — Do not show success
When order submission fails, the application must not transition to
the order-success state.

## CHK-003 — Allow another attempt
After a failed submission settles, submitting must become false so
the customer can retry.

# eval

The evaluation module. It measures whether Sure's verdicts are right. `sure check` never reads anything here as evidence. Built in Milestone 3; see `docs/PLAN.md`.

## Layout (target)

```text
eval/
  app/                 # the fixture: small Next.js shop (own package.json + lockfile)
    src/
      app/             # App Router UI
      features/{catalog,cart,checkout}/
      domain/          # plain TypeScript behavior
      server/          # in-memory services, no external accounts
    .sure/
      config.yaml
      contracts/       # accepted contracts, committed before any mutation
    oracle/
      unit/            # independent Vitest expectations
      browser/         # optional small Playwright suite
  corpus/
    cases.jsonl        # opaque IDs, split, baseline identity, patch hash, targets, oracle mapping
    patches/           # one replayable patch per case
    splits.json        # frozen dev / holdout
    labels/            # expected outcomes, harness-only
  harness/             # Go, part of the root module (tools behind `make eval-*`)
  episodes/            # agent repair episode format (max 3 iterations)
  runs/                # raw Sure JSON and oracle output, gitignored
```

## Rules

- The harness copies `app/` into a fresh temporary Git repo outside this repository for each case, commits the baseline, applies the patch, then runs `sure check --repo <copy>`. It deletes only the copy it created.
- The candidate copy never contains labels, case titles, patch names, oracle logs or previous results. Model requests carry none of them.
- Sure runs first. The harness then runs the oracle on the same candidate and scores against frozen labels.
- A mutant must typecheck and fail its oracle for the intended reason. A safe change must keep every oracle green. Anything else is an invalid case, not a detection.
- Never tune on holdout. Never weaken a contract or oracle to make a check pass.
- Pin every dependency (no `next: latest`), Node and the package manager. Commit the lockfile before building the corpus.

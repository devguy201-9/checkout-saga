# Plan: <Topic>

> Copy this template to plan a piece of work. Save as `docs/plans/<topic>.md`.

## Context

<!-- Short summary: what this builds and where it fits -->

## Goal

<!-- One clear sentence: what must be achieved. Specific + verifiable -->

Example: "`POST /orders` returns 201 with idempotency working, verified by 100 duplicate-key requests → 1 row inserted."

## Out of scope

<!-- List explicitly what is NOT being done here to avoid scope creep -->

## Tasks

- [ ] Task 1 — concrete
- [ ] Task 2
- [ ] Task 3
- ...

## Deliverables

- [ ] Code: `<paths>`
- [ ] Migration: `<paths>`
- [ ] Tests: `<paths>`
- [ ] Update `docs/codebase-summary.md` if the structure changed
- [ ] Update `docs/checkpoints/<topic>.md` with verify steps
- [ ] ADR (if a new decision)

## Verify checkpoint

Sequential commands; each must pass before the next:

```bash
make lint
make test
# task-specific verifies
```

## Risks / unknowns

<!-- Things not yet certain that could block. List to track -->

## Notes during execution

<!-- Update while building. Most important: record decisions, gotchas, links read -->

## Reflection

<!-- Fill after done: what got done, what got stuck -->

- Stuck points: ...
- Next: ...

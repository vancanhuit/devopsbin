# Architecture decision records

This directory captures the rationale behind architectural decisions
that would otherwise have to be re-derived from the codebase. Each ADR
is immutable: once a decision is accepted, the file is not edited; a
later decision that supersedes it is recorded as a new ADR that links
back.

## Format

We use a lightweight Michael Nygard–style template:

```
# ADR NNNN: <title>

- Status: <Proposed | Accepted | Superseded by ADR-XXXX>
- Date: YYYY-MM-DD

## Context
<the forces at play>

## Decision
<the choice we made>

## Consequences
<positive, negative, neutral fallout>
```

## Index

| ADR | Title | Status |
| --- | ----- | ------ |
| [0001](0001-account-transfer-isolation-retry.md) | Serializable, retrying transactions for account transfers | Accepted |

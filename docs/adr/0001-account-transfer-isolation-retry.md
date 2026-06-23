# ADR 0001: Serializable, retrying transactions for account transfers

- Status: Accepted
- Date: 2025-01-15

## Context

The `POST /transfer` endpoint moves money between two accounts. It is the
canonical correctness demo for the service: a transfer must be **atomic** (never
debit without the matching credit), **isolated** (concurrent transfers must not
lose or double-spend), and **authorized** (only the owner of the source account
may move its funds). The work is a small, fixed unit — lock two rows, check
ownership and balance, debit, credit, and append a ledger row — but doing it
correctly under concurrency is the whole point.

The forces at play:

- **Lost updates / double-spend.** Two concurrent transfers from the same
  account that each read the balance, decide it is sufficient, and then write
  can both succeed and overdraw it. A naive read-modify-write is wrong.
- **Deadlocks.** If transfer A locks account 1 then account 2 while transfer B
  locks account 2 then account 1, the two can deadlock. Lock acquisition order
  matters.
- **Isolation level trade-offs.** `READ COMMITTED` (Postgres' default) does not
  prevent the lost-update anomaly on its own; `SERIALIZABLE` does, but can abort
  a transaction with a `40001` serialization failure that the application must be
  prepared to retry.
- **Authorization is a domain rule, not just a gate.** Ownership of the source
  account has to be checked against the row that is actually being debited, under
  the same lock, so it cannot race with a concurrent change.

The options considered:

- **Application-level optimistic concurrency** (a `version` column, compare-and-
  swap on write). Works, but spreads the invariant across multiple statements and
  re-introduces a retry loop anyway.
- **`READ COMMITTED` with `SELECT ... FOR UPDATE`.** Row locks prevent the
  lost-update on the locked rows and never produce `40001`, but rely entirely on
  correct, deterministic lock ordering to avoid deadlocks and give weaker
  guarantees for any read not covered by a lock.
- **`SERIALIZABLE` with `SELECT ... FOR UPDATE` and a retry loop.** Strongest
  correctness guarantee; the database detects anomalies and the application
  replays the whole transaction on conflict.

## Decision

Run each transfer in a single transaction at **`SERIALIZABLE`** isolation by
default, and **retry the whole transaction** on a transient `40001`
(serialization failure) or `40P01` (deadlock detected) up to a fixed budget
(5 attempts), surfacing `ErrRetriesExhausted` → `409` if the budget is exhausted.

Within the transaction:

1. Lock **both** accounts `FOR UPDATE` in a single query ordered by id
   (`WHERE id = ANY(...) ORDER BY id FOR UPDATE`). Locking in a deterministic id
   order means two transfers touching the same pair can never deadlock on lock
   ordering, even at weaker isolation levels.
2. Verify the source and destination exist (`ErrAccountNotFound` → `404`).
3. Verify the session user owns the source account, under the lock
   (`ErrNotAccountOwner` → `403`).
4. Verify sufficient funds (`ErrInsufficientFunds` → `409`).
5. Debit the source, credit the destination, and append a ledger row.

The isolation level is overridable per request via `?isolation=` and a delay can
be injected inside the transaction via `?holdMs=` purely to make the contention
window observable for the demo; neither changes the default production behaviour.
Authorization is enforced in the store, against the locked row, so it cannot
race a concurrent change. The endpoint is session-protected and CSRF-protected
(it is a state-changing request on an authenticated route).

## Consequences

Positive:

- Concurrent transfers cannot lose updates or double-spend: the database detects
  serialization anomalies and the retry loop resolves them transparently.
- Deterministic lock ordering eliminates the classic transfer deadlock, so the
  retry budget is spent on genuine serialization conflicts, not lock cycles.
- The money-movement invariant (atomic debit+credit+ledger) and the ownership
  check live together in one transaction, so partial application is impossible.

Negative:

- `SERIALIZABLE` costs more than `READ COMMITTED` and can abort under contention,
  so the application must carry a retry loop and a bounded budget. Pathological
  contention surfaces as a `409` rather than blocking forever.
- A retry replays the whole unit of work; the in-transaction `?holdMs=` hold, if
  used, is paid on each attempt.

Neutral:

- The retry budget and default isolation are constants in the store rather than
  configuration; they can be promoted to config later without changing callers.
- `FOR UPDATE` makes most same-pair transfers serialize on row locks rather than
  abort, so observed `40001` retries are most visible under genuinely concurrent,
  overlapping access (which `?holdMs=` exists to demonstrate).

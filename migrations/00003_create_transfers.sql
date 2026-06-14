-- +goose Up
-- +goose StatementBegin
CREATE TABLE transfers (
    id              uuid         PRIMARY KEY DEFAULT uuidv7(),
    from_account_id uuid         NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    to_account_id   uuid         NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    amount_cents    bigint       NOT NULL CHECK (amount_cents > 0),
    created_at      timestamptz  NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX transfers_from_account_id_idx ON transfers (from_account_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX transfers_to_account_id_idx ON transfers (to_account_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE transfers;
-- +goose StatementEnd

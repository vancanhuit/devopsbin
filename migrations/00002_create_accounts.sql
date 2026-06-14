-- +goose Up
-- +goose StatementBegin
CREATE TABLE accounts (
    id            uuid         PRIMARY KEY DEFAULT uuidv7(),
    user_id       uuid         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name          text         NOT NULL,
    balance_cents bigint       NOT NULL DEFAULT 0 CHECK (balance_cents >= 0),
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX accounts_user_id_idx ON accounts (user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE accounts;
-- +goose StatementEnd

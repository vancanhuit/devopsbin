-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id            uuid         PRIMARY KEY DEFAULT uuidv7(),
    username      text         NOT NULL UNIQUE,
    password_hash text         NOT NULL,
    role          text         NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE users;
-- +goose StatementEnd

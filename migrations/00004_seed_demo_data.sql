-- +goose Up
-- Seed two demo users with starter accounts so the auth and transfer demos are
-- usable out of the box. The bcrypt password hashes (cost 12) are precomputed
-- offline and embedded as literals; the demo plaintext passwords are documented
-- in the README and the auth ADR (demo only — these are not secrets).
--
--   alice / alicepass  (role: user)
--   admin / adminpass  (role: admin)
-- +goose StatementBegin
INSERT INTO users (username, password_hash, role) VALUES
    ('alice', '$2a$12$uKlXsyVgFo/kZVeC/2COyud4ag75eD2.bpMznJLk0Tk/G7ZAw8Nxe', 'user'),
    ('admin', '$2a$12$FNGDbky64vV0adDK4Nlfj.0nCR6gn5gMvIso7k2/Xst/nW0sutiSe', 'admin');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO accounts (user_id, name, balance_cents)
SELECT id, 'Checking', 100000 FROM users WHERE username IN ('alice', 'admin');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM users WHERE username IN ('alice', 'admin');
-- +goose StatementEnd

// Package migrations embeds the SQL migration files so the running binary can
// apply them via `devopsbin migrate` without any external files. The files are
// goose-format (-- +goose Up / -- +goose Down) and are applied by the
// internal/migrate package using the goose Provider API.
package migrations

import "embed"

// FS holds the embedded goose migration files (NNNNN_description.sql).
//
//go:embed *.sql
var FS embed.FS

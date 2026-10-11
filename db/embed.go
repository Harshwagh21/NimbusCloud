// Package db embeds migration files so integration tests can apply the same SQL that
// `make migrate-up` applies, without depending on the migrate CLI being on PATH.
package db

import "embed"

// MigrationFiles holds every numbered .up.sql / .down.sql under migrations/.
//
//go:embed migrations/*.sql
var MigrationFiles embed.FS

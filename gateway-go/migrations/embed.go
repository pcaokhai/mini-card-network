// Package migrations embeds the goose SQL migrations for the acquirer database.
package migrations

import "embed"

// FS holds every *.sql migration file for goose to run against the acquirer database.
//
//go:embed *.sql
var FS embed.FS

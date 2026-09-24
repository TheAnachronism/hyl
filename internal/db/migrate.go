package db

import (
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func setupGoose() {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		panic(err)
	}
}

// Migrate applies every pending migration.
func Migrate(pool *sql.DB) error {
	setupGoose()
	return goose.Up(pool, "migrations")
}

// MigrateDown rolls every migration back. It exists for tests and for
// operators who want to reset a scratch database. (goose.Down only reverts the
// most recent migration, so this goes to version 0.)
func MigrateDown(pool *sql.DB) error {
	setupGoose()
	return goose.DownTo(pool, "migrations", 0)
}

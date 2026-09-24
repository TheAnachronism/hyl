package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/markbeep/hyl/internal/config"
)

// Open opens the SQLite pool with the pragmas hyl relies on: WAL journalling,
// a 5 s busy timeout, enforced foreign keys, NORMAL synchronous writes and
// immediate write transactions (so a writer waits instead of failing with
// SQLITE_BUSY). A ping runs immediately so a bad path or unsupported pragma
// fails at boot rather than at the first request.
func Open(cfg config.Config) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"+
			"&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_txlock=immediate",
		cfg.DBPath,
	)
	pool, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	pool.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("opening %s: %w", cfg.DBPath, err)
	}
	return pool, nil
}

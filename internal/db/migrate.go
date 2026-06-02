// Package db provides database connection and schema management utilities.
//
// Migration notes:
//   - golang-migrate uses lib/pq (not pgx/v5) internally, opening a short-lived
//     second connection alongside the pgx pool. This is intentional and harmless
//     as long as DATABASE_URL contains no pgx-specific query parameters
//     (e.g. pool_max_conns, pool_min_conns). Never append pgx DSN params to the URL.
//   - Concurrent migrations are serialised via PostgreSQL advisory locks, which
//     are released automatically if the process dies. However, non-transactional
//     DDL (e.g. CREATE INDEX CONCURRENTLY) can leave a dirty state on a crash.
//     Avoid non-transactional DDL in future migration files.
package db

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/matangi/eventpulse/migrations"
)

// Migrate applies all pending up migrations. It is safe to call on every
// startup — if no migrations are pending it returns immediately.
func Migrate(databaseURL string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			slog.Warn("migration source close", "err", srcErr)
		}
		if dbErr != nil {
			slog.Warn("migration db close", "err", dbErr)
		}
	}()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			slog.Info("database schema up to date")
			return nil
		}
		return fmt.Errorf("apply migrations: %w", err)
	}

	version, dirty, verErr := m.Version()
	if verErr != nil {
		slog.Info("migrations applied")
	} else {
		slog.Info("migrations applied", "version", version, "dirty", dirty)
	}
	return nil
}

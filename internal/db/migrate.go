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
	defer m.Close()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			slog.Info("database schema up to date")
			return nil
		}
		return fmt.Errorf("apply migrations: %w", err)
	}

	version, _, _ := m.Version()
	slog.Info("migrations applied", "version", version)
	return nil
}

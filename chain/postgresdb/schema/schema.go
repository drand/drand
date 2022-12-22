// Package schema helps to keep the database up to date with schema.
package schema

import (
	"context"
	_ "embed" // Calls init function.
	"fmt"

	"github.com/drand/drand/chain/postgresdb/database"

	"github.com/ardanlabs/darwin/v2"
	"github.com/jmoiron/sqlx"
)

var (
	//go:embed schema.sql
	schemaDoc string
)

// Migrate attempts to bring the schema for db up to date with the migrations
// defined in this package.
func Migrate(ctx context.Context, db *sqlx.DB) error {
	if err := database.StatusCheck(ctx, db); err != nil {
		return fmt.Errorf("status check database: %w", err)
	}

	driver, err := darwin.NewGenericDriver(db.DB, darwin.PostgresDialect{})
	if err != nil {
		return fmt.Errorf("construct darwin driver: %w", err)
	}

	d := darwin.New(driver, darwin.ParseMigrations(schemaDoc))
	return d.Migrate()
}

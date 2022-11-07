package pg

import (
	"context"
	_ "embed" // Calls init function.
	"fmt"

	"github.com/ardanlabs/darwin"
	"github.com/drand/drand/chain/pg/database"
	"github.com/jmoiron/sqlx"
)

var (
	//go:embed schema.sql
	schemaDoc string
)

// migrate attempts to bring the schema for db up to date with the migrations
// defined in this package.
func migrate(ctx context.Context, db *sqlx.DB) error {
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

package cfg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/drand/drand/internal/chain"
	"github.com/drand/drand/internal/chain/postgresdb/database"
	"github.com/drand/drand/internal/chain/postgresdb/schema"
	"github.com/drand/drand/internal/test"
)

// To be used when dbEngineType is postgres.
var c *test.Container

func BootContainer() func() {
	var err error
	c, err = test.StartPGDB()
	if err != nil {
		panic(err)
	}
	return func() {
		test.StopPGDB(c)
	}
}

const defaultPgDSN = "postgres://postgres:postgres@%s/%s?sslmode=disable&connect_timeout=5"

func ComputePgDSN(dbEngineType chain.StorageType) func() string {
	return func() string {
		dsn := defaultPgDSN
		dbName := test.ComputeDBName()

		if dbEngineType != chain.PostgreSQL {
			return ""
		}

		withTestDB(dbName)
		return fmt.Sprintf(dsn, c.Host, dbName)
	}
}

func withTestDB(dbName string) {
	newRegression(c, dbName)
}

// newRegression creates a test database inside a Docker container.
// Unlike test.NewUnit, this ignores all the cleanup features as the
// container will be removed at the end of the run.
// Since we are running in an unpredictable environment, proper cleanup
// is harder to implement than just letting the container delete itself.
func newRegression(c *test.Container, dbName string) {
	dbName = strings.ToLower(dbName)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dbM, err := database.Open(ctx, database.Config{
		User:       "postgres",
		Password:   "postgres",
		Host:       c.Host,
		Name:       "postgres",
		DisableTLS: true,
	})
	if err != nil {
		err := fmt.Errorf("opening database connection %v", err)
		panic(err)
	}

	fmt.Printf("creating database %s...\n", dbName)
	_, err = dbM.ExecContext(context.Background(), "CREATE DATABASE "+dbName)
	if err != nil {
		err := fmt.Errorf("creating database %s %v", dbName, err)
		panic(err)
	}

	err = dbM.Close()
	if err != nil {
		err := fmt.Errorf("closing database connection %v", err)
		panic(err)
	}

	// =========================================================================

	db, err := database.Open(ctx, database.Config{
		User:       "postgres",
		Password:   "postgres",
		Host:       c.Host,
		Name:       dbName,
		DisableTLS: true,
	})
	if err != nil {
		err := fmt.Errorf("opening database connection %v", err)
		panic(err)
	}

	fmt.Println("Perform migrations ...")

	if err := schema.Migrate(ctx, db); err != nil {
		fmt.Printf("Logs for %s\n%s:\n", c.ID, test.DumpContainerLogs(c.ID))
		err := fmt.Errorf("migrating error: %w", err)
		panic(err)
	}

	fmt.Println("Ready for testing ...")
}

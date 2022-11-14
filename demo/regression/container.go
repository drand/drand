package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/postgresdb/database"
	"github.com/drand/drand/chain/postgresdb/schema"
	"github.com/drand/drand/test"
)

// To be used when dbEngineType is postgres.
var c *test.Container

func bootContainer() func() {
	var err error
	c, err = test.StartDB()
	if err != nil {
		panic(err)
	}
	return func() {
		test.StopDB(c)
	}
}

const defaultPgDSN = "postgres://postgres:postgres@%s/%s?sslmode=disable&timeout=5&connect_timeout=5"

func computePgDSN() func() string {
	return func() string {
		dsn := defaultPgDSN
		dbName := computeDBName()

		if chain.StorageType(*dbEngineType) != chain.PostgresSQL {
			return ""
		}

		withTestDB(dbName)
		return fmt.Sprintf(dsn, c.Host, dbName)
	}
}

// computeDBName helps generate new, unique database names during the runtime of a test.
// By adding the time, with milliseconds, we can avoid this, e.g. testbroadcast_09223736225
func computeDBName() string {
	suffix := strings.Replace(time.Now().Format("02150405.000"), ".", "", -1)
	dbName := fmt.Sprintf("drand_regression_%s", suffix)
	return strings.ToLower(dbName)
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

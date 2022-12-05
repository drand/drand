package test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/drand/drand/chain/postgresdb/database"
	"github.com/drand/drand/chain/postgresdb/schema"
	"github.com/drand/drand/log"
)

// StartPGDB starts a database instance.
func StartPGDB() (*Container, error) {
	image := "postgres:15.1-alpine3.16"
	port := "5432"
	args := []string{"-e", "POSTGRES_PASSWORD=postgres"}

	c, err := StartContainer(image, port, args...)
	if err != nil {
		return nil, fmt.Errorf("starting container: %w", err)
	}

	fmt.Printf("Image:       %s\n", image)
	fmt.Printf("ContainerID: %s\n", c.ID)
	fmt.Printf("Host:        %s\n", c.Host)

	return c, nil
}

// StopPGDB stops a running database instance.
func StopPGDB(c *Container) {
	StopContainer(c.ID)
	fmt.Println("Stopped:", c.ID)
}

// NewUnit creates a test database inside a Docker container. It creates the
// required table structure but the database is otherwise empty. It returns
// the database to use as well as a function to call at the end of the test.
func NewUnit(t *testing.T, c *Container, dbName string) (log.Logger, *sqlx.DB) {
	t.Helper()

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
	require.NoError(t, err, "opening database connection")

	t.Logf("creating database %s...\n", dbName)

	_, err = dbM.ExecContext(context.Background(), "CREATE DATABASE "+dbName)
	require.NoError(t, err, "creating database %s", dbName)

	err = dbM.Close()
	require.NoError(t, err)

	// =========================================================================

	db, err := database.Open(ctx, database.Config{
		User:       "postgres",
		Password:   "postgres",
		Host:       c.Host,
		Name:       dbName,
		DisableTLS: true,
	})
	require.NoError(t, err, "opening database connection")

	t.Log("Perform migrations ...")

	if err := schema.Migrate(ctx, db); err != nil {
		t.Logf("Logs for %s\n%s:", c.ID, DumpContainerLogs(c.ID))
		t.Fatalf("Migrating error: %s", err)
	}

	t.Log("Ready for testing ...")

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	l := log.NewLogger(zapcore.AddSync(writer), log.LogDebug)

	// teardown is the function that should be invoked when the caller is done
	// with the database.
	t.Cleanup(func() {
		t.Helper()
		err := db.Close()
		require.NoError(t, err)

		_ = writer.Flush()
		fmt.Println("******************** LOGS ********************")
		fmt.Print(buf.String())
		fmt.Println("******************** LOGS ********************")
	})

	return l, db
}

// ComputeDBName helps generate new, unique database names during the runtime of a test.
// By adding the time, with milliseconds, we can avoid this, e.g. drand_test_09223736225
func ComputeDBName() string {
	suffix := strings.Replace(time.Now().Format("02150405.000"), ".", "", -1)
	dbName := fmt.Sprintf("drand_test_%s", suffix)
	return strings.ToLower(dbName)
}

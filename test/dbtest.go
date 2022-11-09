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

	"github.com/drand/drand/chain/pg/database"
	"github.com/drand/drand/log"
)

// Success and failure markers.
const (
	Success = "\u2713"
	Failed  = "\u2717"
)

// StartDB starts a database instance.
func StartDB() (*Container, error) {
	image := "postgres:15.0-alpine3.16"
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

// StopDB stops a running database instance.
func StopDB(c *Container) {
	StopContainer(c.ID)
	fmt.Println("Stopped:", c.ID)
}

// NewUnit creates a test database inside a Docker container. It creates the
// required table structure but the database is otherwise empty. It returns
// the database to use as well as a function to call at the end of the test.
func NewUnit(t *testing.T, c *Container, dbName string) (log.Logger, *sqlx.DB, func()) {
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

	t.Log("Database ready")

	//language=postgresql
	createQuery := `CREATE EXTENSION IF NOT EXISTS dblink;
DO $$
BEGIN
PERFORM dblink_exec('', 'CREATE DATABASE %s');
EXCEPTION WHEN duplicate_database THEN
    RAISE NOTICE '% exists, skipping', SQLERRM USING ERRCODE = SQLSTATE;
END
$$;`
	_, err = dbM.ExecContext(ctx, fmt.Sprintf(createQuery, dbName))
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

	t.Log("Ready for testing ...")

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	l := log.NewLogger(zapcore.AddSync(writer), log.LogDebug)

	// teardown is the function that should be invoked when the caller is done
	// with the database.
	teardown := func() {
		t.Helper()
		err := db.Close()
		require.NoError(t, err)

		_ = writer.Flush()
		fmt.Println("******************** LOGS ********************")
		fmt.Print(buf.String())
		fmt.Println("******************** LOGS ********************")
	}

	return l, db, teardown
}

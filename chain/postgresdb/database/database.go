// Package database provides support for access the database.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq" // Calls init function.

	"github.com/drand/drand/log"
)

// lib/pq errorCodeNames
// https://github.com/lib/pq/blob/master/error.go#L178
const (
	uniqueViolation = "23505"
	undefinedTable  = "42P01"
)

// Set of error variables for CRUD operations.
var (
	ErrDBNotFound        = sql.ErrNoRows
	ErrDBDuplicatedEntry = errors.New("duplicated entry")
	ErrUndefinedTable    = errors.New("undefined table")
)

// Config is the required properties to use the database.
type Config struct {
	User         string
	Password     string
	Host         string
	Name         string
	MaxIdleConns int
	MaxOpenConns int
	DisableTLS   bool
}

// ConfigFromDSN provides support for creating a config from a DSN.
func ConfigFromDSN(dsn string) (Config, error) {
	conf, err := url.Parse(dsn)
	if err != nil {
		return Config{}, err
	}
	query := conf.Query()

	password, _ := conf.User.Password()
	cfg := Config{
		User:       conf.User.Username(),
		Password:   password,
		Host:       conf.Host,
		Name:       conf.Path,
		DisableTLS: true,
	}

	if query.Has("sslmode") {
		sslMode := query.Get("sslmode")
		switch sslMode {
		//nolint:goconst // Having to add constants is overkill.
		case "disable":
			cfg.DisableTLS = true
		case "required":
			cfg.DisableTLS = false
		default:
			return Config{}, fmt.Errorf("unsupported ssl mode %q. Expected disable or required", sslMode)
		}
	}

	cfg.MaxIdleConns = 2
	if query.Has("max-idle") {
		max := query.Get("max-idle")
		m, err := strconv.Atoi(max)
		if err != nil {
			return Config{}, fmt.Errorf("expected number for max-idle, got err: %w", err)
		}
		if m >= 0 {
			cfg.MaxIdleConns = m
		}
	}

	cfg.MaxOpenConns = 0
	if query.Has("max-open") {
		open := query.Get("max-open")
		o, err := strconv.Atoi(open)
		if err != nil {
			return Config{}, fmt.Errorf("expected number for max-open, got err: %w", err)
		}
		if o >= 0 {
			cfg.MaxOpenConns = o
		}
	}

	return cfg, nil
}

// Open knows how to open a database connection based on the configuration.
// It also performs a health check to make sure the connection is healthy.
//
//nolint:gocritic // There is nothing wrong with using value semantics here.
func Open(ctx context.Context, cfg Config) (*sqlx.DB, error) {
	sslMode := "require"
	if cfg.DisableTLS {
		sslMode = "disable"
	}

	q := make(url.Values)
	q.Set("sslmode", sslMode)
	q.Set("timezone", "utc")

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     cfg.Host,
		Path:     strings.ToLower(cfg.Name),
		RawQuery: q.Encode(),
	}

	db, err := sqlx.Open("postgres", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetMaxOpenConns(cfg.MaxOpenConns)

	return db, StatusCheck(ctx, db)
}

// StatusCheck returns nil if it can successfully talk to the database. It
// returns a non-nil error otherwise.
func StatusCheck(ctx context.Context, db *sqlx.DB) error {
	var pingError error
	var attempts int

	//nolint:gomnd // We want to have a reasonable retry period
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()

check:
	for {
		select {
		case <-t.C:
			attempts++
			pingError = db.Ping()
			if pingError == nil {
				break check
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	const q = `SELECT true`
	var tmp bool
	return db.QueryRowContext(ctx, q).Scan(&tmp)
}

// WithinTran runs passed function and do commit/rollback at the end.
func WithinTran(l log.Logger, db *sqlx.DB, fn func(*sqlx.Tx) error) error {
	l.Infow("begin tran")
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("begin tran: %w", err)
	}

	defer func() {
		if err := tx.Rollback(); err != nil {
			if errors.Is(err, sql.ErrTxDone) {
				return
			}
			l.Errorw("unable to rollback tran", "ERROR", err)
		}
		l.Infow("rollback tran")
	}()

	if err := fn(tx); err != nil {
		//nolint:errorlint // Have no clue why this is a problem :(.
		if pqerr, ok := err.(*pq.Error); ok && pqerr.Code == uniqueViolation {
			return ErrDBDuplicatedEntry
		}
		return fmt.Errorf("exec tran: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tran: %w", err)
	}
	l.Infow("commit tran")

	return nil
}

// ExecContext is a helper function to execute a CUD operation with
// logging and tracing.
func ExecContext(ctx context.Context, l log.Logger, db sqlx.ExtContext, query string) error {
	return NamedExecContext(ctx, l, db, query, struct{}{})
}

// NamedExecContext is a helper function to execute a CUD operation with
// logging and tracing where field replacement is necessary.
func NamedExecContext(ctx context.Context, l log.Logger, db sqlx.ExtContext, query string, data any) error {
	q, err := queryString(query, data)
	if err != nil {
		return err
	}

	if _, ok := data.(struct{}); ok {
		//nolint:gomnd // Having to add constants is overkill.
		l.AddCallerSkip(2).Infow("database.NamedExecContext", "query", q)
	} else {
		l.AddCallerSkip(1).Infow("database.NamedExecContext", "query", q)
	}

	if _, err := sqlx.NamedExecContext(ctx, db, query, data); err != nil {
		//nolint:errorlint // Have no clue why this is a problem :(.
		if pqerr, ok := err.(*pq.Error); ok {
			switch pqerr.Code {
			case undefinedTable:
				return ErrUndefinedTable
			case uniqueViolation:
				return ErrDBDuplicatedEntry
			}
		}
		return err
	}

	return nil
}

// QuerySlice is a helper function for executing queries that return a
// collection of data to be unmarshalled into a slice.
func QuerySlice[T any](ctx context.Context, l log.Logger, db sqlx.ExtContext, query string, dest *[]T) error {
	return NamedQuerySlice(ctx, l, db, query, struct{}{}, dest)
}

// NamedQuerySlice is a helper function for executing queries that return a
// collection of data to be unmarshalled into a slice where field replacement is necessary.
func NamedQuerySlice[T any](ctx context.Context, l log.Logger, db sqlx.ExtContext, query string, data any, dest *[]T) error {
	q, err := queryString(query, data)
	if err != nil {
		return err
	}

	if _, ok := data.(struct{}); ok {
		//nolint:gomnd // Having to add constants is overkill.
		l.AddCallerSkip(2).Infow("database.QuerySlice", "query", q)
	} else {
		l.AddCallerSkip(1).Infow("database.QuerySlice", "query", q)
	}

	rows, err := sqlx.NamedQueryContext(ctx, db, query, data)
	if err != nil {
		//nolint:errorlint // We want this
		if pqerr, ok := err.(*pq.Error); ok && pqerr.Code == undefinedTable {
			return ErrUndefinedTable
		}
		return err
	}
	defer rows.Close()

	var slice []T
	for rows.Next() {
		v := new(T)
		if err := rows.StructScan(v); err != nil {
			return err
		}
		slice = append(slice, *v)
	}
	*dest = slice

	return nil
}

// QueryStruct is a helper function for executing queries that return a
// single value to be unmarshalled into a struct type where field replacement is necessary.
func QueryStruct(ctx context.Context, l log.Logger, db sqlx.ExtContext, query string, dest any) error {
	return NamedQueryStruct(ctx, l, db, query, struct{}{}, dest)
}

// NamedQueryStruct is a helper function for executing queries that return a
// single value to be unmarshalled into a struct type where field replacement is necessary.
func NamedQueryStruct(ctx context.Context, l log.Logger, db sqlx.ExtContext, query string, data, dest any) error {
	q, err := queryString(query, data)
	if err != nil {
		return err
	}

	if _, ok := data.(struct{}); ok {
		//nolint:gomnd // This doesn't need to be a constant
		l.AddCallerSkip(2).Infow("database.QueryStruct", "query", q)
	} else {
		l.AddCallerSkip(1).Infow("database.QueryStruct", "query", q)
	}

	rows, err := sqlx.NamedQueryContext(ctx, db, query, data)
	if err != nil {
		//nolint:errorlint // Have no clue why this is a problem :(.
		if pqerr, ok := err.(*pq.Error); ok && pqerr.Code == undefinedTable {
			return ErrUndefinedTable
		}
		return err
	}
	defer rows.Close()

	if !rows.Next() {
		return ErrDBNotFound
	}

	if err := rows.StructScan(dest); err != nil {
		return err
	}

	return nil
}

// queryString provides a pretty print version of the query and parameters.
func queryString(query string, args ...any) (string, error) {
	query, params, err := sqlx.Named(query, args)
	if err != nil {
		return "", err
	}

	for _, param := range params {
		var value string
		switch v := param.(type) {
		case string:
			value = fmt.Sprintf("%q", v)
		case []byte:
			value = fmt.Sprintf("%q", string(v))
		default:
			value = fmt.Sprintf("%v", v)
		}
		query = strings.Replace(query, "?", value, 1)
	}

	query = strings.ReplaceAll(query, "\t", "")
	query = strings.ReplaceAll(query, "\n", " ")

	return strings.Trim(query, " "), nil
}

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

	"github.com/drand/drand/common/tracer"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // Calls init function.

	"github.com/drand/drand/common/log"
)

// Config is the required properties to use the database.
type Config struct {
	User           string
	Password       string
	Host           string
	Name           string
	ConnectTimeout int
	MaxIdleConns   int
	MaxOpenConns   int
	DisableTLS     bool
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

	cfg.ConnectTimeout = 5
	if query.Has("connect_timeout") {
		timeout := query.Get("connect_timeout")
		t, err := strconv.Atoi(timeout)
		if err != nil {
			return Config{}, fmt.Errorf("expected number for connect_timeout, got err: %w", err)
		}

		cfg.ConnectTimeout = t
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
	ctx, span := tracer.NewSpan(ctx, "database.Open")
	defer span.End()

	sslMode := "require"
	if cfg.DisableTLS {
		sslMode = "disable"
	}

	q := make(url.Values)
	q.Set("sslmode", sslMode)
	q.Set("timezone", "utc")
	q.Set("connect_timeout", strconv.Itoa(cfg.ConnectTimeout))

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
	ctx, span := tracer.NewSpan(ctx, "database.StatusCheck")
	defer span.End()

	var pingError error

	//nolint:gomnd // We want to have a reasonable retry period
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()

check:
	for {
		select {
		case <-t.C:
			pingError = db.PingContext(ctx)
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
func WithinTran(ctx context.Context, l log.Logger, db *sqlx.DB, fn func(context.Context, *sqlx.Tx) error) error {
	ctx, span := tracer.NewSpan(ctx, "database.WithinTran")
	defer span.End()

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

	if err := fn(ctx, tx); err != nil {
		return fmt.Errorf("exec tran: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tran: %w", err)
	}
	l.Infow("commit tran")

	return nil
}

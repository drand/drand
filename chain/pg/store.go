// Package pg implements the required details to use PostgreSQL as a storage engine.
package pg

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jmoiron/sqlx"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

// ErrNoBeaconSaved is the error returned when no beacon have been saved in the
// database yet.
var ErrNoBeaconSaved = errors.New("beacon not found in database")

// Options holds all the options available for this storage engine
type Options struct{}

// beacon represents a beacon that is stored in the database.
type dbBeacon struct {
	PreviousSig []byte `db:"previous_sig"`
	Round       uint64 `db:"round"`
	Signature   []byte `db:"signature"`
}

// =============================================================================

type PGStore struct {
	log       log.Logger
	tableName string
	db        *sqlx.DB
}

// NewPGStore returns a Store implementation using the PostgreSQL storage engine.
// TODO implement options.
// TODO figure out if/how the DB connection can be initialized only once and shared between beacons.
func NewPGStore(log log.Logger, tableName string, opts *Options) (PGStore, error) {
	db, err := Open(Config{
		User:         "drand",
		Password:     "drand",
		Host:         "127.0.0.1:35432",
		Name:         "drand",
		MaxIdleConns: 5,
		MaxOpenConns: 10,
		DisableTLS:   true,
	})
	if err != nil {
		return PGStore{}, err
	}

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			round        BIGINT NOT NULL CONSTRAINT %[1]s_pk PRIMARY KEY,
			signature    BYTEA  NOT NULL,
			previous_sig BYTEA  NOT NULL
		)`,
		tableName)

	if err := ExecContext(context.Background(), log, db, query); err != nil {
		return PGStore{}, err
	}

	pg := PGStore{
		log:       log,
		tableName: tableName,
		db:        db,
	}
	return pg, nil
}

func (p PGStore) Len() (int, error) {
	query := fmt.Sprintf(`
		SELECT
			value AS count(*)
		FROM
			%s`,
		p.tableName)

	var ret struct {
		Count int `db:"count"`
	}
	if err := NamedQueryStruct(context.Background(), p.log, p.db, query, nil, &ret); err != nil {
		return 0, err
	}

	return ret.Count, nil
}

func (p PGStore) Put(beacon *chain.Beacon) error {
	data := dbBeacon{
		Round:       beacon.Round,
		Signature:   beacon.Signature,
		PreviousSig: beacon.PreviousSig,
	}

	query := fmt.Sprintf(`
		INSERT INTO %s
			(round, signature, previous_sig)
		VALUES
			(:round, :signature, :previous_sig) ON CONFLICT DO NOTHING`,
		p.tableName)

	if err := NamedExecContext(context.Background(), p.log, p.db, query, data); err != nil {
		return err
	}

	return nil
}

func (p PGStore) Last() (*chain.Beacon, error) {
	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
		ORDER BY
			round DESC LIMIT 1`,
		p.tableName)

	var dbBeacon dbBeacon
	if err := NamedQueryStruct(context.Background(), p.log, p.db, query, nil, &dbBeacon); err != nil {
		return nil, err
	}

	beacon := chain.Beacon(dbBeacon)
	return &beacon, nil
}

func (p PGStore) Get(round uint64) (*chain.Beacon, error) {
	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
			WHERE round=$1`,
		p.tableName)

	var dbBeacon dbBeacon
	if err := NamedQueryStruct(context.Background(), p.log, p.db, query, nil, &dbBeacon); err != nil {
		return nil, err
	}

	beacon := chain.Beacon(dbBeacon)
	return &beacon, nil
}

func (p PGStore) Cursor(fn func(chain.Cursor) error) error {
	return fn(&pgCursor{p.log, p.db, p.tableName, 0})
}

func (p PGStore) Close() error {
	// TODO nothing?
	return nil
}

func (p PGStore) Del(round uint64) error {
	// TODO implement me
	panic("implement me")
}

func (p PGStore) SaveTo(w io.Writer) error {
	// TODO implement me
	panic("implement me")
}

// =============================================================================

type pgCursor struct {
	log       log.Logger
	db        *sqlx.DB
	tableName string
	pos       int
}

func (p *pgCursor) First() (*chain.Beacon, error) {
	defer func() {
		p.pos++
	}()

	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
		ORDER BY
			round ASC LIMIT 1`,
		p.tableName)

	var dbBeacon dbBeacon
	if err := NamedQueryStruct(context.Background(), p.log, p.db, query, nil, &dbBeacon); err != nil {
		return nil, err
	}

	beacon := chain.Beacon(dbBeacon)
	return &beacon, nil
}

func (p *pgCursor) Next() (*chain.Beacon, error) {
	defer func() {
		p.pos++
	}()

	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
		ORDER BY
			round ASC OFFSET $1 LIMIT 1`,
		p.tableName)

	var dbBeacon dbBeacon
	if err := NamedQueryStruct(context.Background(), p.log, p.db, query, nil, &dbBeacon); err != nil {
		return nil, err
	}

	beacon := chain.Beacon(dbBeacon)
	return &beacon, nil
}

func (p *pgCursor) Seek(round uint64) (*chain.Beacon, error) {
	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
			WHERE round=$1`,
		p.tableName)

	var dbBeacon dbBeacon
	if err := NamedQueryStruct(context.Background(), p.log, p.db, query, nil, &dbBeacon); err != nil {
		return nil, err
	}

	beacon := chain.Beacon(dbBeacon)
	return &beacon, nil
}

func (p *pgCursor) Last() (*chain.Beacon, error) {
	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
		ORDER BY
			round DESC LIMIT 1`,
		p.tableName)

	var dbBeacon dbBeacon
	if err := NamedQueryStruct(context.Background(), p.log, p.db, query, nil, &dbBeacon); err != nil {
		return nil, err
	}

	beacon := chain.Beacon(dbBeacon)
	return &beacon, nil
}

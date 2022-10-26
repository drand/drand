// Package pg implements the required details to use PostgreSQL as a storage engine.
package pg

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jmoiron/sqlx"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/pg/database"
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
	log        log.Logger
	db         *sqlx.DB
	beaconName string
}

// NewPGStore returns a Store implementation using the PostgreSQL storage engine.
// TODO implement options.
// TODO figure out if/how the DB connection can be initialized only once and shared between beacons.
func NewPGStore(log log.Logger, db *sqlx.DB, beaconName string, opts *Options) (PGStore, error) {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			round        BIGINT NOT NULL CONSTRAINT s_pk PRIMARY KEY,
			signature    BYTEA  NOT NULL,
			previous_sig BYTEA  NOT NULL
		)`, beaconName)

	if err := database.ExecContext(context.Background(), log, db, query); err != nil {
		return PGStore{}, err
	}

	pg := PGStore{
		log:        log,
		db:         db,
		beaconName: beaconName,
	}
	return pg, nil
}

func (p PGStore) Len() (int, error) {
	query := fmt.Sprintf(`
		SELECT
			COUNT(*)
		FROM
			%s`,
		p.beaconName)

	var ret struct {
		Count int `db:"count"`
	}
	if err := database.QueryStruct(context.Background(), p.log, p.db, query, &ret); err != nil {
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
			(:round, :signature, :previous_sig)
		ON CONFLICT DO NOTHING`,
		p.beaconName)

	if err := database.NameExecContext(context.Background(), p.log, p.db, query, data); err != nil {
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
		p.beaconName)

	var dbBeacon []dbBeacon
	if err := database.QuerySlice(context.Background(), p.log, p.db, query, &dbBeacon); err != nil {
		return nil, err
	}

	if len(dbBeacon) == 0 {
		return nil, errors.New("beacon not found")
	}

	beacon := chain.Beacon(dbBeacon[0])
	return &beacon, nil
}

func (p PGStore) Get(round uint64) (*chain.Beacon, error) {
	data := struct {
		Round uint64 `db:"round"`
	}{
		Round: round,
	}

	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
		WHERE
			round = :round`,
		p.beaconName)

	var dbBeacon dbBeacon
	if err := database.NamedQueryStruct(context.Background(), p.log, p.db, query, data, &dbBeacon); err != nil {
		return nil, ErrNoBeaconSaved
	}

	beacon := chain.Beacon(dbBeacon)
	return &beacon, nil
}

func (p PGStore) Cursor(fn func(chain.Cursor) error) error {
	return fn(&pgCursor{p.log, p.db, p.beaconName, 0})
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
	log        log.Logger
	db         *sqlx.DB
	beaconName string
	pos        int
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
		p.beaconName)

	var dbBeacon []dbBeacon
	if err := database.QuerySlice(context.Background(), p.log, p.db, query, &dbBeacon); err != nil {
		return nil, err
	}

	if len(dbBeacon) == 0 {
		return nil, errors.New("beacon not found")
	}

	beacon := chain.Beacon(dbBeacon[0])
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
		p.beaconName)

	var dbBeacon []dbBeacon
	if err := database.QuerySlice(context.Background(), p.log, p.db, query, &dbBeacon); err != nil {
		return nil, err
	}

	if len(dbBeacon) == 0 {
		return nil, errors.New("beacon not found")
	}

	beacon := chain.Beacon(dbBeacon[0])
	return &beacon, nil
}

func (p *pgCursor) Seek(round uint64) (*chain.Beacon, error) {
	data := struct {
		Round uint64 `db:"round"`
	}{
		Round: round,
	}

	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
		WHERE
			round = :round`,
		p.beaconName)

	var dbBeacon dbBeacon
	if err := database.NamedQueryStruct(context.Background(), p.log, p.db, query, data, &dbBeacon); err != nil {
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
		p.beaconName)

	var dbBeacon []dbBeacon
	if err := database.QuerySlice(context.Background(), p.log, p.db, query, &dbBeacon); err != nil {
		return nil, err
	}

	if len(dbBeacon) == 0 {
		return nil, errors.New("beacon not found")
	}

	beacon := chain.Beacon(dbBeacon[0])
	return &beacon, nil
}

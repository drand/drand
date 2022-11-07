// Package pg implements the required details to use PostgreSQL as a storage engine.
package pg

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jmoiron/sqlx"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	chainerrors "github.com/drand/drand/chain/errors"
	"github.com/drand/drand/chain/pg/database"
	"github.com/drand/drand/log"
)

// beacon represents a beacon that is stored in the database.
type dbBeacon struct {
	PreviousSig []byte `db:"previous_sig"`
	Round       uint64 `db:"round"`
	Signature   []byte `db:"signature"`
}

// =============================================================================

// Store represents access to the postgres database for beacon management.
type Store struct {
	log        log.Logger
	db         *sqlx.DB
	beaconName string
}

// NewPGStore returns a new PG Store that provides the CRUD based API need for
// supporting drand serialization.
func NewPGStore(log log.Logger, db *sqlx.DB, beaconName string) (*Store, error) {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			round        BIGINT NOT NULL CONSTRAINT s_pk PRIMARY KEY,
			signature    BYTEA  NOT NULL,
			previous_sig BYTEA  NOT NULL
		)`, beaconName)

	if err := database.ExecContext(context.Background(), log, db, query); err != nil {
		return nil, err
	}

	p := Store{
		log:        log,
		db:         db,
		beaconName: beaconName,
	}
	return &p, nil
}

// Len returns the number of beacons in the configured beacon table.
func (p *Store) Len() (int, error) {
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

// Put adds the specified beacon to the configured beacon table.
func (p *Store) Put(beacon *chain.Beacon) error {
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

	if err := database.NamedExecContext(context.Background(), p.log, p.db, query, data); err != nil {
		return err
	}

	return nil
}

// Last returns the last beacon stored in the configured beacon table.
func (p *Store) Last() (*chain.Beacon, error) {
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

	b := chain.Beacon(dbBeacon[0])
	return &b, nil
}

// Get returns the specified beacon from the configured beacon table.
func (p *Store) Get(round uint64) (*chain.Beacon, error) {
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
		return nil, beacon.ErrNoBeaconStored
	}

	b := chain.Beacon(dbBeacon)
	return &b, nil
}

// Close does something and I am not sure just yet.
func (p *Store) Close() error {
	// We don't want to close the db with this!!
	return nil
}

// Del removes the specified round from the beacon table.
func (p *Store) Del(round uint64) error {
	data := struct {
		Round uint64 `db:"round"`
	}{
		Round: round,
	}

	query := fmt.Sprintf(`
		DELETE FROM %s
		WHERE
			round = :round`,
		p.beaconName)

	if err := database.NamedExecContext(context.Background(), p.log, p.db, query, data); err != nil {
		return err
	}

	return nil
}

// Cursor returns a cursor for iterating over the beacon table.
func (p *Store) Cursor(fn func(chain.Cursor) error) error {
	c := cursor{
		pgStore: p,
		pos:     0,
	}

	return fn(&c)
}

// SaveTo does something and I am not sure just yet.
func (p *Store) SaveTo(w io.Writer) error {
	panic("implement me")
}

// =============================================================================

// cursor implements support for iterating through the beacon table.
type cursor struct {
	pgStore *Store
	pos     int
}

// First returns the first beacon from the configured beacon table.
func (c *cursor) First() (*chain.Beacon, error) {
	defer func() {
		c.pos++
	}()

	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
		ORDER BY
			round ASC LIMIT 1`,
		c.pgStore.beaconName)

	var dbBeacon []dbBeacon
	if err := database.QuerySlice(context.Background(), c.pgStore.log, c.pgStore.db, query, &dbBeacon); err != nil {
		return nil, err
	}

	if len(dbBeacon) == 0 {
		return nil, chainerrors.ErrNoBeaconStored
	}

	b := chain.Beacon(dbBeacon[0])
	return &b, nil
}

// Next returns the next beacon from the configured beacon table.
func (c *cursor) Next() (*chain.Beacon, error) {
	defer func() {
		c.pos++
	}()

	data := struct {
		Offset int `db:"offset"`
	}{
		Offset: c.pos,
	}

	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
		ORDER BY
			round ASC OFFSET :offset LIMIT 1`,
		c.pgStore.beaconName)

	var dbBeacon []dbBeacon
	if err := database.NamedQuerySlice(context.Background(), c.pgStore.log, c.pgStore.db, query, data, &dbBeacon); err != nil {
		return nil, err
	}

	if len(dbBeacon) == 0 {
		return nil, chainerrors.ErrNoBeaconStored
	}

	b := chain.Beacon(dbBeacon[0])
	return &b, nil
}

// Seek searches the beacon table for the specified round
func (c *cursor) Seek(round uint64) (*chain.Beacon, error) {
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
		c.pgStore.beaconName)

	var dbBeacon dbBeacon
	err := database.NamedQueryStruct(context.Background(), c.pgStore.log, c.pgStore.db, query, data, &dbBeacon)
	if err != nil {
		if errors.Is(err, database.ErrDBNotFound) {
			return nil, chainerrors.ErrNoBeaconStored
		}
		return nil, err
	}

	b := chain.Beacon(dbBeacon)
	return &b, nil
}

// Last returns the last beacon from the configured beacon table.
func (c *cursor) Last() (*chain.Beacon, error) {
	query := fmt.Sprintf(`
		SELECT
			round, signature, previous_sig
		FROM %s
		ORDER BY
			round DESC LIMIT 1`,
		c.pgStore.beaconName)

	var dbBeacon []dbBeacon
	if err := database.QuerySlice(context.Background(), c.pgStore.log, c.pgStore.db, query, &dbBeacon); err != nil {
		return nil, err
	}

	if len(dbBeacon) == 0 {
		return nil, chainerrors.ErrNoBeaconStored
	}

	b := chain.Beacon(dbBeacon[0])
	return &b, nil
}

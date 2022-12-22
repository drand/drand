package pgdb

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/jmoiron/sqlx"

	"github.com/drand/drand/chain"
	chainerrors "github.com/drand/drand/chain/errors"
	"github.com/drand/drand/log"
)

// Store represents access to the postgres database for beacon management.
type Store struct {
	log      log.Logger
	db       *sqlx.DB
	beaconID int
}

// NewStore returns a new store that provides the CRUD based API needed for
// supporting drand serialization.
func NewStore(ctx context.Context, l log.Logger, db *sqlx.DB, beaconName string) (*Store, error) {
	p := Store{
		log: l,
		db:  db,
	}

	id, err := p.AddBeaconID(ctx, beaconName)
	if err != nil {
		return nil, err
	}

	p.beaconID = id

	return &p, nil
}

// Close is an noop.
func (p *Store) Close(context.Context) error {
	return nil
}

// AddBeaconID adds the beacon to the database if it does not exist.
func (p *Store) AddBeaconID(ctx context.Context, beaconName string) (int, error) {
	const create = `
	INSERT INTO beacons
		(name)
	VALUES
		(:name)
	ON CONFLICT DO NOTHING`

	data := struct {
		Name string `db:"name"`
	}{
		Name: beaconName,
	}

	if _, err := p.db.NamedExecContext(ctx, create, data); err != nil {
		return 0, err
	}

	const query = `
	SELECT
		id
	FROM
		beacons
	WHERE
		name = :name
	LIMIT 1`

	var ret int

	rows, err := p.db.NamedQueryContext(ctx, query, data)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, sql.ErrNoRows
	}
	err = rows.Scan(&ret)
	return ret, err
}

// Len returns the number of beacons in the configured beacon table.
func (p *Store) Len(ctx context.Context) (int, error) {
	const query = `
	SELECT
		COUNT(*)
	FROM
		beacon_details
	WHERE
		beacon_id = :beacon_id`

	data := struct {
		BeaconID int `db:"beacon_id"`
	}{
		BeaconID: p.beaconID,
	}

	var ret struct {
		Count int `db:"count"`
	}
	rows, err := p.db.NamedQueryContext(ctx, query, data)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, chainerrors.ErrNoBeaconStored
	}

	err = rows.StructScan(&ret)
	return ret.Count, err
}

// Put adds the specified beacon to the database.
func (p *Store) Put(ctx context.Context, b *chain.Beacon) error {
	const query = `
	INSERT INTO beacon_details
		(beacon_id, round, signature, previous_sig)
	VALUES
		(:beacon_id, :round, :signature, :previous_sig)
	ON CONFLICT DO NOTHING`

	data := struct {
		BeaconID int `db:"beacon_id"`
		dbBeacon
	}{
		BeaconID: p.beaconID,
		dbBeacon: dbBeacon{
			Round:       b.Round,
			Signature:   b.Signature,
			PreviousSig: b.PreviousSig,
		},
	}

	_, err := p.db.NamedExecContext(ctx, query, data)
	return err
}

// Last returns the last beacon stored in the configured beacon table.
func (p *Store) Last(ctx context.Context) (*chain.Beacon, error) {
	const query = `
	SELECT
		round, signature, previous_sig
	FROM
		beacon_details
	WHERE
		beacon_id = :id
	ORDER BY
		round DESC
	LIMIT 1`

	data := struct {
		ID int `db:"id"`
	}{
		ID: p.beaconID,
	}

	return p.getBeacon(ctx, query, data)
}

// Get returns the specified beacon from the configured beacon table.
func (p *Store) Get(ctx context.Context, round uint64) (*chain.Beacon, error) {
	const query = `
	SELECT
		round, signature, previous_sig 
	FROM
		beacon_details
	WHERE
		beacon_id = :id AND
		round = :round
	LIMIT 1`

	data := struct {
		ID    int    `db:"id"`
		Round uint64 `db:"round"`
	}{
		ID:    p.beaconID,
		Round: round,
	}

	return p.getBeacon(ctx, query, data)
}

// Del removes the specified round from the beacon table.
func (p *Store) Del(ctx context.Context, round uint64) error {
	const query = `
	DELETE FROM
		beacon_details
	WHERE
		beacon_id = :id AND
		round = :round`

	data := struct {
		ID    int    `db:"id"`
		Round uint64 `db:"round"`
	}{
		ID:    p.beaconID,
		Round: round,
	}

	_, err := p.db.NamedExecContext(ctx, query, data)
	return err
}

// Cursor returns a cursor for iterating over the beacon table.
func (p *Store) Cursor(ctx context.Context, fn func(context.Context, chain.Cursor) error) error {
	c := cursor{
		store: p,
		pos:   0,
	}

	return fn(ctx, &c)
}

// SaveTo does something and I am not sure just yet.
func (p *Store) SaveTo(context.Context, io.Writer) error {
	return fmt.Errorf("saveTo not implemented for Postgres Store")
}

// =============================================================================

// cursor implements support for iterating through the beacon table.
type cursor struct {
	store *Store
	pos   uint64
}

// First returns the first beacon from the configured beacon table.
func (c *cursor) First(ctx context.Context) (*chain.Beacon, error) {
	defer func() {
		c.pos = 0
	}()

	const query = `
	SELECT
		round, signature, previous_sig
	FROM
		beacon_details
	WHERE
		beacon_id = :id
	ORDER BY
		round ASC LIMIT 1`

	data := struct {
		ID int `db:"id"`
	}{
		ID: c.store.beaconID,
	}

	return c.store.getBeacon(ctx, query, data)
}

// Next returns the next beacon from the configured beacon table.
func (c *cursor) Next(ctx context.Context) (*chain.Beacon, error) {
	defer func() {
		c.pos++
	}()

	const query = `
	SELECT
		round, signature, previous_sig
	FROM
		beacon_details
	WHERE
		beacon_id = :id
	ORDER BY
		round ASC OFFSET :offset
	LIMIT 1`

	data := struct {
		ID     int    `db:"id"`
		Offset uint64 `db:"offset"`
	}{
		ID:     c.store.beaconID,
		Offset: c.pos + 1,
	}

	return c.store.getBeacon(ctx, query, data)
}

// Seek searches the beacon table for the specified round
func (c *cursor) Seek(ctx context.Context, round uint64) (*chain.Beacon, error) {
	const query = `
	SELECT
		round, signature, previous_sig
	FROM
		beacon_details
	WHERE
		beacon_id = :id AND
		round = :round
	LIMIT 1`

	data := struct {
		ID    int    `db:"id"`
		Round uint64 `db:"round"`
	}{
		ID:    c.store.beaconID,
		Round: round,
	}

	ret, err := c.store.getBeacon(ctx, query, data)
	if err != nil {
		return nil, err
	}

	err = c.seekPosition(ctx, ret.Round)
	return ret, err
}

// Last returns the last beacon from the configured beacon table.
func (c *cursor) Last(ctx context.Context) (*chain.Beacon, error) {
	const query = `
	SELECT
		round, signature, previous_sig
	FROM
		beacon_details
	WHERE
		beacon_id = :id
	ORDER BY
		round DESC
	LIMIT 1`

	data := struct {
		ID int `db:"id"`
	}{
		ID: c.store.beaconID,
	}

	ret, err := c.store.getBeacon(ctx, query, data)
	if err != nil {
		return nil, err
	}

	err = c.seekPosition(ctx, ret.Round)
	return ret, err
}

// seekPosition updates the cursor position in the database for the next operation to work
func (c *cursor) seekPosition(ctx context.Context, round uint64) error {
	const query = `
	SELECT
		count(beacon_id) as round_offset
	FROM
	    beacon_details
	WHERE
	    beacon_id = :id
		AND round < :round`

	data := struct {
		ID    int    `db:"id"`
		Round uint64 `db:"round"`
	}{
		ID:    c.store.beaconID,
		Round: round,
	}

	var p struct {
		Position uint64 `db:"round_offset"`
	}
	rows, err := c.store.db.NamedQueryContext(ctx, query, data)
	if err != nil {
		return err
	}
	defer rows.Close()

	if !rows.Next() {
		return chainerrors.ErrNoBeaconStored
	}
	err = rows.StructScan(&p)

	c.pos = p.Position
	return err
}

func (p *Store) getBeacon(ctx context.Context, query string, data interface{}) (*chain.Beacon, error) {
	var ret dbBeacon
	rows, err := p.db.NamedQueryContext(ctx, query, data)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, chainerrors.ErrNoBeaconStored
	}

	if err := rows.StructScan(&ret); err != nil {
		return nil, err
	}

	return toChainBeacon(ret), nil
}

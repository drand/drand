package pgdb

import (
	"context"
	"fmt"
	"io"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	chainerrors "github.com/drand/drand/chain/errors"
	"github.com/drand/drand/chain/postgresdb/database"
	"github.com/drand/drand/log"

	"github.com/jmoiron/sqlx"
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

	id, err := p.AddBeacon(ctx, beaconName)
	if err != nil {
		return nil, err
	}

	p.beaconID = id

	return &p, nil
}

// Close is an noop.
func (p *Store) Close() error {
	return nil
}

// AddBeacon adds the beacon to the database if it does not exist.
func (p *Store) AddBeacon(ctx context.Context, beaconName string) (int, error) {
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
	if err := database.NamedExecContext(context.Background(), p.log, p.db, create, data); err != nil {
		return 0, err
	}

	const query = `
	SELECT
		id
	FROM
		beacons
	WHERE
		name = :name`

	var ret struct {
		ID int `db:"id"`
	}
	if err := database.NamedQueryStruct(ctx, p.log, p.db, query, data, &ret); err != nil {
		return 0, err
	}

	return ret.ID, nil
}

// Len returns the number of beacons in the configured beacon table.
func (p *Store) Len() (int, error) {
	const query = `
	SELECT
		COUNT(*)
	FROM
		beacon_details
	WHERE
		beacon_id = :id`

	data := struct {
		ID int `db:"id"`
	}{
		ID: p.beaconID,
	}

	var ret struct {
		Count int `db:"count"`
	}
	if err := database.NamedQueryStruct(context.Background(), p.log, p.db, query, data, &ret); err != nil {
		return 0, err
	}

	return ret.Count, nil
}

// Put adds the specified beacon to the database.
func (p *Store) Put(b *chain.Beacon) error {
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

	if err := database.NamedExecContext(context.Background(), p.log, p.db, query, data); err != nil {
		return err
	}

	return nil
}

// Last returns the last beacon stored in the configured beacon table.
func (p *Store) Last() (*chain.Beacon, error) {
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

	var dbBeacons []dbBeacon
	err := database.NamedQuerySlice(context.Background(), p.log, p.db, query, data, &dbBeacons)
	if err != nil {
		return nil, err
	}

	if len(dbBeacons) == 0 {
		return nil, chainerrors.ErrNoBeaconSaved
	}

	return toChainBeacon(dbBeacons[0]), nil
}

// Get returns the specified beacon from the configured beacon table.
func (p *Store) Get(round uint64) (*chain.Beacon, error) {
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

	var dbBeacon dbBeacon
	if err := database.NamedQueryStruct(context.Background(), p.log, p.db, query, data, &dbBeacon); err != nil {
		return nil, beacon.ErrNoBeaconStored
	}

	return toChainBeacon(dbBeacon), nil
}

// Del removes the specified round from the beacon table.
func (p *Store) Del(round uint64) error {
	const query = `
	DELETE
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

	if err := database.NamedExecContext(context.Background(), p.log, p.db, query, data); err != nil {
		return err
	}

	return nil
}

// Cursor returns a cursor for iterating over the beacon table.
func (p *Store) Cursor(fn func(chain.Cursor) error) error {
	c := cursor{
		store: p,
		pos:   0,
	}

	return fn(&c)
}

// SaveTo does something and I am not sure just yet.
func (p *Store) SaveTo(io.Writer) error {
	return fmt.Errorf("saveTo not implemented for Postgres Store")
}

// =============================================================================

// cursor implements support for iterating through the beacon table.
type cursor struct {
	store *Store
	pos   uint64
}

// First returns the first beacon from the configured beacon table.
func (c *cursor) First() (*chain.Beacon, error) {
	defer func() {
		c.pos = 0
	}()

	const query = `
	SELECT
		round, signature, previous_sig
	FROM
		(SELECT
			round, signature, previous_sig
		FROM
			beacon_details
		WHERE
			beacon_id = :id
		ORDER BY
			round ASC LIMIT 1) AS T
	WHERE
		round IS NOT NULL`

	data := struct {
		ID int `db:"id"`
	}{
		ID: c.store.beaconID,
	}

	var dbBeacons []dbBeacon
	if err := database.NamedQuerySlice(context.Background(), c.store.log, c.store.db, query, data, &dbBeacons); err != nil {
		return nil, err
	}

	if len(dbBeacons) == 0 {
		return nil, chainerrors.ErrNoBeaconStored
	}

	return toChainBeacon(dbBeacons[0]), nil
}

// Next returns the next beacon from the configured beacon table.
func (c *cursor) Next() (*chain.Beacon, error) {
	defer func() {
		c.pos++
	}()

	const query = `
	SELECT
		round, signature, previous_sig
	FROM
		(SELECT
			round, signature, previous_sig
		FROM
			beacon_details
		WHERE
			beacon_id = :id
		ORDER BY
			round ASC OFFSET :offset
		LIMIT 1) AS T
	WHERE
		round IS NOT NULL`

	data := struct {
		ID     int    `db:"id"`
		Offset uint64 `db:"offset"`
	}{
		ID:     c.store.beaconID,
		Offset: c.pos + 1,
	}

	var dbBeacons []dbBeacon
	if err := database.NamedQuerySlice(context.Background(), c.store.log, c.store.db, query, data, &dbBeacons); err != nil {
		return nil, err
	}

	if len(dbBeacons) == 0 {
		return nil, chainerrors.ErrNoBeaconStored
	}

	return toChainBeacon(dbBeacons[0]), nil
}

// Seek searches the beacon table for the specified round
func (c *cursor) Seek(round uint64) (*chain.Beacon, error) {
	const query = `
	SELECT
		round, signature, previous_sig
	FROM
		(SELECT
			round, signature, previous_sig
		FROM
			beacon_details
		WHERE
			beacon_id = :id AND
			round = :round
		LIMIT 1) AS T
	WHERE
		round IS NOT NULL`

	data := struct {
		ID    int    `db:"id"`
		Round uint64 `db:"round"`
	}{
		ID:    c.store.beaconID,
		Round: round,
	}

	var dbBeacon dbBeacon
	if err := database.NamedQueryStruct(context.Background(), c.store.log, c.store.db, query, data, &dbBeacon); err != nil {
		return nil, chainerrors.ErrNoBeaconStored
	}

	if err := c.seekPosition(round); err != nil {
		return nil, err
	}

	return toChainBeacon(dbBeacon), nil
}

// Last returns the last beacon from the configured beacon table.
func (c *cursor) Last() (*chain.Beacon, error) {
	const query = `
	SELECT
		round, signature, previous_sig
	FROM
		(SELECT
			round, signature, previous_sig
		FROM
			beacon_details
		WHERE
			beacon_id = :id
		ORDER BY
			round DESC
		LIMIT 1) AS T
	WHERE
		round IS NOT NULL`

	data := struct {
		ID int `db:"id"`
	}{
		ID: c.store.beaconID,
	}

	var dbBeacons []dbBeacon
	if err := database.NamedQuerySlice(context.Background(), c.store.log, c.store.db, query, data, &dbBeacons); err != nil {
		return nil, err
	}

	if len(dbBeacons) == 0 {
		return nil, chainerrors.ErrNoBeaconStored
	}

	if err := c.seekPosition(dbBeacons[0].Round); err != nil {
		return nil, err
	}

	return toChainBeacon(dbBeacons[0]), nil
}

// seekPosition updates the cursor position in the database for the next operation to work
func (c *cursor) seekPosition(round uint64) error {
	const query = `
	SELECT
		round_offset
	FROM
		(SELECT
			round_offset
		FROM
			(SELECT
				round, row_number() OVER(ORDER BY round ASC) AS round_offset
			FROM
				beacon_details
			WHERE
				beacon_id = :id
			ORDER BY
				round ASC) AS result
		WHERE
			round = :round) AS T2
	WHERE
		round_offset IS NOT NULL`

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
	if err := database.NamedQueryStruct(context.Background(), c.store.log, c.store.db, query, data, &p); err != nil {
		return err
	}

	c.pos = p.Position

	return nil
}

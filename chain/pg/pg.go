// Package pg implements the required details to use PostgreSQL as a storage engine.
package pg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/beacon"
	chainerrors "github.com/drand/drand/chain/errors"
	"github.com/drand/drand/chain/pg/database"
	"github.com/drand/drand/log"
)

// Store represents access to the postgres database for beacon management.
type Store struct {
	log        log.Logger
	db         *sqlx.DB
	beaconName string
}

// NewPGStore returns a new PG Store that provides the CRUD based API need for
// supporting drand serialization.
func NewPGStore(ctx context.Context, l log.Logger, db *sqlx.DB, beaconName string) (*Store, error) {
	// This needs to be used in identifiers, so let's make it consistent everywhere.
	// Prefix "drand_" is added to avoid names such as "default" colliding with reserved identifiers.
	beaconName = "drand_" + strings.ToLower(beaconName)

	if err := migrate(ctx, db); err != nil {
		return nil, err
	}

	const query = `SELECT drand_maketable(:tableName)`

	data := struct {
		TableName string `db:"tableName"`
	}{
		TableName: beaconName,
	}

	err := database.NamedExecContext(ctx, l, db, query, data)
	if err != nil {
		return nil, err
	}

	p := Store{
		log:        l,
		db:         db,
		beaconName: strings.ToLower(beaconName),
	}
	return &p, nil
}

// Len returns the number of beacons in the configured beacon table.
func (p *Store) Len() (int, error) {
	const query = `SELECT drand_tablesize(:tableName) AS table_size`

	data := struct {
		TableName string `db:"tableName"`
	}{
		TableName: p.beaconName,
	}

	var ret struct {
		Count int `db:"table_size"`
	}
	if err := database.NamedQueryStruct(context.Background(), p.log, p.db, query, data, &ret); err != nil {
		if errors.Is(err, database.ErrUndefinedTable) {
			return 0, nil
		}
		return 0, err
	}

	return ret.Count, nil
}

// Put adds the specified beacon to the configured beacon table.
func (p *Store) Put(b *chain.Beacon) error {
	const query = `SELECT drand_insertround(:tableName, :round, :signature, :previous_sig)`

	data := struct {
		TableName   string `db:"tableName"`
		PreviousSig []byte `db:"previous_sig"`
		Round       uint64 `db:"round"`
		Signature   []byte `db:"signature"`
	}{
		TableName:   p.beaconName,
		Round:       b.Round,
		Signature:   b.Signature,
		PreviousSig: b.PreviousSig,
	}

	if err := database.NamedExecContext(context.Background(), p.log, p.db, query, data); err != nil {
		return err
	}

	return nil
}

// Last returns the last beacon stored in the configured beacon table.
func (p *Store) Last() (*chain.Beacon, error) {
	const query = `SELECT round, signature, previous_sig FROM drand_getlastround(:tableName) WHERE round IS NOT NULL`

	data := struct {
		TableName string `db:"tableName"`
	}{
		TableName: p.beaconName,
	}

	var dbBeacons []dbBeacon
	err := database.NamedQuerySlice(context.Background(), p.log, p.db, query, data, &dbBeacons)
	if err != nil {
		return nil, err
	}

	if len(dbBeacons) == 0 {
		return nil, chainerrors.ErrNoBeaconSaved
	}

	b := chain.Beacon(dbBeacons[0])
	return &b, nil
}

// Get returns the specified beacon from the configured beacon table.
func (p *Store) Get(round uint64) (*chain.Beacon, error) {
	const query = `SELECT round, signature, previous_sig FROM drand_getround(:tableName, :round) WHERE round IS NOT NULL`

	data := struct {
		TableName string `db:"tableName"`
		Round     uint64 `db:"round"`
	}{
		TableName: p.beaconName,
		Round:     round,
	}

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
	const query = `SELECT DRAND_DeleteRound(:tableName, :round)`

	data := struct {
		TableName string `db:"tableName"`
		Round     uint64 `db:"round"`
	}{
		TableName: p.beaconName,
		Round:     round,
	}

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
func (p *Store) SaveTo(io.Writer) error {
	return fmt.Errorf("saveTo not implemented for Postgres Store")
}

// =============================================================================

// cursor implements support for iterating through the beacon table.
type cursor struct {
	pgStore *Store
	pos     uint64
}

// First returns the first beacon from the configured beacon table.
func (c *cursor) First() (*chain.Beacon, error) {
	const query = "SELECT round, signature, previous_sig FROM drand_getfirstround(:tableName) WHERE round IS NOT NULL"

	defer func() {
		c.pos = 0
	}()

	pgStore := c.pgStore

	data := struct {
		TableName string `db:"tableName"`
	}{
		TableName: pgStore.beaconName,
	}

	var dbBeacon []dbBeacon
	if err := database.NamedQuerySlice(context.Background(), pgStore.log, pgStore.db, query, data, &dbBeacon); err != nil {
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
	const query = "SELECT round, signature, previous_sig FROM drand_getoffsetround(:tableName, :offset) WHERE round IS NOT NULL"

	defer func() {
		c.pos++
	}()

	pgStore := c.pgStore

	data := struct {
		TableName string `db:"tableName"`
		Offset    uint64 `db:"offset"`
	}{
		TableName: pgStore.beaconName,
		Offset:    c.pos + 1,
	}

	var dbBeacon []dbBeacon
	if err := database.NamedQuerySlice(context.Background(), pgStore.log, pgStore.db, query, data, &dbBeacon); err != nil {
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
	const query = "SELECT round, signature, previous_sig FROM drand_getround(:tableName, :round) WHERE round IS NOT NULL"

	pgStore := c.pgStore

	data := struct {
		TableName string `db:"tableName"`
		Round     uint64 `db:"round"`
	}{
		TableName: pgStore.beaconName,
		Round:     round,
	}

	var dbBeacon dbBeacon
	err := database.NamedQueryStruct(context.Background(), pgStore.log, pgStore.db, query, data, &dbBeacon)
	if err != nil {
		if errors.Is(err, database.ErrDBNotFound) {
			return nil, chainerrors.ErrNoBeaconStored
		}
		return nil, err
	}

	err = c.seekPosition(round)
	if err != nil {
		return nil, err
	}

	b := chain.Beacon(dbBeacon)
	return &b, nil
}

// Last returns the last beacon from the configured beacon table.
func (c *cursor) Last() (*chain.Beacon, error) {
	const query = "SELECT round, signature, previous_sig FROM drand_getlastround(:tableName) WHERE round IS NOT NULL"

	pgStore := c.pgStore

	data := struct {
		TableName string `db:"tableName"`
	}{
		TableName: pgStore.beaconName,
	}

	var dbBeacon []dbBeacon
	err := database.NamedQuerySlice(context.Background(), pgStore.log, pgStore.db, query, data, &dbBeacon)
	if err != nil {
		return nil, err
	}

	if len(dbBeacon) == 0 {
		return nil, chainerrors.ErrNoBeaconStored
	}

	b := chain.Beacon(dbBeacon[0])
	err = c.seekPosition(b.Round)
	if err != nil {
		return nil, err
	}

	return &b, nil
}

// seekPosition updates the cursor position in the database for the next operation to work
func (c *cursor) seekPosition(round uint64) error {
	const query = "SELECT round_offset FROM drand_getroundposition(:tableName, :round) WHERE round_offset IS NOT NULL"

	p := struct {
		Position uint64 `db:"round_offset"`
	}{}

	pgStore := c.pgStore

	data := struct {
		TableName string `db:"tableName"`
		Round     uint64 `db:"round"`
	}{
		TableName: pgStore.beaconName,
		Round:     round,
	}

	err := database.NamedQueryStruct(context.Background(), pgStore.log, pgStore.db, query, data, &p)
	if err != nil {
		return err
	}

	c.pos = p.Position
	return nil
}

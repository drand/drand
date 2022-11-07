// Package pg implements the required details to use PostgreSQL as a storage engine.
package pg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

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

var bootstrapFuncs sync.Once

// Load all the required functions in the database, or ignore their creation.
//
//nolint:funclen // working as intended
func doBootstrapFuncs(ctx context.Context, db *sqlx.DB, isTest bool) (err error) {
	queries := []string{
		//language=postgresql
		`DO
$$
    BEGIN
        IF NOT EXISTS (SELECT *
                       FROM pg_type typ
                                INNER JOIN pg_namespace nsp
                                           ON nsp.oid = typ.typnamespace
                       WHERE nsp.nspname = current_schema()
                         AND typ.typname = 'drand_round') THEN
            CREATE  TYPE drand_round AS (round bigint, signature bytea, previous_sig bytea);
        END IF;
    END;
$$
LANGUAGE plpgsql;`,
		//language=postgresql
		`DO
$$
    BEGIN
        IF NOT EXISTS (SELECT *
                       FROM pg_type typ
                                INNER JOIN pg_namespace nsp
                                           ON nsp.oid = typ.typnamespace
                       WHERE nsp.nspname = current_schema()
                         AND typ.typname = 'drand_round_offset') THEN
            CREATE  TYPE drand_round_offset AS (round_offset bigint);
        END IF;
    END;
$$
LANGUAGE plpgsql;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_maketable(tableName char)
    RETURNS VOID
	LANGUAGE plpgsql
AS
$$
    BEGIN
		EXECUTE format('CREATE TABLE IF NOT EXISTS %I (
			round        BIGINT NOT NULL CONSTRAINT %1$I_pk PRIMARY KEY,
			signature    BYTEA  NOT NULL,
			previous_sig BYTEA  NOT NULL
		)', tableName);
	END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_tablesize(tableName char)
	RETURNS bigint
	LANGUAGE plpgsql
AS
$$
    DECLARE ret bigint;
    BEGIN
        EXECUTE (format('SELECT COUNT(*) FROM %I', tableName)) INTO ret;
        RETURN ret;
	END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_insertround(tableName char, round bigint, signature bytea, previous_sig bytea)
	RETURNS VOID
	LANGUAGE plpgsql
AS
$$
    BEGIN
        EXECUTE format('INSERT INTO %I
			(round, signature, previous_sig)
		VALUES
			(%L, %L, %L)
		ON CONFLICT DO NOTHING', tableName, round, signature, previous_sig);
	END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getlastround(tableName char)
    RETURNS drand_round
    LANGUAGE plpgsql
AS
$$
DECLARE
    ret drand_round;
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
                     ORDER BY round DESC
                     LIMIT 1',
                     tableName)) INTO ret;
    RETURN ret;
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getround(tableName char, round bigint)
    RETURNS drand_round
    LANGUAGE plpgsql
AS
$$
DECLARE
    ret drand_round;
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
                     WHERE round=%L
                     LIMIT 1',
                     tableName, round)) INTO ret;
    RETURN ret;
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_deleteround(tableName char, round bigint)
    RETURNS VOID
    LANGUAGE plpgsql
AS
$$
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
                     WHERE round=%L
                     LIMIT 1',
                     tableName, round));
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getroundposition(tableName char, round_num int)
    RETURNS drand_round_offset
    LANGUAGE plpgsql
AS
$$
DECLARE ret drand_round_offset;
BEGIN
    EXECUTE (format('SELECT round_offset FROM (
   	        SELECT round, row_number() OVER(ORDER BY round ASC) AS round_offset
		    FROM %I
            ORDER BY round ASC
        ) result WHERE round=%L',
        tableName, round_num)) INTO ret;
    RETURN ret;
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getfirstround(tableName char)
    RETURNS drand_round
    LANGUAGE plpgsql
AS
$$
DECLARE
    ret drand_round;
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
                     ORDER BY round ASC
                     LIMIT 1',
                     tableName)) INTO ret;
    RETURN ret;
END;
$$;`,
		//language=postgresql
		`CREATE OR REPLACE FUNCTION drand_getoffsetround(tableName char, r_offset bigint)
    RETURNS drand_round
    LANGUAGE plpgsql
AS
$$
DECLARE
    ret drand_round;
BEGIN
    EXECUTE (format('SELECT round, signature, previous_sig FROM %I
					ORDER BY round ASC OFFSET %L LIMIT 1',
                     tableName, r_offset)) INTO ret;
    RETURN ret;

END;
$$;`,
	}

	runQueries := func() {
		for _, query := range queries {
			_, err = db.DB.ExecContext(ctx, query)
			if err != nil {
				return
			}
		}
	}

	if isTest {
		runQueries()
	} else {
		bootstrapFuncs.Do(runQueries)
	}

	return err
}

// NewPGStore returns a new PG Store that provides the CRUD based API need for
// supporting drand serialization.
func NewPGStore(l log.Logger, db *sqlx.DB, beaconName string, isTest bool) (*Store, error) {
	beaconName = strings.ToLower(beaconName)

	ctx := context.Background()

	err := doBootstrapFuncs(ctx, db, isTest)
	if err != nil {
		return nil, err
	}

	//language=postgresql
	query := `SELECT drand_maketable(:tableName)`

	data := struct {
		TableName string `db:"tableName"`
	}{
		TableName: beaconName,
	}

	err = database.NamedExecContext(ctx, l, db, query, data)
	if err != nil {
		return nil, err
	}

	p := Store{
		log:        l,
		db:         db,
		beaconName: beaconName,
	}
	return &p, nil
}

// Len returns the number of beacons in the configured beacon table.
func (p *Store) Len() (int, error) {
	//language=postgresql
	query := `SELECT drand_tablesize(:tableName) AS table_size`

	data := struct {
		TableName string `db:"tableName"`
	}{
		TableName: p.beaconName,
	}

	var ret struct {
		Count int `db:"table_size"`
	}
	if err := database.NamedQueryStruct(context.Background(), p.log, p.db, query, data, &ret); err != nil {
		return 0, err
	}

	return ret.Count, nil
}

// Put adds the specified beacon to the configured beacon table.
func (p *Store) Put(b *chain.Beacon) error {
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

	//language=postgresql
	query := `SELECT drand_insertround(:tableName, :round, :signature, :previous_sig)`

	if err := database.NamedExecContext(context.Background(), p.log, p.db, query, data); err != nil {
		return err
	}

	return nil
}

// Last returns the last beacon stored in the configured beacon table.
func (p *Store) Last() (*chain.Beacon, error) {
	//language=postgresql
	query := `SELECT round, signature, previous_sig FROM drand_getlastround(:tableName) WHERE round IS NOT NULL`

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
	query := `SELECT round, signature, previous_sig FROM drand_getround(:tableName, :round) WHERE round IS NOT NULL`

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
	// TODO (dlsniper): we should use soft delete here, probably.
	data := struct {
		TableName string `db:"tableName"`
		Round     uint64 `db:"round"`
	}{
		TableName: p.beaconName,
		Round:     round,
	}

	//language=postgresql
	query := `SELECT DRAND_DeleteRound(:tableName, :round)`

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
	defer func() {
		c.pos = 0
	}()

	//language=postgresql
	query := `SELECT round, signature, previous_sig FROM drand_getfirstround(:tableName) WHERE round IS NOT NULL`

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

	//language=postgresql
	query := `SELECT round, signature, previous_sig FROM drand_getoffsetround(:tableName, :offset) WHERE round IS NOT NULL`

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
	//language=postgresql
	query := `SELECT round, signature, previous_sig FROM drand_getround(:tableName, :round) WHERE round IS NOT NULL`

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
	//language=postgresql
	query := `SELECT round, signature, previous_sig FROM drand_getlastround(:tableName) WHERE round IS NOT NULL`

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
	//language=postgresql
	query := `SELECT round_offset FROM drand_getroundposition(:tableName, :round) WHERE round_offset IS NOT NULL`
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

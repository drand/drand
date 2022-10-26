// Package pg implements the required details to use PostgreSQL as a storage engine.
package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/log"
)

// Options holds all the options available for this storage engine
type Options struct{}

type PGStore struct {
	log       log.Logger
	tableName string
	db        *sqlx.DB
}

// ErrNoBeaconSaved is the error returned when no beacon have been saved in the
// database yet.
var ErrNoBeaconSaved = errors.New("beacon not found in database")

// language=postgresql
const createTableQuery = `CREATE TABLE IF NOT EXISTS %s (
    round   BIGINT NOT NULL CONSTRAINT %[1]s_pk PRIMARY KEY,
    sig     BYTEA  NOT NULL,
    prevsig BYTEA  NOT NULL
)`

// NewPGStore returns a Store implementation using the PostgreSQL storage engine.
// TODO implement options.
// TODO figure out if/how the DB connection can be initialized only once and shared between beacons.
func NewPGStore(l log.Logger, tableName string, opts *Options) (PGStore, error) {
	sslMode := "require"
	if true {
		sslMode = "disable"
	}

	q := make(url.Values)
	q.Set("sslmode", sslMode)
	q.Set("timezone", "utc")

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("drand", "drand"),
		Host:     "127.0.0.1:35432",
		Path:     "drand",
		RawQuery: q.Encode(),
	}

	db, err := sqlx.Open("postgres", u.String())
	if err != nil {
		return PGStore{}, err
	}
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(100)

	_, err = db.QueryxContext(context.Background(), fmt.Sprintf(createTableQuery, tableName))
	if err != nil {
		return PGStore{}, err
	}

	pg := PGStore{
		log:       l,
		tableName: tableName,
		db:        db,
	}
	return pg, nil
}

const lenQuery = `SELECT count(*) FROM %s`

func (p PGStore) Len() int {
	count := 0
	err := p.db.GetContext(
		context.Background(),
		&count,
		fmt.Sprintf(lenQuery, p.tableName),
	)
	if err != nil {
		p.log.Errorw("error getting length", "err", err)
	}

	return count
}

const putQuery = `INSERT INTO %s (round, sig, prevsig) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`

func (p PGStore) Put(beacon *chain.Beacon) error {
	_, err := p.db.QueryxContext(
		context.Background(),
		fmt.Sprintf(putQuery, p.tableName),
		beacon.Round,
		beacon.Signature,
		beacon.PreviousSig,
	)
	return err
}

const lastQuery = `SELECT round, sig, prevsig FROM %s ORDER BY round DESC LIMIT 1`

func (p PGStore) Last() (*chain.Beacon, error) {
	return beaconFromQuery(p.log, p.db, fmt.Sprintf(lastQuery, p.tableName))
}

const getQuery = `SELECT round, sig, prevsig FROM %s WHERE round=$1`

func (p PGStore) Get(round uint64) (*chain.Beacon, error) {
	return beaconFromQuery(p.log, p.db, fmt.Sprintf(getQuery, p.tableName), round)
}

func (p PGStore) Cursor(fn func(chain.Cursor)) {
	fn(&pgCursor{p.log, p.db, p.tableName, 0})
}

func (p PGStore) Close() {
	// TODO nothing?
}

func (p PGStore) Del(round uint64) error {
	// TODO implement me
	panic("implement me")
}

func (p PGStore) SaveTo(w io.Writer) error {
	// TODO implement me
	panic("implement me")
}

type pgCursor struct {
	log       log.Logger
	db        *sqlx.DB
	tableName string
	pos       int
}

const firstQuery = `SELECT round, sig, prevsig FROM %s ORDER BY round ASC LIMIT 1`

func (p *pgCursor) First() *chain.Beacon {
	defer func() {
		p.pos++
	}()
	ret, _ := beaconFromQuery(p.log, p.db, fmt.Sprintf(firstQuery, p.tableName))
	return ret
}

const nextQuery = `SELECT round, sig, prevsig FROM %s ORDER BY round ASC OFFSET $1 LIMIT 1`

func (p *pgCursor) Next() *chain.Beacon {
	defer func() {
		p.pos++
	}()
	ret, err := beaconFromQuery(p.log, p.db, fmt.Sprintf(nextQuery, p.tableName), p.pos)
	if errors.Is(err, ErrNoBeaconSaved) {
		return nil
	}
	return ret
}

func (p *pgCursor) Seek(round uint64) *chain.Beacon {
	ret, _ := beaconFromQuery(p.log, p.db, fmt.Sprintf(getQuery, p.tableName), round)
	return ret
}

func (p *pgCursor) Last() *chain.Beacon {
	ret, _ := beaconFromQuery(p.log, p.db, fmt.Sprintf(lastQuery, p.tableName))
	return ret
}

func beaconFromQuery(l log.Logger, db *sqlx.DB, query string, args ...interface{}) (*chain.Beacon, error) {
	b := struct {
		Round       uint64 `db:"round"`
		Signature   []byte `db:"sig"`
		PreviousSig []byte `db:"prevsig"`
	}{}
	err := db.GetContext(context.Background(), &b, query, args...)
	if err != nil {
		l.Errorw("failed to run query", "query", query, "args", args, "err", err)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoBeaconSaved
		}
		return nil, err
	}

	return &chain.Beacon{
		Round:       b.Round,
		Signature:   b.Signature,
		PreviousSig: b.PreviousSig,
	}, nil
}

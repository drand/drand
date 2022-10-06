// Package pg implements the required details to use PostgreSQL as a storage engine.
package pg

import (
	"io"
	"net/url"

	"github.com/drand/drand/chain"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Options holds all the options available for this storage engine
type Options struct{}

type pgStore struct {
	tableName string
	db        *sqlx.DB
}

// NewPGStore returns a Store implementation using the PostgreSQL storage engine.
// TODO implement options.
// TODO figure out if/how the DB connection can be initialized only once and shared between beacons.
func NewPGStore(tableName string, opts *Options) (chain.Store, error) {
	sslMode := "require"
	if true {
		sslMode = "disable"
	}

	q := make(url.Values)
	q.Set("sslmode", sslMode)
	q.Set("timezone", "utc")

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("user", "pass"),
		Host:     "127.0.0.1:5432",
		Path:     "drand",
		RawQuery: q.Encode(),
	}

	db, err := sqlx.Open("postgres", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(5)
	db.SetMaxOpenConns(100)

	pg := pgStore{
		db: db,
	}
	return pg, nil
}

func (p pgStore) Len() int {
	// TODO implement me
	panic("implement me")
}

func (p pgStore) Put(beacon *chain.Beacon) error {
	// TODO implement me
	panic("implement me")
}

func (p pgStore) Last() (*chain.Beacon, error) {
	// TODO implement me
	panic("implement me")
}

func (p pgStore) Get(round uint64) (*chain.Beacon, error) {
	// TODO implement me
	panic("implement me")
}

func (p pgStore) Cursor(fn func(chain.Cursor)) {
	// TODO implement me
	fn(&pgCursor{})
	panic("implement me")
}

func (p pgStore) Close() {
	// TODO implement me
	panic("implement me")
}

func (p pgStore) Del(round uint64) error {
	// TODO implement me
	panic("implement me")
}

func (p pgStore) SaveTo(w io.Writer) error {
	// TODO implement me
	panic("implement me")
}

// TODO implement this
type pgCursor struct{}

func (p pgCursor) First() *chain.Beacon {
	// TODO implement me
	panic("implement me")
}

func (p pgCursor) Next() *chain.Beacon {
	// TODO implement me
	panic("implement me")
}

func (p pgCursor) Seek(round uint64) *chain.Beacon {
	// TODO implement me
	panic("implement me")
}

func (p pgCursor) Last() *chain.Beacon {
	// TODO implement me
	panic("implement me")
}

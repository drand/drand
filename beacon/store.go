package beacon

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	bolt "github.com/coreos/bbolt"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/nikkolasg/slog"
)

// store contains all the definitions and implementation of the logic that
// stores and loads beacon signatures. At the moment of writing, it consists of
// a boltdb key/value database store.

// Store is an interface to store Beacons packets where they can also be
// retrieved to be delivered to end clients.
type Store interface {
	Len() int
	Put(*Beacon) error
	Last() (*Beacon, error)
	Get(round uint64) (*Beacon, error)
	Cursor(func(Cursor))
	// XXX Misses a delete function
	Close()
	del(rount uint64)
}

// Iterate over items in sorted key order. This starts from the
// first key/value pair and updates the k/v variables to the
// next key/value on each iteration.
//
// The loop finishes at the end of the cursor when a nil key is returned.
//    for k, v := c.First(); k != nil; k, v = c.Next() {
//        fmt.Printf("A %s is %s.\n", k, v)
//    }
type Cursor interface {
	First() *Beacon
	Next() *Beacon
	Seek(round uint64) *Beacon
	Last() *Beacon
}

// boldStore implements the Store interface using the kv storage boltdb (native
// golang implementation). Internally, Beacons are stored as JSON-encoded in the
// db file.
type boltStore struct {
	sync.Mutex
	db  *bolt.DB
	len int
}

var beaconBucket = []byte("beacons")

// BoltFileName is the name of the file boltdb writes to
const BoltFileName = "drand.db"

// NewBoltStore returns a Store implementation using the boltdb storage engine.
func NewBoltStore(folder string, opts *bolt.Options) (Store, error) {
	dbPath := path.Join(folder, BoltFileName)
	db, err := bolt.Open(dbPath, 0660, opts)
	if err != nil {
		return nil, err
	}
	var baseLen = 0
	// create the bucket already
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(beaconBucket)
		if err != nil {
			return err
		}
		baseLen += bucket.Stats().KeyN
		return nil
	})

	return &boltStore{
		db:  db,
		len: baseLen,
	}, err
}

func (b *boltStore) Len() int {
	var length = 0
	b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		length = bucket.Stats().KeyN
		return nil
	})
	return length
}

func (b *boltStore) Close() {
	if err := b.db.Close(); err != nil {
		slog.Debugf("boltdb store: %s", err)
	}
}

// Put implements the Store interface. WARNING: It does NOT verify that this
// beacon is not already saved in the database or not.
func (b *boltStore) Put(beacon *Beacon) error {
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		key := roundToBytes(beacon.Round)
		buff, err := beacon.Marshal()
		if err != nil {
			return err
		}
		return bucket.Put(key, buff)
	})
	if err != nil {
		return err
	}
	return nil
}

// ErrNoBeaconSaved is the error returned when no beacon have been saved in the
// database yet.
var ErrNoBeaconSaved = errors.New("beacon not found in database")

// Last returns the last beacon signature saved into the db
func (b *boltStore) Last() (*Beacon, error) {
	var beacon *Beacon
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		cursor := bucket.Cursor()
		_, v := cursor.Last()
		if v == nil {
			return ErrNoBeaconSaved
		}
		b := &Beacon{}
		if err := b.Unmarshal(v); err != nil {
			return err
		}
		beacon = b
		return nil
	})
	return beacon, err
}

// Get returns the beacon saved at this round
func (b *boltStore) Get(round uint64) (*Beacon, error) {
	var beacon *Beacon
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		v := bucket.Get(roundToBytes(round))
		if v == nil {
			return ErrNoBeaconSaved
		}
		b := &Beacon{}
		if err := b.Unmarshal(v); err != nil {
			return err
		}
		beacon = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return beacon, err
}

func (b *boltStore) del(round uint64) {
	b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		return bucket.Delete(roundToBytes(round))
	})
}

func (b *boltStore) Cursor(fn func(Cursor)) {
	b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(beaconBucket)
		c := bucket.Cursor()
		fn(&boltCursor{Cursor: c})
		return nil
	})
}

type boltCursor struct {
	*bolt.Cursor
}

func (c *boltCursor) First() *Beacon {
	k, v := c.Cursor.First()
	if k == nil {
		return nil
	}
	b := new(Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

func (c *boltCursor) Next() *Beacon {
	k, v := c.Cursor.Next()
	if k == nil {
		return nil
	}
	b := new(Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

func (c *boltCursor) Seek(round uint64) *Beacon {
	k, v := c.Cursor.Seek(roundToBytes(round))
	if k == nil {
		return nil
	}
	b := new(Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

func (c *boltCursor) Last() *Beacon {
	k, v := c.Cursor.Last()
	if k == nil {
		return nil
	}
	b := new(Beacon)
	if err := b.Unmarshal(v); err != nil {
		return nil
	}
	return b
}

type CallbackStore struct {
	Store
	cbs []func(*Beacon)
	sync.Mutex
}

// NewCallbackStore returns a Store that calls the given callback in a goroutine
// each time a new Beacon is saved into the given store. It does not call the
// callback if there has been any errors while saving the beacon.
func NewCallbackStore(s Store) *CallbackStore {
	return &CallbackStore{Store: s}
}

func (c *CallbackStore) Put(b *Beacon) error {
	if err := c.Store.Put(b); err != nil {
		return err
	}
	if b.Round != 0 {
		go func() {
			c.Lock()
			defer c.Unlock()
			for _, cb := range c.cbs {
				cb(b)
			}
		}()
	}
	return nil
}

func (c *CallbackStore) AddCallback(fn func(*Beacon)) {
	c.Lock()
	defer c.Unlock()
	c.cbs = append(c.cbs, fn)
}

func roundToBytes(r uint64) []byte {
	var buff bytes.Buffer
	binary.Write(&buff, binary.BigEndian, r)
	return buff.Bytes()
}

func printStore(s Store) string {
	time.Sleep(1 * time.Second)
	var out = ""
	s.Cursor(func(c Cursor) {
		for b := c.First(); b != nil; b = c.Next() {
			out += fmt.Sprintf("%s\n", b)
		}
	})
	return out
}

type roundCache struct {
	round       uint64
	previous    uint64
	previousSig []byte
	sigs        [][]byte
	seens       map[int]bool
	done        bool
}

func newRoundCache(round, prev uint64, prevSig []byte) *roundCache {
	return &roundCache{
		round:       round,
		previous:    prev,
		previousSig: prevSig,
		seens:       make(map[int]bool),
	}
}

func (cache *roundCache) tryAppend(p *drand.PartialBeaconPacket) bool {
	round := p.GetRound()
	prevRound := p.GetPreviousRound()
	prevSig := p.GetPreviousSig()
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
	if _, seen := cache.seens[idx]; seen {
		return false
	}

	sameRound := round == cache.round
	samePrevR := prevRound == cache.previous
	samePrevS := bytes.Equal(prevSig, cache.previousSig)
	if sameRound && samePrevR && samePrevS {
		cache.sigs = append(cache.sigs, p.GetPartialSig())
		cache.seens[idx] = true
		return true
	}
	return false
}

func (r *roundCache) Len() int {
	return len(r.sigs)
}

func (r *roundCache) Msg() []byte {
	return Message(r.previousSig, r.previous, r.round)
}

func (r *roundCache) Partials() [][]byte {
	return r.sigs
}

// chainStore is a Store that deals with reconstructing the beacons, sync when
// needed and arranges the head
type chainStore struct {
	Store
	l                log.Logger
	client           net.ProtocolClient
	safe             *cryptoSafe
	ticker           *ticker
	done             chan bool
	newPartials      chan partialInfo
	newAggregated    chan *Beacon
	newBeaconNetwork chan *Beacon
	newBeaconSync    chan *Beacon
	lastInserted     chan *Beacon
	syncNeeded       chan *Beacon
}

func newChainStore(l log.Logger, client net.ProtocolClient, safe *cryptoSafe, s Store, ticker *ticker) *chainStore {
	chain := &chainStore{
		l:                l,
		client:           client,
		safe:             safe,
		Store:            s,
		done:             make(chan bool, 1),
		ticker:           ticker,
		newPartials:      make(chan partialInfo, 10),
		newAggregated:    make(chan *Beacon, 1),
		newBeaconNetwork: make(chan *Beacon, 100),
		lastInserted:     make(chan *Beacon, 1),
		syncNeeded:       make(chan *Beacon, 1),
		newBeaconSync:    make(chan *Beacon, 100),
	}
	// TODO maybe look if it's worth having multiple workers there
	go chain.runChainLoop()
	go chain.runAggregator()
	return chain
}

func (c *chainStore) NewValidPartial(addr string, p *drand.PartialBeaconPacket) {
	c.newPartials <- partialInfo{
		addr: addr,
		p:    p,
	}
}

func (c *chainStore) NewBeacon(addr string, proto *drand.BeaconPacket) {
	c.newBeaconNetwork <- protoToBeacon(proto)
}

func (c *chainStore) RunSync() {
	b, _ := c.Store.Last()
	c.syncNeeded <- b
}

func (c *chainStore) Stop() {
	close(c.done)
}

// runAggregator runs a continuous loop that tries to aggregate partial
// signatures when it can
func (c *chainStore) runAggregator() {
	var caches []*roundCache
	newRound := c.ticker.Channel()
	lastBeacon, _ := c.Store.Last()
	var currRound = roundInfo{
		round: c.ticker.CurrentRound(),
	}
	for {
		select {
		case <-c.done:
			return
		case lastBeacon = <-c.lastInserted:
			break
		case currRound = <-newRound:
			// remove all caches that are previous to this round
			var filtered []*roundCache
			for _, cache := range caches {
				if cache.round <= currRound.round {
					continue
				}
				filtered = append(filtered, cache)
			}
			fmt.Println(" AGGREGATOR RECEIVED NEW ROUND FROM TICKER", currRound.round)
			c.l.Debug("new_round", currRound.round, "filtered_cache", fmt.Sprintf("%d/%d", len(filtered), len(caches)))
			caches = filtered
		case partial := <-c.newPartials:
			var cache *roundCache
			for _, c := range caches {
				if !c.tryAppend(partial.p) {
					continue
				}
				cache = c
			}

			ginfo, err := c.safe.GetInfo(partial.p.GetRound())
			if err != nil {
				c.l.Error("no_info_for", partial.p.GetRound())
				continue
			}
			// +1 is because depending on clock skew, this node may not have
			// passed yet to the new round for which he receives a partial.
			shouldStore := partial.p.GetRound() == currRound.round || partial.p.GetRound() == currRound.round+1
			if !shouldStore {
				c.l.Error("ignoring_partial", partial.p.GetRound(), "current_round", currRound.round)
				continue
			}
			if cache == nil {
				cache = newRoundCache(partial.p.GetRound(), partial.p.GetPreviousRound(), partial.p.GetPreviousSig())
				caches = append(caches, cache)
				if !cache.tryAppend(partial.p) {
					c.l.Fatal("bug_cache_partial")
				}
			}
			thr := ginfo.group.Threshold
			c.l.Debug("store_partial", partial.addr, "round", cache.round, "len_partials", fmt.Sprintf("%d/%d", cache.Len(), thr))

			// check if we can reconstruct
			if cache.done || cache.Len() < thr {
				// check if it doesn't correspond to what we want, we may want to
				// sync. 2 because we dont want to sync as soon as we get one,
				// it may be a random one - aritrarily chosen XXX put more
				// thoughts into that.
				if lastBeacon != nil && cache.Len() >= 2 {
					c.maybeRunSync(currRound, lastBeacon, partial.p)
				}
				break
			}
			pub := ginfo.pub
			n := ginfo.group.Len()
			msg := cache.Msg()
			finalSig, err := key.Scheme.Recover(pub, msg, cache.Partials(), thr, n)
			if err != nil {
				c.l.Debug("invalid_recovery", err, "round", partial.p.GetRound(), "got", fmt.Sprintf("%d/%d", cache.Len(), n))
				break
			}
			if err := key.Scheme.VerifyRecovered(pub.Commit(), msg, finalSig); err != nil {
				c.l.Error("invalid_sig", err, "round", partial.p.GetRound(), "prev", partial.p.GetPreviousRound())
				return
			}
			cache.done = true
			newBeacon := &Beacon{
				Round:         cache.round,
				PreviousRound: cache.previous,
				PreviousSig:   cache.previousSig,
				Signature:     finalSig,
			}
			c.l.Info("aggregated_beacon", newBeacon.Round, "previous_round", newBeacon.PreviousRound)
			c.newAggregated <- newBeacon
			break
		}
	}
}

func (c *chainStore) runChainLoop() {
	lastBeacon, err := c.Store.Last()
	if err != nil {
		c.l.Fatal("store_last_init", err)
	}
	newRound := c.ticker.Channel()
	var currRound = roundInfo{
		round: c.ticker.CurrentRound(),
	}
	insert := func(newB *Beacon) {
		c.l.Info("new_beacon", "storing", "round", newB.Round, "prev_round", newB.PreviousRound, "signature", shortSigStr(newB.Signature))
		if err := c.Store.Put(newB); err != nil {
			c.l.Fatal("new_beacon_storing", err)
		}
		lastBeacon = newB
		c.lastInserted <- newB
	}
	var synci *syncInfo
	for {
		select {
		case newBeacon := <-c.newAggregated:
			if c.isReorg(lastBeacon, newBeacon) {
				// TODO write depending on the final specs
				break
			}
			if !c.isAppendable(lastBeacon, newBeacon) {
				fmt.Println(" NOT APPENDABLE?")
				c.maybeRunSync(currRound, lastBeacon, newBeacon)
				break
			}
			insert(newBeacon)
		case newBeacon := <-c.newBeaconNetwork:
			if lastBeacon.Equal(newBeacon) {
				// we dont even verify it
				break
			}
			if c.isReorg(lastBeacon, newBeacon) {
				// TODO write depending on the final specs
				break
			}
			if newBeacon.Round < currRound.round {
				// beacon in the past whereas it should be current round,
				// we don't care - simple DOS protection
				break
			}
			info, err := c.safe.GetInfo(newBeacon.GetRound())
			if err != nil {
				c.l.Error("no_info_round", newBeacon.GetRound())
				continue
			}
			if err := VerifyBeacon(info.pub.Commit(), newBeacon); err != nil {
				c.l.Debug("invalid_beacon", err, "round", newBeacon.Round, "prev", newBeacon.PreviousRound)
				break
			}
			if !c.isAppendable(lastBeacon, newBeacon) {
				// if it's not appendable directly, we may need to sync with
				// other nodes
				c.maybeRunSync(currRound, lastBeacon, newBeacon)
				break
			}
			insert(newBeacon)
		case newBeacon := <-c.newBeaconSync:
			fmt.Println("NEW BEACON FROM SYNC", newBeacon)
			if lastBeacon.Equal(newBeacon) {
				c.l.Debug("sync_beacon", "equal_to_last", newBeacon.Round)
				break
			}
			if c.isReorg(lastBeacon, newBeacon) {
				break
			}
			if !c.isAppendable(lastBeacon, newBeacon) {
				c.l.Debug("sync_beacon", "not_appendable", "last", lastBeacon.String(), "new", newBeacon.String())
				break
			}
			insert(newBeacon)
			// check if we stil need to sync
			if newBeacon.Round == currRound.round {
				// sounds safe to stop now
				if synci == nil {
					c.l.Error("sync_info", "nil")
				} else {
					synci.cancel()
				}
			}
		case info := <-newRound:
			currRound = info
			if synci != nil {
				synci = nil
			}
		case _ = <-c.syncNeeded:
			if synci != nil {
				// we already ran a sync process for this round
				break
			}
			ctx, cancel := context.WithCancel(context.Background())
			synci = &syncInfo{
				to:     currRound.round,
				from:   lastBeacon.Round,
				cancel: cancel,
			}
			go syncChain(ctx, c.l, c.safe, lastBeacon, synci.to, c.client, c.newBeaconSync)
		case <-c.done:
			return
		}
	}
}

func (c *chainStore) isReorg(last, newb *Beacon) bool {
	// TODO
	return false
}

func (c *chainStore) isAppendable(lastBeacon, newBeacon *Beacon) bool {
	if lastBeacon.Round >= newBeacon.Round {
		c.l.Debug("invalid_new_round", newBeacon.Round, "last_beacon_round", lastBeacon.Round, "new>last?", lastBeacon.Round >= newBeacon.Round)
		return false
	}

	if lastBeacon.Round != newBeacon.PreviousRound {
		c.l.Debug("invalid_previous_round", newBeacon.Round, "last_beacon_previous_round", lastBeacon.Round)
		return false
	}

	if !bytes.Equal(lastBeacon.Signature, newBeacon.PreviousSig) {
		c.l.Debug("invalid_previous_signature", shortSigStr(newBeacon.Signature), "last_beacon_signature", shortSigStr(lastBeacon.Signature))
		return false
	}
	return true
}

type likeBeacon interface {
	GetRound() uint64
	GetPreviousRound() uint64
}

// This MUST be called only if isAppendable returns false
func (c *chainStore) maybeRunSync(curr roundInfo, last *Beacon, newB likeBeacon) {
	if newB.GetPreviousRound() > last.GetRound() {
		// run sync !
		c.syncNeeded <- last
	}
}

type partialInfo struct {
	addr string
	p    *drand.PartialBeaconPacket
}

type beaconInfo struct {
	addr   string
	beacon *Beacon
}

type syncInfo struct {
	from   uint64
	to     uint64
	cancel context.CancelFunc
}

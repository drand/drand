package net

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dedis/kyber"
	"github.com/nikkolasg/dsign/key"
	"github.com/nikkolasg/dsign/net/transport"
	"github.com/nikkolasg/slog"
)

type Private interface {
	Key() kyber.Scalar
	Public()
}

type Identity interface {
	Public() kyber.Point
	Address() string
}

// Gateway is the gateway between dsign and the rest of the world. It enables to
// send messages and to receive messages. It uses a given underlying transport
// for its communication. For the moment,
type Gateway interface {
	// Gateway uses an underlying transport mechanism. Note that if you use any
	// function of the transport itself directly, the Gateway is not
	// responsible and do not manage any connection made this way.
	Transport() transport.Transport
	// Send sends a message to the given peer represented by this identity.
	Send(to *key.Identity, msg []byte) error
	// Broadcast sends the same message to the given group. Implementations must
	// return an error in case at least one transmission went wrong.
	Broadcast(group []*key.Identity, msg []byte) error
	// Start runs the Transport. The given Processor will be handled any new
	// incoming packets from the Transport. It is a non blocking call.
	Start(Processor) error
	// Stop closes all conections and stop the listening
	Stop() error
}

// Processor is a function that receives messages from the network
type Processor func(from *key.Identity, msg []byte)

// gateway is a straightforward implementation of a Gateway.
type gateway struct {
	id        *key.Identity       // the public identity of the running gw
	transport transport.Transport // the underlying transport
	conns     *connStore          // the list of active connections
	processor Processor           // the processor to call upon new packets
	closed    bool                // true if the gateway is closed already
	wg        sync.WaitGroup      // to count all goroutines started
	sync.Mutex
}

// NewGateway returns a default gateway using the underlying given transport
// implementation.
func NewGateway(id *key.Identity, t transport.Transport) Gateway {
	return &gateway{
		id:        id,
		transport: t,
		conns:     newConnStore(id.ID),
	}
}

func (g *gateway) Send(to *key.Identity, msg []byte) error {
	if to.ID == g.id.ID {
		panic("whoa are we sending to ourself!?")
	}
	var err error
	conn, ok := g.conns.Get(to.ID)
	if !ok {
		conn, err = g.connect(to)
		if err != nil {
			return err
		}
	}
	return sendBytes(conn, msg)
}

// Broadcasts send the given message to each peers in the list EXCEPT its own.
func (g *gateway) Broadcast(group []*key.Identity, msg []byte) error {
	var errStr string
	var errMut sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(group) - 1)
	for _, id := range group {
		if g.id.Equals(id) {
			continue
		}
		go func(to *key.Identity) {
			if err := g.Send(to, msg); err != nil {
				errMut.Lock()
				errStr += err.Error() + "\n"
				errMut.Unlock()
			}
			wg.Done()
		}(id)
	}
	wg.Wait()
	if errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

func (g *gateway) runNewConn(remote *key.Identity, c transport.Conn) {
	g.conns.Add(remote.ID, c)
	g.wg.Add(1)
	go g.listenIncoming(remote, c)
}

func (g *gateway) listenIncoming(remote *key.Identity, c transport.Conn) {
	defer func() {
		g.wg.Done()
		g.conns.Del(remote.ID)
		c.Close()
	}()
	for {
		buff, err := rcvBytes(c)
		if err != nil {
			//fmt.Printf("gateway %p: error receiving from %s: %s\n", g, remote.Address, err)
			return
		}
		if g.processor == nil {
			continue
		}
		// XXX maybe switch to a consumer/producer style if needed
		g.processor(remote, buff)
	}
}

// drand retries to connect with exponential back off time with the base time
// being baseRetryTime. In total, the worst case wait time is
// baseRetryTime * 2^(maxRetryConnect)
// = 500 * 2^10 / 1000 / 60 =  8.5m
var baseRetryTime = 500 * time.Millisecond

// how many time do we try to connect to a given node
var maxRetryConnect = 10

// connect tries to connect multiple to the given identity with exponential
// backoff timeout between trials.
func (g *gateway) connect(p *key.Identity) (net.Conn, error) {
	var nTries = 1
	var waitTime = baseRetryTime
	var c net.Conn
	var err error
	for nTries <= maxRetryConnect {
		slog.Debug("gateway: trying to connect to", p.Address)
		c, err = g.transport.Dial(p)
		if err == nil {
			break
		}
		waitTime = waitTime * time.Duration(nTries)
		time.Sleep(waitTime)
		nTries++
	}
	//c, err := net.Dial("tcp", p.Address)
	if err != nil {
		//slog.Info("router: failed to connect to ", p.Address, " after ", nTries, " times")
		return nil, err
	}
	g.runNewConn(p, c)
	fmt.Println("gateway: successful connection to ", p.Address)
	return c, nil
}

func (g *gateway) Start(h Processor) error {
	if g.processor != nil {
		return errors.New("router only supports one handler registration")
	}
	g.processor = h
	go g.transport.Listen(g.runNewConn)
	return nil
}

func (g *gateway) Stop() error {
	g.Lock()
	if g.closed {
		g.Unlock()
		return nil
	}
	g.closed = true
	g.Unlock()

	g.conns.CloseAll()

	if err := g.transport.Close(); err != nil {
		slog.Debugf("gateway: error closing listener: %s", err)
	}

	g.wg.Wait()
	return nil
}

func (g *gateway) Transport() transport.Transport {
	return g.transport
}

func (g *gateway) isClosed() bool {
	g.Lock()
	defer g.Unlock()
	return g.closed
}

func sendBytes(c net.Conn, b []byte) error {
	packetSize := len(b)
	if packetSize > MaxPacketSize {
		return fmt.Errorf("sending too much (%d bytes) to %s", packetSize, c.RemoteAddr().String())
	}
	// first write the size
	if err := binary.Write(c, globalOrder, uint32(packetSize)); err != nil {
		return err
	}

	// then send everything through the connection
	// send chunk by chunk
	var sent int
	for sent < packetSize {
		n, err := c.Write(b[sent:])
		if err != nil {
			return err
		}
		sent += n
	}
	return nil
}

func rcvBytes(c net.Conn) ([]byte, error) {
	c.SetReadDeadline(time.Now().Add(readTimeout))
	// First read the size
	var total uint32
	if err := binary.Read(c, globalOrder, &total); err != nil {
		return nil, err
	}
	if total > MaxPacketSize {
		return nil, fmt.Errorf("too big packet (%d bytes) from %s", total, c.RemoteAddr().String())
	}

	b := make([]byte, total)
	var buffer bytes.Buffer
	var read uint32
	for read < total {
		// read the size of the next packet.
		c.SetReadDeadline(time.Now().Add(readTimeout))
		n, err := c.Read(b)
		// quit if there is an error.
		if err != nil {
			return nil, err
		}
		// append the read bytes into the buffer.
		if _, err := buffer.Write(b[:n]); err != nil {
			return nil, err
		}
		b = b[n:]
		read += uint32(n)
	}
	return buffer.Bytes(), nil
}

// a connection will return an io.EOF after readTimeout if nothing has been
// sent.
var readTimeout = 1 * time.Minute

// MaxPacketSize represents the maximum number of bytes can we receive or write
// to a net.Conn in bytes.
const MaxPacketSize = 1300

// globalOrder is the endianess used to write the size of a message.
var globalOrder = binary.BigEndian

type connStore struct {
	Conns map[string][]net.Conn
	own   string
	sync.Mutex
}

func newConnStore(own string) *connStore {
	return &connStore{
		own:   own,
		Conns: make(map[string][]net.Conn),
	}
}

func (c *connStore) Add(id string, conn net.Conn) {
	c.Lock()
	defer c.Unlock()
	c.Conns[id] = append(c.Conns[id], conn)
}

func (c *connStore) Get(id string) (net.Conn, bool) {
	c.Lock()
	defer c.Unlock()
	arr, ok := c.Conns[id]
	if !ok {
		return nil, false
	}
	var idx int
	if c.own == id {
		panic("that should never happen => send to ourself!!=??" + id + ":" + c.own)
	}
	if c.own < id {
		idx = 0
	} else if len(arr) > 1 && c.own > id {
		idx = 1
	}
	//idx = 0
	return arr[idx], true
}

func (c *connStore) Del(id string) {
	c.Lock()
	defer c.Unlock()
	delete(c.Conns, id)
}

func (c *connStore) CloseAll() {
	c.Lock()
	defer c.Unlock()
	for id, arr := range c.Conns {
		for _, c := range arr {
			if err := c.Close(); err != nil {
				if strings.Contains(err.Error(), "closed network") {
					continue
				}
				slog.Debugf("gateway: err closing conn to %s: %s", id, err)
			}
		}
	}
}

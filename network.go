package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dedis/protobuf"
	"github.com/nikkolasg/slog"
)

// a connection will return an io.EOF after readTimeout if nothing has been
// sent.
var readTimeout = 1 * time.Minute

// how much time do a router have to wait for an incoming connection
var maxIncomingWaitTime = 10 * time.Second

// Conn is a wrapper around the native golang connection that provides a
// automatic encoding and decoding of protobuf encoded messages.
type Conn struct {
	net.Conn
}

// Send marshals the given Drand packet and write it on the underlying
// connection.
func (c *Conn) Send(d *DrandPacket) error {
	b, err := protobuf.Encode(d)
	if err != nil {
		return err
	}

	packetSize := uint16(len(b))
	if err := binary.Write(c.Conn, binary.LittleEndian, packetSize); err != nil {
		return err
	}

	// then send everything through the connection
	// Send chunk by chunk
	var sent uint16
	for sent < packetSize {
		n, err := c.Conn.Write(b[sent:])
		if err != nil {
			return err
		}
		sent += uint16(n)
	}
	return nil
}

func (c *Conn) Receive() ([]byte, error) {
	c.Conn.SetReadDeadline(time.Now().Add(readTimeout))
	// First read the size
	var total uint16
	if err := binary.Read(c.Conn, binary.LittleEndian, &total); err != nil {
		return nil, err
	}

	b := make([]byte, total)
	var read uint16
	var buffer bytes.Buffer
	for read < total {
		// Read the size of the next packet.
		c.Conn.SetReadDeadline(time.Now().Add(readTimeout))
		n, err := c.Conn.Read(b)
		// Quit if there is an error.
		if err != nil {
			return nil, err
		}
		// Append the read bytes into the buffer.
		if _, err := buffer.Write(b[:n]); err != nil {
			slog.Debug("Couldn't write to buffer:", err)
		}
		read += uint16(n)
		b = b[n:]
	}
	return buffer.Bytes(), nil
}

// Router holds all incoming and outgoing alive connections, permits application
// layer above to send and receive messages with each connections mapped to a
// public identity.
type Router struct {
	priv  *Private
	list  *Group
	index int
	addr  string

	// key are ID of the public key
	conns map[string]Conn
	cond  *sync.Cond

	messages chan messageWrapper

	listener net.Listener
	listMut  sync.Mutex
}

func NewRouter(priv *Private, list *Group) *Router {
	idx, ok := list.Index(priv.Public)
	if !ok {
		panic("public key not found in the list")
	}
	return &Router{
		priv:     priv,
		index:    idx,
		list:     list,
		addr:     priv.Public.Address,
		conns:    make(map[string]Conn),
		messages: make(chan messageWrapper, 100),
		cond:     sync.NewCond(&sync.Mutex{}),
	}
}

// Listens opens a tcp port on the address taken in the public key
func (r *Router) Listen() {
	listener, err := net.Listen("tcp", r.addr)
	if err != nil {
		panic("can't listen on addresse: " + err.Error())
	}
	r.listMut.Lock()
	r.listener = listener
	r.listMut.Unlock()
	for {
		c, err := listener.Accept()
		if err != nil {
			slog.Info("error with listening: ", err)
			if strings.Contains(err.Error(), "closed") {
				return
			}
		}
		go r.handleIncoming(Conn{c})
	}
}

// Receive returns the next enqueued message coming from any active connections
func (r *Router) Receive() (*Public, []byte) {
	wrap := <-r.messages
	return wrap.Pub, wrap.Message
}

// Send checks if a connections exists and if so, marshals the packet and sends
// it through. If the connection does not exists, the router checks whether it
// must initiates the connection or wait for the destination to make the
// connection. It does so by looking at the index of the destination in the list
// of public keys. If the index of the router is higher than the one of the
// destination, the router waits for  destination to trigger the connection. If
// the index of the router is lower, then it initiates the connection.
func (r *Router) Send(pub *Public, d *DrandPacket) error {
	r.cond.L.Lock()
	slog.Debug(r.addr, "searching for conn to ", pub.Address)
	c, ok := r.conns[pub.Key.String()]
	r.cond.L.Unlock()
	if ok {
		// already connected to it
		slog.Debug(c.LocalAddr(), "sent to ", c.RemoteAddr())
		err := c.Send(d)
		return err
	}
	slog.Debug(r.addr, "no connection to ", pub.Address)
	// check action to take according to index
	ridx, ok := r.list.Index(pub)
	if !ok {
		return errors.New("router: does not know the public key")
	}
	if ridx > r.index {
		cc, err := r.connect(pub)
		if err != nil {
			return err
		}
		c = cc
	} else if ridx < r.index {
		// wait for incoming conn
		cc, err := r.waitIncoming(pub)
		if err != nil {
			return err
		}
		c = cc
	} else {
		return errors.New("router: don't send to ourself")
	}
	return c.Send(d)
}

func (r *Router) Stop() {
	r.listMut.Lock()
	r.listener.Close()
	r.listMut.Unlock()

	r.cond.L.Lock()
	for _, c := range r.conns {
		c.Close()
	}
	r.cond.L.Unlock()
}

// waitIncoming
func (r *Router) waitIncoming(pub *Public) (Conn, error) {
	id := pub.Key.String()
	var c *Conn
	// condition is that the connection is registered
	condition := func() bool {
		ci, ok := r.conns[id]
		if ok {
			c = &ci
			return true
		}
		return false
	}
	var timeout bool
	var timeLock sync.Mutex
	// trigger the lock after the maximum time out
	go func() {
		time.Sleep(maxIncomingWaitTime)
		timeLock.Lock()
		timeout = true
		timeLock.Unlock()
		r.cond.Broadcast()
	}()

	r.cond.L.Lock()
	for !condition() {
		r.cond.Wait()
		timeLock.Lock()
		defer timeLock.Unlock()
		if timeout {
			break
		}

	}
	r.cond.L.Unlock()
	if c == nil {
		return Conn{}, errors.New("router: time out waiting on incoming connection")
	}
	return *c, nil
}

// connect actively tries to connect to the address given in the Public and
// registers that connection to the router.
func (r *Router) connect(p *Public) (Conn, error) {
	c, err := net.Dial("tcp", p.Address)
	if err != nil {
		return Conn{}, err
	}
	cc := Conn{c}
	hello := &DrandPacket{Hello: r.priv.Public}
	slog.Debugf("router(Addr: %s / conn: %s): sending Hello message to %s", r.addr, c.LocalAddr(), c.RemoteAddr())
	if err := cc.Send(hello); err != nil {
		return Conn{}, err
	}
	return r.registerConn(p, c), nil
}

// registerConn simply puts the connection in the global map
func (r *Router) registerConn(pub *Public, c net.Conn) Conn {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	if ci, ok := r.conns[pub.Key.String()]; ok {
		slog.Debug("router: already connected to ", pub.Address)
		return ci
	}
	cc := Conn{c}
	r.conns[pub.Key.String()] = cc
	r.cond.Broadcast()
	return cc
}

// handleIncoming expects to receive the public identity of the remote party
// first, then handle the connection normally as in handleConn.
func (r *Router) handleIncoming(c Conn) {
	buff, err := c.Receive()
	if err != nil {
		slog.Debug("router: error receiving from ", c.RemoteAddr())
		return
	}

	drand, err := unmarshal(g2, buff)
	if err != nil {
		slog.Debug("router: error unmarshalling pub key from", c.RemoteAddr())
		return
	}
	if drand.Hello == nil {
		slog.Debugf("router(%s): no hello message from %s", c.LocalAddr(), c.RemoteAddr())
		return
	}
	pub := drand.Hello
	// chekc that we know this public key. Not a security measure but merely to
	// only deal with keys this router knows
	if !r.list.Contains(pub) {
		slog.Debug("router: unknown public key from ", c.RemoteAddr())
		return
	}

	r.handleConnection(pub, r.registerConn(pub, c.Conn))
}

func (r *Router) handleConnection(p *Public, c Conn) {
	for {
		buff, err := c.Receive()
		if err != nil {
			slog.Info("router: conn. error from ", p.Address)
			return
		}
		r.messages <- messageWrapper{Pub: p, Message: buff}
	}
}

type messageWrapper struct {
	Pub     *Public
	Message []byte
}

func host(c net.Conn) string {
	host, _, _ := net.SplitHostPort(c.RemoteAddr().Network())
	return host
}

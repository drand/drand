package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"time"

	kyber "gopkg.in/dedis/kyber.v1"

	"github.com/dedis/onet/log"
	"github.com/dedis/protobuf"
	"github.com/nikkolasg/slog"
)

// a connection will return an io.EOF after readTimeout if nothing has been
// sent.
var readTimeout = 1 * time.Minute

// Conn is a wrapper around the native golang connection that provides a
// automatic encoding and decoding of protobuf encoded messages.
type Conn struct {
	net.Conn
}

// Send marshals the given Drand packet and write it on the underlying
// connection.
func (c *Conn) Send(d *Drand) error {
	buff, err := protobuf.Encode(d)
	if err != nil {
		return err
	}

	_, err := c.Conn.Write(buff)
	return err
}

func (c *Conn) Receive() ([]byte, error) {
	c.conn.SetReadDeadline(time.Now().Add(readTimeout))
	// First read the size
	var total Size
	if err := binary.Read(c.conn, globalOrder, &total); err != nil {
		return nil, handleError(err)
	}

	b := make([]byte, total)
	var read Size
	var buffer bytes.Buffer
	for read < total {
		// Read the size of the next packet.
		c.conn.SetReadDeadline(time.Now().Add(readTimeout))
		n, err := c.conn.Read(b)
		// Quit if there is an error.
		if err != nil {
			return nil, handleError(err)
		}
		// Append the read bytes into the buffer.
		if _, err := buffer.Write(b[:n]); err != nil {
			log.Error("Couldn't write to buffer:", err)
		}
		read += Size(n)
		b = b[n:]
	}
	return b
}

// Router holds all incoming and outgoing alive connections, permits application
// layer above to send and receive messages with each connections mapped to a
// public identity.
type Router struct {
	priv     *Private
	list     Publics
	index    int
	addr     string
	pubGroup kyber.Group

	conns   map[string]Conn
	connMut sync.Mutex
}

func NewRouter(priv *Private, list Publics, idx int, pubGroup kyber.Group) *Router {
	return &Router{
		priv:  priv,
		index: idx,
		list:  list,
		addr:  list[idx].Address,
		conns: make(map[string]Conn),
	}
}

func (r *Router) Listen() {
	listener, err := net.Listen("tcp", r.addr)
	if err != nil {
		panic("can't listen on addresse: " + err.Error())
	}

	for {
		c, err := listener.Accept()
		if err != nil {
			slog.Info("error with listening: ", err)
		}
		go r.handleIncoming(c)
	}
}

// handleIncoming expects to receive the public identity of the remote party
// first, then handle the connection normally as in handleConn.
func (r *Router) handleIncoming(c Conn) {
	buff, err := c.Receive()
	if err != nil {
		slog.Debug("router: error receiving from ", c.RemoteAddr())
		return
	}

	drand, err := unmarshal(r.pubGroup, buff)
	if err != nil {
		slog.Debug("router: error unmarshalling pub key from", c.RemoteAddr())
		return
	}

	if drand.Hello == nil {
		slog.Debug("router: no hello message from", c.RemoteAddr())
		return
	}
	pub := drand.Hello
	// chekc that we know this public key. Not a security measure but merely to
	// only deal with keys this router knows
	if !r.list.Contains(pub) {
		slog.Debug("router: unknown public key from ", c.RemoteAddr())
		return
	}

	r.connMut.Lock()
	if _, ok := r.conns[pub.Address]; ok {
		slog.Debug("router: already connected to ", pub.Address)
		r.connMut.Unlock()
		return
	}

	c.conns[pub.Address] = c
	r.connMut.Unlock()
	r.handleConnection(pub, c)
}

func (r *Router) handleConnection(p *Public, c Conn) {

}

func host(c net.Conn) string {
	host, _, _ := net.SplitHostPort(c.RemoteAddr().Network())
	return host
}

package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"time"

	kyber "gopkg.in/dedis/kyber.v1"

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
	return b, nil
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

	messages chan messageWrapper
}

func NewRouter(priv *Private, list Publics, idx int, pubGroup kyber.Group) *Router {
	return &Router{
		priv:     priv,
		index:    idx,
		list:     list,
		addr:     list[idx].Address,
		conns:    make(map[string]Conn),
		messages: make(chan messageWrapper),
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
		go r.handleIncoming(Conn{c})
	}
}

func (r *Router) Receive() (*Public, []byte) {
	wrap := <-r.messages
	return wrap.Pub, wrap.Message
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

	r.conns[pub.Address] = c
	r.connMut.Unlock()
	r.handleConnection(pub, c)
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

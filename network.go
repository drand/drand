package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"reflect"
	"time"

	kyber "gopkg.in/dedis/kyber.v1"

	"github.com/dedis/onet/log"
	"github.com/dedis/protobuf"
	"github.com/nikkolasg/slog"
)

// a connection will return an io.EOF after readTimeout if nothing has been
// sent.
var readTimeout = 1 * time.Minute

// Router holds all incoming and outgoing alive connections, permits application
// layer above to send and receive messages with each connections mapped to a
// public identity.
type Router struct {
	priv  *Private
	index int
	list  Publics
	addr  string
}

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

func (c *Conn) Receive(g kyber.Group) (*Drand, error) {
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

}

func NewRouter(priv *Private, list Publics) (*Router, error) {
	var idx = -1
	for i, p := range list {
		if !priv.Public.Equal(p) {
			continue
		}
		idx = i
	}
	if idx == -1 {
		return nil, errors.New("public identity not found in the list")
	}
	return &Router{
		priv:  priv,
		index: idx,
		list:  list,
		addr:  list[index].Address,
	}, nil
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
func (r *Router) handleIncoming(c net.Conn) {

}

func host(c net.Conn) string {
	host, _, _ := net.SplitHostPort(c.RemoteAddr().Network())
	return host
}

func constructors(g kyber.Group) protobuf.Constructors {
	cons := make(protobuf.Constructors)
	var s kyber.Scalar
	var p kyber.Point
	cons[reflect.TypeOf(&s).Elem()] = func() interface{} { return g.Scalar() }
	cons[reflect.TypeOf(&p).Elem()] = func() interface{} { return g.Point() }
	return cons
}

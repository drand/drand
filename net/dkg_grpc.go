package net

import (
	"crypto/tls"
	"fmt"

	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func NewDKGClient(addr string, useTLS bool) (drand.DKGClient, error) {
	conn, err := grpcConnection(addr, useTLS)
	if err != nil {
		return nil, err
	}

	return drand.NewDKGClient(conn), nil
}

func NewDKGControlClient(addr string) (drand.DKGControlClient, error) {
	conn, err := grpcConnection(addr, false)
	if err != nil {
		return nil, err
	}

	return drand.NewDKGControlClient(conn), nil
}

func grpcConnection(addr string, useTLS bool) (*grpc.ClientConn, error) {
	var conn *grpc.ClientConn
	network, host := listenAddrFor(addr)
	if network != grpcDefaultIPNetwork {
		host = fmt.Sprintf("%s://%s", network, host)
	}

	var err error
	if useTLS {
		// the intention in the medium term is to remove TLS negotiation from drand...
		// so let's skip verification here >.>
		//golint:gosec
		tlsCredentials := credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		})
		conn, err = grpc.Dial(host, grpc.WithTransportCredentials(tlsCredentials))
	} else {
		conn, err = grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if err != nil {
		log.DefaultLogger().Errorw("", "DKG client", "connect failure", "err", err)
		return nil, err
	}
	return conn, nil
}

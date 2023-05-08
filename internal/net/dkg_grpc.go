package net

import (
	"fmt"

	"github.com/drand/drand/common/log"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewDKGControlClient(l log.Logger, addr string) (drand.DKGControlClient, error) {
	conn, err := grpcConnection(l, addr)
	if err != nil {
		return nil, err
	}

	return drand.NewDKGControlClient(conn), nil
}

func grpcConnection(l log.Logger, addr string) (*grpc.ClientConn, error) {
	network, host := listenAddrFor(addr)
	if network != grpcDefaultIPNetwork {
		host = fmt.Sprintf("%s://%s", network, host)
	}

	conn, err := grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		l.Errorw("", "DKG client", "connect failure", "err", err)
		return nil, err
	}
	return conn, nil
}

package net

import (
	"fmt"
	"github.com/drand/drand/log"
	"github.com/drand/drand/protobuf/drand"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewDKGClient(addr string) (drand.DKGClient, error) {
	conn, err := grpcConnection(addr)
	if err != nil {
		return nil, err
	}

	return drand.NewDKGClient(conn), nil
}

func NewDKGControlClient(addr string) (drand.DKGControlClient, error) {
	conn, err := grpcConnection(addr)
	if err != nil {
		return nil, err
	}

	return drand.NewDKGControlClient(conn), nil
}

func grpcConnection(addr string) (*grpc.ClientConn, error) {
	var conn *grpc.ClientConn
	network, host := listenAddrFor(addr)
	if network != grpcDefaultIPNetwork {
		host = fmt.Sprintf("%s://%s", network, host)
	}

	conn, err := grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.DefaultLogger().Errorw("", "DKG client", "connect failure", "err", err)
		return nil, err
	}
	return conn, nil
}

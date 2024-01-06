package net

import (
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pdkg "github.com/drand/drand/protobuf/dkg"

	"github.com/drand/drand/common/log"
)

func NewDKGControlClient(l log.Logger, addr string) (pdkg.DKGControlClient, error) {
	conn, err := grpcConnection(l, addr)
	if err != nil {
		return nil, err
	}

	return pdkg.NewDKGControlClient(conn), nil
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

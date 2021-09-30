package drand

import (
	"context"
	"google.golang.org/grpc"
)

type Interceptors interface {
	NodeVersionValidator(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response interface{}, err error)
	NodeVersionStreamValidator(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error
}

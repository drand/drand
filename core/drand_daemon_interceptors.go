package core

import (
	"context"

	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/protobuf/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MetadataGetter interface {
	GetMetadata() *common.Metadata
}

func (dd *DrandDaemon) NodeVersionValidator(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response interface{}, err error) {
	reqWithContext, ok := req.(MetadataGetter)

	if !ok {
		return handler(ctx, req)
	}

	metadata := reqWithContext.GetMetadata()
	if metadata == nil {
		return handler(ctx, req)
	}

	v := metadata.GetNodeVersion()
	if v == nil {
		return handler(ctx, req)
	}

	rcvVer := commonutils.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
	if !d.version.IsCompatible(rcvVer) {
		d.log.Warnw("", "node_version_interceptor", "node version rcv is no compatible --> rejecting message", "version", rcvVer)
		return nil, status.Error(codes.PermissionDenied, "Node Version not valid")
	}

	return handler(ctx, req)
}

func (dd *DrandDaemon) NodeVersionStreamValidator(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	reqWithContext, ok := srv.(MetadataGetter)

	if !ok {
		return handler(srv, ss)
	}

	metadata := reqWithContext.GetMetadata()
	if metadata == nil {
		return handler(srv, ss)
	}

	v := metadata.GetNodeVersion()
	if v == nil {
		return handler(srv, ss)
	}

	rcvVer := commonutils.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
	if !d.version.IsCompatible(rcvVer) {
		d.log.Warnw("", "node_version_interceptor", "node version rcv is no compatible --> rejecting message", "version", rcvVer)
		return status.Error(codes.PermissionDenied, "Node Version not valid")
	}

	return handler(srv, ss)
}

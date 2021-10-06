package core

import (
	"context"

	"github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MetadataGetter interface {
	GetMetadata() *common.Metadata
}

func (d *Drand) NodeVersionValidator(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response interface{}, err error) {
	reqWithContext, ok := req.(MetadataGetter)

	//d.log.Debugw("", "node_version_interceptor", fmt.Sprintf("request type --> %T", req))
	//d.log.Debugw("", "node_version_interceptor", fmt.Sprintf("GetMetadata method is present --> %t", ok))
	if !ok {
		return handler(ctx, req)
	}

	metadata := reqWithContext.GetMetadata()
	if metadata == nil {
		return handler(ctx, req)
	}
	//d.log.Debugw("", "node_version_interceptor", "metadata is present")

	v := metadata.GetNodeVersion()
	if v == nil {
		return handler(ctx, req)
	}
	//d.log.Debugw("", "node_version_interceptor", "version is present")

	rcvVer := utils.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
	if !d.version.IsCompatible(rcvVer) {
		d.log.Warn("node_version_interceptor", "node version rcv is no compatible --> rejecting message", "version", rcvVer)
		return nil, status.Error(codes.PermissionDenied, "Node Version not valid")
	}

	//d.log.Debugw("", "node_version_interceptor", "version rcv is compatible with our node version")
	return handler(ctx, req)
}

func (d *Drand) NodeVersionStreamValidator(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	reqWithContext, ok := srv.(MetadataGetter)

	//d.log.Debugw("", "node_version_interceptor", fmt.Sprintf("request type --> %T", srv))
	//d.log.Debugw("", "node_version_interceptor", fmt.Sprintf("GetMetadata method is present --> %t", ok))
	if !ok {
		return handler(srv, ss)
	}

	metadata := reqWithContext.GetMetadata()
	if metadata == nil {
		return handler(srv, ss)
	}
	//d.log.Debugw("", "node_version_interceptor", "metadata is present")

	v := metadata.GetNodeVersion()
	if v == nil {
		return handler(srv, ss)
	}
	//d.log.Debugw("", "node_version_interceptor", "version field is present")

	rcvVer := utils.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
	if !d.version.IsCompatible(rcvVer) {
		d.log.Warn("node_version_interceptor", "node version rcv is no compatible --> rejecting message", "version", rcvVer)
		return status.Error(codes.PermissionDenied, "Node Version not valid")
	}

	//d.log.Debugw("", "node_version_interceptor", "version rcv is compatible with our node version")
	return handler(srv, ss)
}

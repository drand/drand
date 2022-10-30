package core

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonutils "github.com/drand/drand/common"
	"github.com/drand/drand/metrics"
	"github.com/drand/drand/protobuf/common"
)

type MetadataGetter interface {
	GetMetadata() *common.Metadata
}

func (dd *DrandDaemon) NodeVersionValidator(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response interface{}, err error) {
	ctx, span := metrics.NewSpan(ctx, "dd.NodeVersionValidator")
	defer span.End()

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

	prerelease := ""
	if v.Prerelease != nil {
		prerelease = *v.Prerelease
	}
	rcvVer := commonutils.Version{
		Major:      v.Major,
		Minor:      v.Minor,
		Patch:      v.Patch,
		Prerelease: prerelease,
	}
	if !dd.version.IsCompatible(rcvVer) {
		dd.log.Named("node_version_interceptor").Warnw(
			"node version rcv is no compatible --> rejecting message",
			"received", rcvVer,
			"our node", dd.version)
		msg := fmt.Sprintf("Incompatible node version. Current: %v, received: %v", dd.version, rcvVer)
		return nil, status.Error(codes.PermissionDenied, msg)
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

	prerelease := ""
	if v.Prerelease != nil {
		prerelease = *v.Prerelease
	}
	rcvVer := commonutils.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch, Prerelease: prerelease}
	if !dd.version.IsCompatible(rcvVer) {
		dd.log.Warnw("", "node_version_interceptor", "node version rcv is no compatible --> rejecting message", "version", rcvVer)
		msg := fmt.Sprintf("Incompatible node version. Current: %v, received: %v", dd.version, rcvVer)
		return status.Error(codes.PermissionDenied, msg)
	}

	return handler(srv, ss)
}

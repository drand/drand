/*
Package protobuf contains wire definitions of messages passed between drand nodes.
*/
//go:generate protoc -I=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=require_unimplemented_servers=false,paths=source_relative:. drand/api.proto drand/common.proto drand/control.proto drand/protocol.proto drand/metrics.proto
//go:generate protoc -I=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=require_unimplemented_servers=false,paths=source_relative:. dkg/dkg.proto dkg/dkg_control.proto
package protobuf

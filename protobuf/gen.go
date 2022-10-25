/*
Package protobuf contains wire definitions of messages passed between drand nodes.
*/
//go:generate protoc -I=. --go_out=. --go_opt=paths=source_relative common/common.proto
//go:generate protoc -I=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=require_unimplemented_servers=false,paths=source_relative:. drand/api.proto drand/common.proto drand/control.proto drand/protocol.proto
//go:generate protoc -I=. --go_out=. --go_opt=paths=source_relative crypto/dkg/dkg.proto
package protobuf

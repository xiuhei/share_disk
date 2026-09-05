// Package sharediskv1 contains generated Protocol Buffers code for the P2P
// transfer protocol and the local IPC protocol.
//
// Regenerate with:
//
//	protoc --proto_path=contracts/proto --go_out=. --go_opt=module=github.com/share-disk/share-disk contracts/proto/sharedisk/v1/transfer.proto contracts/proto/sharedisk/v1/local.proto
package sharediskv1

//go:generate protoc --proto_path=contracts/proto --go_out=. --go_opt=module=github.com/share-disk/share-disk contracts/proto/sharedisk/v1/transfer.proto contracts/proto/sharedisk/v1/local.proto

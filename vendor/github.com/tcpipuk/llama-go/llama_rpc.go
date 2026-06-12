//go:build rpc
// +build rpc

// This file provides Remote Procedure Call (RPC) acceleration support when built
// with the 'rpc' build tag. It enables offloading computation to remote servers
// for distributed inference across heterogeneous clusters.
//
// Build with: BUILD_TYPE=rpc make libbinding.a
//
// Requires RPC server setup on remote machines. The RPC backend enables
// distributed inference, allowing workloads to be offloaded to remote GPUs or
// split across multiple machines. See llama.cpp RPC documentation for server
// configuration.
//
// CGO flags required:
//
//	-lpthread
package llama

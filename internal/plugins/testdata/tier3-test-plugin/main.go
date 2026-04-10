// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// tier3-test-plugin is a minimal Tier 3 plugin for integration tests.
// It echoes payloads back on Execute and exits cleanly on Shutdown.
package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"google.golang.org/grpc"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
)

type pluginServer struct {
	siplyv1.UnimplementedSiplyPluginServiceServer
}

func (s *pluginServer) Initialize(_ context.Context, req *siplyv1.InitializeRequest) (*siplyv1.InitializeResponse, error) {
	return &siplyv1.InitializeResponse{Success: true}, nil
}

func (s *pluginServer) Execute(_ context.Context, req *siplyv1.ExecuteRequest) (*siplyv1.ExecuteResponse, error) {
	return &siplyv1.ExecuteResponse{
		Success: true,
		Result:  req.GetPayload(),
	}, nil
}

func (s *pluginServer) Shutdown(_ context.Context, _ *siplyv1.ShutdownRequest) (*siplyv1.ShutdownResponse, error) {
	go func() { os.Exit(0) }()
	return &siplyv1.ShutdownResponse{}, nil
}

func main() {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}

	srv := grpc.NewServer()
	siplyv1.RegisterSiplyPluginServiceServer(srv, &pluginServer{})

	// Write address to stdout for core to read.
	fmt.Printf("PLUGIN_ADDR=%s\n", lis.Addr().String())

	if err := srv.Serve(lis); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		os.Exit(1)
	}
}

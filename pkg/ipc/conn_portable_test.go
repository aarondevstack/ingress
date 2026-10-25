// Copyright (c) 2026 aarondevstack
// SPDX-License-Identifier: Apache-2.0

package ipc

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type portableService struct {
	UnimplementedIngressServiceServer
}

func (portableService) UpdateIngressState(context.Context, *UpdateIngressStateRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

type portableHandler struct {
	UnimplementedIngressHandlerServer
}

func (portableHandler) GetPipelineDot(context.Context, *GstPipelineDebugDotRequest) (*GstPipelineDebugDotResponse, error) {
	return &GstPipelineDebugDotResponse{DotFile: "portable IPC"}, nil
}

func TestPortableIPCClients(t *testing.T) {
	// Short OS temp paths fit AF_UNIX limits; Windows retains its real drive letter.
	dir, err := os.MkdirTemp("", "ipc space ")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	for _, name := range []string{"service_ipc.sock", "handler_ipc.sock"} {
		listener, err := net.Listen("unix", filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		server := grpc.NewServer()
		RegisterIngressServiceServer(server, portableService{})
		RegisterIngressHandlerServer(server, portableHandler{})
		t.Cleanup(server.Stop)
		go func() {
			if err := server.Serve(listener); err != nil {
				t.Errorf("serve IPC: %v", err)
			}
		}()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := GetServiceClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The generated client keeps its connection private; close it through the package type.
	defer client.(*ingressServiceClient).cc.(*grpc.ClientConn).Close()
	if _, err := client.UpdateIngressState(ctx, &UpdateIngressStateRequest{}); err != nil {
		t.Fatal(err)
	}
	handler, err := GetHandlerClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	response, err := handler.GetPipelineDot(ctx, &GstPipelineDebugDotRequest{})
	if err != nil || response.GetDotFile() != "portable IPC" {
		t.Fatalf("handler RPC: %v, %v", response, err)
	}
}

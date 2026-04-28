package grpcutil

import (
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Dial(addr string) *grpc.ClientConn {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4*1024*1024)),
	)
	if err != nil {
		log.Fatalf("gRPC dial %s failed: %v", addr, err)
	}
	return conn
}

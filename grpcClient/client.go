package grpcClient

import (
	"context"
	"fmt"
	"time"

	"github.com/fcwrsmall/tron-wallet/enums"
	"github.com/fcwrsmall/tron-wallet/grpcClient/proto/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GrpcClient controller structure
type GrpcClient struct {
	Address     string
	Conn        *grpc.ClientConn
	Client      api.WalletClient
	grpcTimeout time.Duration
	opts        []grpc.DialOption
	apiKey      string
}

var GrpcClientOne *GrpcClient

func GetGrpcClient(node enums.Node) (*GrpcClient, error) {
	if GrpcClientOne != nil {
		return GrpcClientOne, nil
	}
	c := &GrpcClient{
		Address:     string(node),
		grpcTimeout: 5 * time.Second,
	}

	err := c.Start(grpc.WithTransportCredentials(insecure.NewCredentials()))
	GrpcClientOne = c
	return c, err
}

func GetGrpcClientByApikey(node enums.Node, apiKey string) (*GrpcClient, error) {
	if GrpcClientOne != nil {
		return GrpcClientOne, nil
	}
	c := &GrpcClient{
		Address:     string(node),
		grpcTimeout: 5 * time.Second,
		apiKey:      apiKey,
	}

	err := c.Start(grpc.WithTransportCredentials(insecure.NewCredentials()))
	GrpcClientOne = c
	return c, err
}

func (g *GrpcClient) Start(opts ...grpc.DialOption) error {
	var err error
	if len(g.Address) == 0 {
		g.Address = "grpc.trongrid.io:50051"
	}
	g.opts = opts
	g.Conn, err = grpc.Dial(g.Address, opts...)

	if err != nil {
		return fmt.Errorf("Connecting GRPC Client: %v", err)
	}
	g.Client = api.NewWalletClient(g.Conn)
	return nil
}

func (g *GrpcClient) getContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), g.grpcTimeout)
	if len(g.apiKey) > 0 {
		ctx = metadata.AppendToOutgoingContext(ctx, "TRON-PRO-API-KEY", g.apiKey)
	}
	return ctx, cancel
}

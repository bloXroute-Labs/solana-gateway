package gateway

import (
	"context"

	proto "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
)

type Registrar interface {
	Register() error
}

type bdnRegistrar struct {
	ctx     context.Context
	client  proto.RelayClient
	header  string
	version string
}

func NewBDNRegistrar(ctx context.Context, client proto.RelayClient, header, version string) Registrar {
	return &bdnRegistrar{
		ctx:     ctx,
		client:  client,
		header:  header,
		version: version,
	}
}

func (r *bdnRegistrar) Register() error {
	_, err := r.client.Register(r.ctx, &proto.RegisterRequest{
		AuthHeader: r.header,
		Version:    r.version,
	})

	return err
}

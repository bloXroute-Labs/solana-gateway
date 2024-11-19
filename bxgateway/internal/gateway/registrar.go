package gateway

import (
	"context"

	proto "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
)

type Registrar interface {
	Register() (string, error)
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

// Register calls register endpoint on relay and returns relay's udp address to send and recv shreds
func (r *bdnRegistrar) Register() (string, error) {
	rsp, err := r.client.Register(r.ctx, &proto.RegisterRequest{
		AuthHeader: r.header,
		Version:    r.version,
	})

	if err != nil {
		return "", err
	}

	return rsp.GetUdpAddress(), nil
}

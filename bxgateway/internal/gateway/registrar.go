package gateway

import (
	"context"
	"fmt"

	"github.com/bloXroute-Labs/solana-gateway/pkg/config"
	proto "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
)

type Registrar interface {
	Register() (*proto.RegisterResponse, error)
}

type ofrRegistrar struct {
	ctx    context.Context
	client proto.RelayClient
	cfg    *config.Gateway
}

func NewOFRRegistrar(ctx context.Context, client proto.RelayClient, cfg *config.Gateway) Registrar {
	return &ofrRegistrar{
		ctx:    ctx,
		client: client,
		cfg:    cfg,
	}
}

// Register calls register endpoint on relay and returns relay's udp address to send and recv shreds
func (r *ofrRegistrar) Register() (*proto.RegisterResponse, error) {
	bz, err := r.cfg.Marshal()
	if err != nil {
		return nil, fmt.Errorf("ofrRegistrar.Register(): %v", err)
	}

	return r.client.Register(r.ctx, &proto.RegisterRequest{
		AuthHeader:           r.cfg.AuthHeader,
		GatewayConfiguration: bz,
	})
}

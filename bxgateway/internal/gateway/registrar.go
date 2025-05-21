package gateway

import (
	"context"
	"fmt"

	"github.com/bloXroute-Labs/solana-gateway/pkg/config"
	proto "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
)

type Registrar interface {
	Register() (*proto.RegisterResponse, error)
	RefreshToken(token string) (*proto.RefreshTokenResponse, error)
}

type ofrRegistrar struct {
	ctx        context.Context
	client     proto.RelayClient
	cfg        *config.Gateway
	version    string
	serverPort int64
}

func NewOFRRegistrar(ctx context.Context, client proto.RelayClient, cfg *config.Gateway, version string, serverPort int64) Registrar {
	return &ofrRegistrar{
		ctx:        ctx,
		client:     client,
		cfg:        cfg,
		version:    version,
		serverPort: serverPort,
	}
}

// Register calls register endpoint on relay and returns relay's udp address to send and recv shreds
func (r *ofrRegistrar) Register() (*proto.RegisterResponse, error) {
	bz, err := r.cfg.Marshal()
	if err != nil {
		return nil, fmt.Errorf("ofrRegistrar.Register(): %v", err)
	}
	rsp, err := r.client.Register(r.ctx, &proto.RegisterRequest{
		AuthHeader:           r.cfg.AuthHeader,
		Version:              r.version,
		ServerPort:           r.serverPort,
		GatewayConfiguration: bz,
	})
	if err != nil {
		return nil, err
	}

	return rsp, nil
}

func (r *ofrRegistrar) RefreshToken(token string) (*proto.RefreshTokenResponse, error) {
	rsp, err := r.client.RefreshToken(r.ctx, &proto.RefreshTokenRequest{
		AuthHeader: r.cfg.AuthHeader,
		JwtToken:   token,
	})
	if err != nil {
		return nil, err
	}

	return rsp, nil
}

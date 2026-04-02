package gateway

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/bloXroute-Labs/solana-gateway/pkg/config"
	proto "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
)

var (
	// ErrInvalidConfig is returned when the config provided to register endpoint is invalid
	ErrInvalidConfig = errors.New("invalid config")
	// ErrUnathorized is returned when the auth header provided to register endpoint is invalid or missing
	ErrUnathorized = errors.New("unauthorized")
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

	resp, err := r.client.Register(r.ctx, &proto.RegisterRequest{
		AuthHeader:           r.cfg.AuthHeader,
		GatewayConfiguration: bz,
	})
	if err != nil {
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.InvalidArgument:
				return nil, fmt.Errorf("%s: %w", s.Message(), ErrInvalidConfig)
			case codes.Unauthenticated, codes.PermissionDenied:
				return nil, fmt.Errorf("%s: %w", s.Message(), ErrUnathorized)
			default:
				return nil, fmt.Errorf("ofrRegistrar.Register(): %s", s.Message())
			}
		}

		return nil, fmt.Errorf("ofrRegistrar.Register(): %v", err)
	}

	return resp, nil

}

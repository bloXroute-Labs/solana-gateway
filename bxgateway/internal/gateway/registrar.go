package gateway

import (
	"context"

	proto "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
)

type Registrar interface {
	Register() (string, error)
}

type ofrRegistrar struct {
	ctx        context.Context
	client     proto.RelayClient
	header     string
	version    string
	serverPort int64
}

func NewOFRRegistrar(ctx context.Context, client proto.RelayClient, header, version string, serverPort int64) Registrar {
	return &ofrRegistrar{
		ctx:        ctx,
		client:     client,
		header:     header,
		version:    version,
		serverPort: serverPort,
	}
}

// Register calls register endpoint on relay and returns relay's udp address to send and recv shreds
func (r *ofrRegistrar) Register() (string, error) {
	rsp, err := r.client.Register(r.ctx, &proto.RegisterRequest{
		AuthHeader: r.header,
		Version:    r.version,
		ServerPort: r.serverPort,
	})

	if err != nil {
		return "", err
	}

	return rsp.GetUdpAddress(), nil
}

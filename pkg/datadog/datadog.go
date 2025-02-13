package datadog

import (
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
)

// Client is an interface for sending metrics
type Client interface {
	Gauge(name string, value float64)
}

// NoopManager is a no-op implementation of the Client interface
type NoopManager struct{}

func (m *NoopManager) Gauge(string, float64) {}

// Manager handles sending metrics to DataDog
type Manager struct {
	client    *statsd.Client
	namespace string
	lg        logger.Logger
}

// NewManager creates a new DataDog stats manager
func NewManager(endpoint, namespace string, lg logger.Logger) (*Manager, error) {
	client, err := statsd.New(endpoint,
		statsd.WithNamespace(namespace),
		statsd.WithErrorHandler(func(err error) {
			lg.Errorf("DataDog client error: %s", err)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create DataDog client: %w", err)
	}

	lg.Infof("DataDog client initialized with endpoint %s and namespace %s", endpoint, namespace)

	return &Manager{
		client:    client,
		namespace: namespace,
		lg:        lg,
	}, nil
}

// Gauge sends a gauge metric to DataDog
func (d *Manager) Gauge(name string, value float64) {
	err := d.client.Gauge(name, value, nil, 1)
	if err != nil {
		d.lg.Errorf("failed to send metric %s to DataDog: %s", name, err)
	} else {
		d.lg.Tracef("sent metric %s to DataDog", name)
	}
}

package txfwd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	proto "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
)

const (
	protocolJSONRPC = "jsonrpc"
	tokenHeader     = "token"
)

type BxForwarder struct {
	lg         logger.Logger
	ctx        context.Context
	mx         *sync.RWMutex
	forwarders []BxForwarderSubmitter
	jwtToken   string
}

type BxForwarderSubmitter interface {
	Forward(rawTxBase64 string) error
	Addr() string
	Close() error
}

func NewBxForwarder(ctx context.Context, lg logger.Logger) *BxForwarder {
	return &BxForwarder{
		lg:         lg,
		ctx:        ctx,
		mx:         &sync.RWMutex{},
		forwarders: []BxForwarderSubmitter{},
	}
}

func (t *BxForwarder) Token() string {
	t.mx.RLock()
	defer t.mx.RUnlock()
	return t.jwtToken
}

func (t *BxForwarder) UpdateToken(jwtToken string) {
	t.mx.Lock()
	defer t.mx.Unlock()
	t.jwtToken = jwtToken
}

// Update updates the forwarders list with new forwarders,
// removes and closes idle connections for forwarders that are not in the new list and adds new ones
func (t *BxForwarder) Update(jwtToken string, txPropagationConfig *proto.TxPropagationConfig) {
	if txPropagationConfig.GetTxForwarders() == nil {
		return
	}

	t.UpdateToken(jwtToken)

	var newForwarders = make([]BxForwarderSubmitter, 0, len(txPropagationConfig.TxForwarders))
	for _, newFwd := range txPropagationConfig.TxForwarders {
		bxfwd, err := NewBxForwarderSubmitter(t.ctx, t.lg, t.jwtToken, newFwd)
		if err != nil {
			t.lg.Errorf("failed to create new bx forwarder: %s", err)
			continue
		}

		newForwarders = append(newForwarders, bxfwd)
	}

	t.mx.Lock()
	for _, fwd := range t.forwarders {
		fwd.Close()
	}

	t.forwarders = newForwarders
	t.mx.Unlock()

	t.lg.Infof("updated BxForwarder with %d forwarders", len(newForwarders))
}

func (t *BxForwarder) Forward(rawTxBase64 string) {
	t.mx.RLock()
	defer t.mx.RUnlock()

	if len(t.forwarders) == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(t.forwarders))

	for _, txfwd := range t.forwarders {
		go func(txfwd BxForwarderSubmitter) {
			defer wg.Done()
			err := txfwd.Forward(rawTxBase64)
			if err != nil {
				t.lg.Errorf("failed to submit tx %s to %s: %s", rawTxBase64, txfwd.Addr(), err)
			}
		}(txfwd)
	}

	// wait for all forwarders to finish
	// this is needed because we want to hold the lock until Submit() is done due to in case of Update() we close all connections
	// note: since it's a read lock we will still submit txs concurrently when Submit() is called in different goroutines
	wg.Wait()

	t.lg.Debugf("submitted tx %s to %d forwarders", rawTxBase64, len(t.forwarders))
}

func NewBxForwarderSubmitter(ctx context.Context, lg logger.Logger, jwtToken string, txForwarder *proto.TxForwarder) (BxForwarderSubmitter, error) {
	// for now jsonrpc is the only supported protocol
	// when we add support for other protocols we will need to range over the list of supported protocols
	// and select the appropriate one
	switch {
	case slices.Contains(txForwarder.GetSupportedProtocols(), protocolJSONRPC):
		return newJSONRPCBxForwarder(ctx, lg, jwtToken, txForwarder)
	default:
		return nil, fmt.Errorf("no supported protocols: %s", txForwarder.GetSupportedProtocols())
	}
}

type jsonRPCBxForwarder struct {
	lg         logger.Logger
	ctx        context.Context
	jwtToken   string
	addr       string
	httpClient *http.Client
}

func newJSONRPCBxForwarder(ctx context.Context, lg logger.Logger, jwtToken string, txForwarder *proto.TxForwarder) (*jsonRPCBxForwarder, error) {
	var bxfwd = &jsonRPCBxForwarder{
		lg:         lg,
		ctx:        ctx,
		jwtToken:   jwtToken,
		addr:       txForwarder.GetAddr(),
		httpClient: newHTTPClient(),
	}

	return bxfwd, nil
}

const jsonRPCURLFormat = "http://%s:%d"
const jsonRPCBody = `{"jsonrpc":"2.0","method":"sendTransaction","params":["%s"],"id":1}`
const jsonPRCPort = 5055

func (t *jsonRPCBxForwarder) Forward(rawTxBase64 string) error {
	rq, err := http.NewRequest(http.MethodPost, fmt.Sprintf(jsonRPCURLFormat, t.addr, jsonPRCPort), bytes.NewBufferString(fmt.Sprintf(jsonRPCBody, rawTxBase64)))
	if err != nil {
		return fmt.Errorf("failed to create request: %s", err)
	}

	rq.Header.Set("Authorization", "Bearer "+t.jwtToken)

	rsp, err := t.httpClient.Do(rq)
	if err != nil {
		return fmt.Errorf("failed to send request: %s", err)
	}

	defer rsp.Body.Close()

	_, err = io.Copy(io.Discard, rsp.Body)
	if err != nil {
		return fmt.Errorf("failed to discard response body: %s", err)
	}

	return nil
}

func (t *jsonRPCBxForwarder) Addr() string { return t.addr }

func (t *jsonRPCBxForwarder) Close() error {
	t.httpClient.CloseIdleConnections()
	return nil
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			MaxConnsPerHost:     100,
			IdleConnTimeout:     0,
			DisableCompression:  false,
			DisableKeepAlives:   false,
			ForceAttemptHTTP2:   false,
		},
	}
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  []string        `json:"params"`
}

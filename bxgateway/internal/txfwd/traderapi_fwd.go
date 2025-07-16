package txfwd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/ofr"
	proto "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
)

type TraderAPIForwarder struct {
	lg         logger.Logger
	mx         *sync.RWMutex
	forwarders []*traderAPIForwarder
	nParallel  int
	authHeader string
}

func NewTraderAPIForwarder(lg logger.Logger, nParallel int, auth string) *TraderAPIForwarder {
	return &TraderAPIForwarder{
		lg:         lg,
		mx:         &sync.RWMutex{},
		forwarders: []*traderAPIForwarder{},
		nParallel:  nParallel,
		authHeader: auth,
	}
}

func (f *TraderAPIForwarder) Update(txPropagationConfig *proto.TxPropagationConfig) {
	if txPropagationConfig.GetTraderApis() == nil {
		return
	}

	traderAPIs := txPropagationConfig.GetTraderApis()
	newForwarders := make([]*traderAPIForwarder, 0, len(traderAPIs))
	var wg sync.WaitGroup
	wg.Add(len(traderAPIs))

	for _, newTraderAPIAddr := range traderAPIs {
		newForwarders = append(newForwarders, newTraderAPIForwarder(f.lg, newTraderAPIAddr, &wg))
	}

	wg.Wait()

	// sort forwarders by ping (nil ping values at the end)
	sort.Slice(newForwarders, func(i, j int) bool {
		if newForwarders[i].ping == nil {
			return false
		}
		if newForwarders[j].ping == nil {
			return true
		}
		return *newForwarders[i].ping < *newForwarders[j].ping
	})

	f.mx.Lock()
	for _, fwd := range f.forwarders {
		fwd.close()
	}

	f.forwarders = newForwarders
	f.nParallel = int(txPropagationConfig.GetNumTraderApisParallel())
	f.mx.Unlock()

	f.lg.Infof("updated TraderAPIForwarder with %d forwarders, parallel use: %d", len(newForwarders), f.nParallel)
}

func (f *TraderAPIForwarder) Forward(rawBody []byte) {
	if len(f.forwarders) == 0 {
		return
	}

	f.mx.RLock()
	defer f.mx.RUnlock()

	for _, forwarder := range f.forwarders[:f.nParallel] {
		go func(forwarder *traderAPIForwarder) {
			err := forwarder.forward(rawBody, f.authHeader)
			if err != nil {
				if forwarder.successRate() < 0.5 {
					f.lg.Errorf("failed to forward tx %s to TraderAPI %s: %v", string(rawBody), forwarder.url, err)
				}
			} else {
				f.lg.Tracef("Submitted tx %s to TraderAPI %s", string(rawBody), forwarder.url)
			}
		}(forwarder)
	}
}

type traderAPIForwarder struct {
	ctx    context.Context
	cancel context.CancelFunc

	lg         logger.Logger
	url        string
	httpClient *http.Client
	ping       *time.Duration

	successCounter *ofr.SuccessCounter
}

func newTraderAPIForwarder(lg logger.Logger, taURL string, wg *sync.WaitGroup) *traderAPIForwarder {
	ctx, cancel := context.WithCancel(context.Background())

	successCounter := ofr.NewSuccessCounter(lg, fmt.Sprintf("TraderAPIForwarder: %s", taURL), 10*time.Minute)
	go successCounter.Print(ctx)

	fwd := &traderAPIForwarder{
		ctx:            ctx,
		cancel:         cancel,
		lg:             lg,
		url:            taURL,
		httpClient:     newHTTPClient(),
		successCounter: successCounter,
	}

	go func() {
		defer wg.Done()
		parsedURL, err := url.Parse(taURL)
		if err != nil {
			lg.Warnf("failed to ping traderAPIForwarder: failed to parse URL %s: %v", taURL, err)
			return
		}

		pingAddr := parsedURL.Host

		// if the host is direct IP instead of a domain, we need to ping the IP
		if strings.Contains(parsedURL.Host, ":") {
			host, _, err := net.SplitHostPort(parsedURL.Host)
			if err != nil {
				lg.Warnf("failed to ping traderAPIForwarder: failed to split host and port for %s: %v", taURL, err)
				return
			}
			pingAddr = host
		}

		pinger, err := probing.NewPinger(pingAddr)
		if err != nil {
			lg.Warnf("failed to ping traderAPIForwarder: failed to create pinger for %s (pingAddr: %s): %v", taURL, pingAddr, err)
			return
		}

		pinger.SetPrivileged(true)
		pinger.Count = 3
		pinger.Timeout = 10 * time.Second

		err = pinger.Run() // blocks until finished.
		if err != nil {
			// try to ping without privileged mode
			pinger.SetPrivileged(false)

			errR := pinger.Run() // blocks until finished.
			if errR != nil {
				// log the original error
				lg.Warnf("failed to ping traderAPIForwarder: failed to run pinger for %s (pingAddr: %s): %v", taURL, pingAddr, err)
				return
			}
		}

		stats := pinger.Statistics() // get send/receive/duplicate/rtt stats
		if stats.PacketsRecv > 0 {
			fwd.ping = &stats.AvgRtt
			lg.Debugf("TraderAPIForwarder: ping to Trader API %s = %v ms", pingAddr, fwd.ping.Milliseconds())
		} else {
			lg.Debugf("TraderAPIForwarder: was not able to calculate ping to Trader API %s", pingAddr)
		}
	}()

	return fwd
}

func (f *traderAPIForwarder) successRate() float64 {
	return f.successCounter.SuccessRate()
}

func (f *traderAPIForwarder) close() {
	f.cancel()
	f.httpClient.CloseIdleConnections()
}

func (f *traderAPIForwarder) forward(rawBody []byte, authHeader string) error {
	req, err := http.NewRequest(http.MethodPost, f.url, bytes.NewReader(rawBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", authHeader)

	rsp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send proxy request: %w", err)
	}

	defer rsp.Body.Close()

	responseBody, err := io.ReadAll(rsp.Body)
	if err != nil {
		f.successCounter.RecordFailure()
		return fmt.Errorf("failed to read response from Trader API: %w", err)
	}

	if rsp.StatusCode != http.StatusOK {
		f.successCounter.RecordFailure()
		return fmt.Errorf("invalid response from Trader API: %d (%s), response: %s", rsp.StatusCode, rsp.Status, string(responseBody))
	}

	f.successCounter.RecordSuccess()

	return nil
}

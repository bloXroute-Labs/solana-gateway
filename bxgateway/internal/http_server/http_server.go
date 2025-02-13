package http_server

import (
	"bytes"
	"fmt"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	probing "github.com/prometheus-community/pro-bing"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	PingTimeout        = 2000.0
	traderApiSubmitURL = "https://%s/api/v2/submit"
)

var traderAPIEndpoints = []string{
	"uk.solana.dex.blxrbdn.com",
	"ny.solana.dex.blxrbdn.com",
	"la.solana.dex.blxrbdn.com",
	"tokyo.solana.dex.blxrbdn.com",
	"germany.solana.dex.blxrbdn.com",
}

type traderAPILatency struct {
	dns     string
	latency float64
}

type HTTPServer struct {
	lg            logger.Logger
	client        *http.Client
	port          int
	traderAPIURLs []string
	authHeader    string
}

func NewHTTPServer(lg logger.Logger, port int, numOfTraderAPISToSend int, authHeader string) *HTTPServer {
	httpServer := &HTTPServer{
		lg:         lg,
		port:       port,
		authHeader: authHeader,
		client: &http.Client{
			Timeout: 1 * time.Second,
		},
	}
	fastestTraderAPIUrls := httpServer.FindFastestEndpoints()
	dnsList := make([]string, numOfTraderAPISToSend)
	for i := 0; i < numOfTraderAPISToSend; i++ {
		url := fmt.Sprintf(traderApiSubmitURL, fastestTraderAPIUrls[i].dns)
		dnsList[i] = url
	}
	httpServer.traderAPIURLs = dnsList
	return httpServer
}

func (s *HTTPServer) Start() error {
	http.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		s.handleSubmission(w, r)
	})
	s.lg.Infof("starting http server on port %d", s.port)
	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), nil)
}

func (s *HTTPServer) handleSubmission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.lg.Errorf("Failed to read request body: %v", err)
		return
	}

	var wg sync.WaitGroup
	successChan := make(chan []byte, 1)
	failureChan := make(chan []byte, 1)

	for _, traderAPIUrl := range s.traderAPIURLs {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			// clone the request for each API call
			req, _ := http.NewRequest(r.Method, url, bytes.NewReader(bodyBytes))
			req.Header.Set("Authorization", s.authHeader)

			response, err := s.forwardToTraderAPI(req)
			if err != nil {
				s.lg.Errorf("Failed to forward tx to %v: %v", url, err)

				select {
				case failureChan <- response:
				default:
					// fail already reported
				}
				return
			}
			s.lg.Debugf("submitted tx to %v, response %v", url, string(response))
			select {
			case successChan <- response:
			default:
				// Success already reported
			}
		}(traderAPIUrl)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case response := <-successChan:
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(response); err != nil {
			s.lg.Errorf("Failed to write valid response: %v", err)
		}
	// if done executed meaning no success response so we send error msg
	case <-done:
		var respMsg []byte
		select {
		case respMsg = <-failureChan:
		default: // should never happen
			respMsg = []byte(`{"message":"All requests failed but no details available"}`)
		}
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write(respMsg); err != nil {
			s.lg.Errorf("Failed to write error response: %v", err)
		}
	}
}

func (s *HTTPServer) forwardToTraderAPI(req *http.Request) ([]byte, error) {
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send proxy request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from Trader API: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return responseBody, fmt.Errorf("bad response from Trader API: %d (%s), response: %s", resp.StatusCode, resp.Status, string(responseBody))
	}

	return responseBody, nil
}

func (s *HTTPServer) FindFastestEndpoints() []traderAPILatency {
	var wg sync.WaitGroup
	pingResults := make([]traderAPILatency, len(traderAPIEndpoints))

	wg.Add(len(traderAPIEndpoints))
	for i, url := range traderAPIEndpoints {
		pingResults[i] = traderAPILatency{dns: url, latency: PingTimeout * 1000}
		go func(result *traderAPILatency) {
			defer wg.Done()

			pinger, err := probing.NewPinger(result.dns)
			if err != nil {
				s.lg.Errorf("Failed to create pinger for %v: %v", result.dns, err)
				return
			}

			pinger.Count = 1
			pinger.Timeout = time.Duration(PingTimeout) * time.Second

			err = pinger.Run()
			if err != nil {
				s.lg.Errorf("Ping failed for %v: %v", result.dns, err)
				return
			}

			stats := pinger.Statistics()
			if stats.AvgRtt > 0 {
				result.latency = stats.AvgRtt.Seconds() * 1000 // Convert to milliseconds
			}
		}(&pingResults[i])
	}

	wg.Wait()

	sort.Slice(pingResults, func(i, j int) bool { return pingResults[i].latency < pingResults[j].latency })

	s.lg.Infof("Latency results for potential trader apis: %v", pingResults)
	return pingResults
}

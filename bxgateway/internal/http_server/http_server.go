package http_server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/txfwd"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/gagliardetto/solana-go"
)

type HTTPServer struct {
	lg           logger.Logger
	port         int
	bxFwd        *txfwd.BxForwarder
	traderAPIFwd *txfwd.TraderAPIForwarder
}

func Run(
	lg logger.Logger,
	port int,
	bxFwd *txfwd.BxForwarder,
	traderAPIFwd *txfwd.TraderAPIForwarder,
) {
	httpServer := &HTTPServer{
		lg:           lg,
		port:         port,
		bxFwd:        bxFwd,
		traderAPIFwd: traderAPIFwd,
	}

	go func() {
		err := httpServer.start()
		if err != nil {
			lg.Errorf("error starting http server: %v", err)
		}
	}()
}

func (s *HTTPServer) start() error {
	http.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		s.handleSubmit(w, r)
	})
	s.lg.Infof("starting http server on port %d", s.port)
	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), nil)
}

func (s *HTTPServer) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write(newResponse("invalid request method"))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.lg.Errorf("failed to read request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(newResponse("failed to read request body"))
		return
	}

	go s.traderAPIFwd.Forward(body)

	var req TraderAPISubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.lg.Errorf("failed to unmarshal request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(newResponse("invalid request body"))
		return
	}

	if req.UseStakedRPCs {
		go s.bxFwd.Forward(req.Transaction.Content)
	}

	tx := solana.Transaction{}
	if err := tx.UnmarshalBase64(req.Transaction.Content); err != nil {
		s.lg.Errorf("failed to unmarshal transaction: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(newResponse("failed to unmarshal transaction"))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(newResponse(tx.Signatures[0].String()))
}

func newResponse(msg string) []byte {
	return []byte(fmt.Sprintf(`{"jsonrpc":"2.0","result":"%s","id":1}`, msg))
}

type TraderAPISubmitRequest struct {
	Transaction struct {
		Content string `json:"content"`
	} `json:"transaction"`
	UseStakedRPCs bool `json:"useStakedRPCs"`
}

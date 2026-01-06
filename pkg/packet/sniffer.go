package packet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
)

const (
	packetLogInterval            = 1200000 // roughly every 10 mins
	invalidPacketsRatioThreshold = 0.01
)

type SniffPacket struct {
	SrcAddr       netip.Addr
	SrcHandleName string
	Payload       [solana.UDPShredSize]byte
	Length        int
	ReceiveTime   time.Time
}

type Validator interface {
	// Validate is suppose to return nil if packet is considered valid or error otherwise
	Validate(payload []byte) error
}

type Counter interface {
	Increment(key netip.Addr)
}

type Option func(*Sniffer)

// WithPacketValidator adds possibility to validate packets before allocating memory for them
// NOTE: validation is blocking so it might slow down the reader in case of heavy cpu-bound operations
func WithPacketValidator(v Validator) Option {
	return func(s *Sniffer) { s.validators = append(s.validators, v) }
}

// WithIntolerant enables intolerant mode: invalid packets cause the sniffer to stop
func WithIntolerant(i bool) Option {
	return func(s *Sniffer) { s.intolerant = i }
}

// WithDynamic sets whether the sniffer is dynamic (is created based on open ports discovery)
func WithDynamic(d bool) Option {
	return func(s *Sniffer) { s.dynamic = d }
}

// WithInvalidPacketsCounter sets the counter for invalid packets
func WithInvalidPacketsCounter(counter Counter) Option {
	return func(s *Sniffer) { s.invalidPacketsCounter = counter }
}

type Sniffer struct {
	HandleName string
	handle     Handler
	intolerant bool
	dynamic    bool

	ctx                   context.Context
	lg                    logger.Logger
	logPrefix             string
	validators            []Validator
	counter               int64
	invalidPackets        int64
	invalidPacketsCounter Counter
}

func NewSniffer(ctx context.Context, lg logger.Logger, logPrefix string, handle Handler, handleName string, opts ...Option) *Sniffer {
	sniffer := &Sniffer{
		handle:                handle,
		HandleName:            handleName,
		ctx:                   ctx,
		lg:                    lg,
		logPrefix:             logPrefix,
		validators:            make([]Validator, 0),
		invalidPacketsCounter: &noopCounter{},
	}

	for _, opt := range opts {
		opt(sniffer)
	}

	return sniffer
}

func (s *Sniffer) SniffUDPNetworkNoClose(ch chan<- SniffPacket) {
	defer s.handle.Close()
	done := s.ctx.Done()

	var eth layers.Ethernet
	var ipv4 layers.IPv4
	var udp layers.UDP
	var payload gopacket.Payload

	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &eth, &ipv4, &udp, &payload)
	decoded := make([]gopacket.LayerType, 0, 3)

	for {
		select {
		case <-done:
			return
		default:
		}

		data, _, err := s.handle.ZeroCopyReadPacketData()
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.lg.Infof("%s: network listener: EOF received", s.logPrefix)
				return
			}

			s.lg.Errorf("%s: network listener: read packet: %s", s.logPrefix, err)
			continue
		}

		pkt, err := s.DecodePacket(parser, &decoded, data, &ipv4, &udp)
		if err != nil {
			s.invalidPackets++
			if s.intolerant || s.dynamic {
				if s.counter > 0 && float64(s.invalidPackets)/float64(s.counter) < invalidPacketsRatioThreshold {
					s.lg.Tracef("%s: network listener: invalid packets ratio is below threshold, skipping packet and keeping sniffer alive, invalid packets : %d ratio: %f",
						s.logPrefix, s.invalidPackets, float64(s.invalidPackets)/float64(s.counter))
					continue
				}
				s.lg.Errorf("%s: network listener: parse udp packet: %s", s.logPrefix, err)
				return
			}

			s.lg.Errorf("%s: network listener: parse udp packet: %s", s.logPrefix, err)

			if !pkt.SrcAddr.IsUnspecified() {
				s.invalidPacketsCounter.Increment(pkt.SrcAddr)
			}

			continue
		}

		s.counter++

		if s.counter%packetLogInterval == 0 {
			s.lg.Tracef("%s: network listener: captured %d packets", s.logPrefix, s.counter)
		}

		select {
		case <-done:
			return
		case ch <- pkt:
		default:
			s.lg.Warnf("%s: network listener: forward packet: channel is full", s.logPrefix)
		}
	}
}

// DecodePacket decodes UDP payload from data ensuring that original bytes are not used
// in the resulting SniffPacket. If error is returned then the packet should not be returned into the pool.
// Returns source address if possible.
func (s *Sniffer) DecodePacket(parser *gopacket.DecodingLayerParser, decoded *[]gopacket.LayerType, data []byte, ipv4 *layers.IPv4, udp *layers.UDP) (pkt SniffPacket, err error) {
	if err = parser.DecodeLayers(data, decoded); err != nil {
		return pkt, fmt.Errorf("decode layers: %w", err)
	}

	if len(*decoded) != 4 {
		return pkt, fmt.Errorf("expected 4 layers, got %d", len(*decoded))
	}

	var ok bool
	// creates new netip.Addr (looses the connection with incoming slice)
	pkt.SrcAddr, ok = netip.AddrFromSlice(ipv4.SrcIP)
	if !ok {
		err = fmt.Errorf("packet contains invalid address: %s", ipv4.SrcIP)
		return
	}

	layerPayload := udp.LayerPayload()
	for _, vdt := range s.validators {
		err = vdt.Validate(layerPayload)
		if err != nil {
			return pkt, fmt.Errorf("packet validation: %w", err)
		}
	}

	pkt.ReceiveTime = time.Now()
	n := copy(pkt.Payload[:], layerPayload)

	// resize if allocated buf if larger than needed
	// e.g. when the packet we got from the pool used to hold larger payload before
	pkt.SrcHandleName = s.HandleName
	pkt.Length = n

	return
}

// IsDynamic returns whether the sniffer is dynamic (is created based on open ports discovery)
func (s *Sniffer) IsDynamic() bool {
	return s.dynamic
}

type noopCounter struct{}

func (n *noopCounter) Increment(netip.Addr) {}

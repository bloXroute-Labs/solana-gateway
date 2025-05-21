package packet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/ofr"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const (
	invalidPacketsStatsInterval = 5 * time.Minute
)

type SniffPacket struct {
	SrcAddr     netip.Addr
	SrcHandle   *pcap.Handle
	Payload     [solana.UDPShredSize]byte
	Length      int
	ReceiveTime time.Time
}

type PacketValidator interface {
	// Validate is suppose to return nil if packet is considered valid or error otherwise
	Validate(payload []byte) error
}

type Option func(*Sniffer)

// WithPacketValidator adds possibility to validate packets before allocating memory for them
// NOTE: validation is blocking so it might slow down the reader in case of heavy cpu-bound operations
func WithPacketValidator(v PacketValidator) Option {
	return func(s *Sniffer) { s.validators = append(s.validators, v) }
}

type Sniffer struct {
	Handle     *pcap.Handle
	Intolerant bool

	ctx        context.Context
	lg         logger.Logger
	logPrefix  string
	validators []PacketValidator
}

func NewSniffer(ctx context.Context, lg logger.Logger, logPrefix string, handle *pcap.Handle, opts ...Option) *Sniffer {
	sniffer := &Sniffer{
		ctx:        ctx,
		lg:         lg,
		logPrefix:  logPrefix,
		Handle:     handle,
		validators: make([]PacketValidator, 0),
	}

	for _, opt := range opts {
		opt(sniffer)
	}

	return sniffer
}

func (s *Sniffer) SniffUDPNetworkNoClose(ch chan<- SniffPacket) {
	defer s.Handle.Close()
	done := s.ctx.Done()

	var invalidPacketsCounter *ofr.KeyedCounter[netip.Addr]
	if !s.Intolerant {
		invalidPacketsCounter = ofr.NewKeyedCounterWithRunner[netip.Addr](s.lg, invalidPacketsStatsInterval, "N invalid packets from Turbine")
	}

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

		data, _, err := s.Handle.ZeroCopyReadPacketData()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}

			s.lg.Errorf("%s: network listener: read packet: %s", s.logPrefix, err)
			continue
		}

		pkt, err := s.DecodePacket(parser, &decoded, data, &ipv4, &udp)
		if err != nil {
			s.lg.Tracef("%s: network listener: parse udp packet: %s", s.logPrefix, err)

			if s.Intolerant {
				return
			}

			if !pkt.SrcAddr.IsUnspecified() {
				invalidPacketsCounter.Increment(pkt.SrcAddr)
			}

			continue
		}

		select {
		case ch <- pkt:
		default:
			s.lg.Warnf("%s: network listener: forward packet: channel is full", s.logPrefix)
		}
	}
}

func (s *Sniffer) SniffUDPNetwork(ch chan<- SniffPacket) {
	defer close(ch)
	s.SniffUDPNetworkNoClose(ch)
}

// DecodePacket decodes UDP payload from data ensuring that original bytes are not used
// in the resulting SniffPacket. If error is returned then the packet should not be returned into the pool.
// Returns source address if possible.
func (s *Sniffer) DecodePacket(parser *gopacket.DecodingLayerParser, decoded *[]gopacket.LayerType, data []byte, ipv4 *layers.IPv4, udp *layers.UDP) (pkt SniffPacket, err error) {
	if err := parser.DecodeLayers(data, decoded); err != nil {
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
		err := vdt.Validate(layerPayload)
		if err != nil {
			return pkt, fmt.Errorf("packet validation: %s", err)
		}
	}

	pkt.ReceiveTime = time.Now()
	n := copy(pkt.Payload[:], layerPayload)

	// resize if allocated buf if larger than needed
	// e.g. when the packet we got from the pool used to hold larger payload before
	pkt.SrcHandle = s.Handle
	pkt.Length = n

	return
}

// NewFileSniffHandle reads pcap from file source
func NewFileSniffHandle(file string) (*pcap.Handle, error) { return pcap.OpenOffline(file) }

func NewUDPNetworkSniffHandle(netInterface string, port int, outgoing bool) (*pcap.Handle, net.IP, error) {
	h, err := pcap.NewInactiveHandle(netInterface)
	if err != nil {
		return nil, nil, err
	}

	// We keep buffer size to default: 2MB
	// https://github.com/the-tcpdump-group/libpcap/blob/master/pcap-linux.c#L2680
	//
	// From pcap man:
	//
	// Packets that arrive for a capture are stored in a buffer,
	// so that they do not have to be read by the application as
	// soon as they arrive. On some platforms, the buffer's size
	// can be set; a size that's too small could mean that, if
	// too many packets are being captured and the snapshot
	// length doesn't limit the amount of data that's buffered,
	// packets could be dropped if the buffer fills up before the
	// application can read packets from it, while a size that's
	// too large could use more non-pageable operating system
	// memory than is necessary to prevent packets from being
	// dropped.

	if err := h.SetImmediateMode(true); err != nil {
		return nil, nil, err
	}

	handle, err := h.Activate()
	if err != nil {
		return nil, nil, err
	}

	inet, err := inetAddr(netInterface)
	if err != nil {
		return nil, nil, err
	}

	dir := "dst"
	if outgoing {
		dir = "src"
	}

	expr := fmt.Sprintf("udp and %s port %d and %s host %s", dir, port, dir, inet)
	if err = handle.SetBPFFilter(expr); err != nil {
		return nil, nil, err
	}

	return handle, inet, nil
}

func inetAddr(netInterface string) (net.IP, error) {
	itf, err := net.InterfaceByName(netInterface)
	if err != nil {
		return nil, err
	}

	addrs, err := itf.Addrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPNet:
			if v.IP.IsLoopback() {
				continue
			}

			if v.IP.To4() != nil {
				return v.IP, nil
			}
		}
	}

	return nil, fmt.Errorf("inet addr for interface %s not found", netInterface)
}

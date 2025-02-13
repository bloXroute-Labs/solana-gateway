package packet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type SniffPacket struct {
	SrcAddr   netip.Addr
	SrcHandle *pcap.Handle
	Payload   []byte

	pool *sync.Pool
}

func (p *SniffPacket) Free() {
	p.pool.Put(p)
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
	pool       *sync.Pool
	validators []PacketValidator
}

func newPacketPoolFunc(pool *sync.Pool) func() interface{} {
	return func() interface{} {
		return &SniffPacket{
			SrcAddr: netip.Addr{},
			Payload: []byte(nil), // don't allocate underlying array for now
			pool:    pool,
		}
	}
}

func NewSniffer(ctx context.Context, lg logger.Logger, handle *pcap.Handle, opts ...Option) *Sniffer {
	var pool = &sync.Pool{}
	pool.New = newPacketPoolFunc(pool)

	var sniffer = &Sniffer{
		ctx:        ctx,
		lg:         lg,
		Handle:     handle,
		pool:       pool,
		validators: make([]PacketValidator, 0),
	}

	for _, opt := range opts {
		opt(sniffer)
	}

	return sniffer
}

func (s *Sniffer) SniffUDPNetworkNoClose(ch chan<- *SniffPacket) {
	defer s.Handle.Close()
	var done = s.ctx.Done()

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

			s.lg.Errorf("sniffer: read packet: %s", err)
			continue
		}

		pkt, err := s.DecodePacket(data)
		if err != nil {
			s.lg.Errorf("sniffer: parse udp packet: %s", err)
			if s.Intolerant {
				return
			}
			continue
		}

		select {
		case ch <- pkt:
		default:
			pkt.Free()
			s.lg.Warn("sniffer: forward packet: channel is full")
		}
	}
}

func (s *Sniffer) SniffUDPNetwork(ch chan<- *SniffPacket) {
	defer close(ch)
	s.SniffUDPNetworkNoClose(ch)
}

// DecodePacket decodes UDP payload from data ensuring that original bytes are not used
// in the resulting SniffPacket. If error is returned then the packet should not be returned into the pool.
func (s *Sniffer) DecodePacket(data []byte) (*SniffPacket, error) {
	packet := gopacket.NewPacket(
		data,
		layers.LayerTypeEthernet,
		gopacket.DecodeOptions{
			NoCopy: true,
		},
	)

	ipv4Layer := packet.Layer(layers.LayerTypeIPv4)
	if ipv4Layer == nil {
		return nil, errors.New("packet does not contain IPv4 Layer")
	}

	ipv4, _ := ipv4Layer.(*layers.IPv4)

	udpLayer := packet.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		return nil, errors.New("packet does not contain UDP Layer")
	}

	var payload = udpLayer.LayerPayload()
	for _, vdt := range s.validators {
		err := vdt.Validate(payload)
		if err != nil {
			return nil, fmt.Errorf("packet validation: %s", err)
		}
	}

	// creates new netip.Addr (looses the connection with incoming slice)
	srcAddr, ok := netip.AddrFromSlice(ipv4.SrcIP)
	if !ok {
		return nil, fmt.Errorf("packet contains invalid address: %s", ipv4.SrcIP)
	}

	var pkt = s.pool.Get().(*SniffPacket)
	if len(payload) > cap(pkt.Payload) {
		pkt.Payload = make([]byte, len(payload))
	}

	copy(pkt.Payload, payload)

	// resize if allocated buf if larger than needed
	// e.g. when the packet we got from the pool used to hold larger payload before
	pkt.Payload = pkt.Payload[:len(payload)]
	pkt.SrcAddr = srcAddr
	pkt.SrcHandle = s.Handle
	return pkt, nil
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

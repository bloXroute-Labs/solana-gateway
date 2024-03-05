package packet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type SniffPacket struct {
	SniffTime time.Time
	Packet    *Packet
}

type Packet struct {
	SrcIP    string
	SrcPort  uint16
	DstIP    string
	DstPort  uint16
	Protocol string
	Payload  []byte
}

type Sniffer struct {
	ctx    context.Context
	lg     logger.Logger
	Handle *pcap.Handle
}

func NewSniffer(ctx context.Context, lg logger.Logger, handle *pcap.Handle) *Sniffer {
	return &Sniffer{
		ctx:    ctx,
		lg:     lg,
		Handle: handle,
	}
}

func (s *Sniffer) SniffUDPNetwork(ch chan<- *SniffPacket) {
	defer s.Handle.Close()
	defer close(ch)
	var done = s.ctx.Done()

	for {
		select {
		case <-done:
			return
		default:
		}

		pkt, ci, err := s.Handle.ReadPacketData()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}

			s.lg.Errorf("sniffer: read packet: %s", err)
			continue
		}

		p, err := parseUDPPacket(pkt)
		if err != nil {
			s.lg.Errorf("sniffer: parse udp packet: %s", err)
			continue
		}

		select {
		case ch <- &SniffPacket{
			SniffTime: ci.Timestamp,
			Packet:    p,
		}:
		default:
			s.lg.Warn("sniffer: forward packet: channel is full")
		}
	}
}

func parseUDPPacket(pkt []byte) (*Packet, error) {
	packet := gopacket.NewPacket(pkt, layers.LayerTypeEthernet, gopacket.Default)

	ipv4Layer := packet.Layer(layers.LayerTypeIPv4)
	if ipv4Layer == nil {
		return nil, errors.New("packet does not contain IPv4 Layer")
	}

	ipv4, _ := ipv4Layer.(*layers.IPv4)

	udpLayer := packet.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		return nil, errors.New("packet does not contain UDP Layer")
	}

	udp, _ := udpLayer.(*layers.UDP)

	return &Packet{
		SrcIP:    ipv4.SrcIP.String(),
		SrcPort:  uint16(udp.SrcPort),
		DstIP:    ipv4.DstIP.String(),
		DstPort:  uint16(udp.DstPort),
		Protocol: "UDP", // hardcoded
		Payload:  udpLayer.LayerPayload(),
	}, nil
}

// NewFileSniffHandle reads pcap from file source
func NewFileSniffHandle(file string) (*pcap.Handle, error) { return pcap.OpenOffline(file) }

func NewIncomingUDPNetworkSniffHandle(netInterface string, port int) (*pcap.Handle, net.IP, error) {
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

	err = handle.SetBPFFilter(fmt.Sprintf("udp and dst port %d and dst host %s", port, inet))
	if err != nil {
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

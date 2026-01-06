package packet

import (
	"fmt"
	"net"

	"github.com/google/gopacket/pcap"
)

// NewUDPNetworkSniffHandle creates pcap handle for sniffing UDP traffic on given interface, IP and port
func NewUDPNetworkSniffHandle(netInterface string, inet net.IP, port int, outgoing bool) (*pcap.Handle, string, error) {
	h, err := pcap.NewInactiveHandle(netInterface)
	if err != nil {
		return nil, "", err
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

	if err = h.SetImmediateMode(true); err != nil {
		return nil, "", fmt.Errorf("set immediate mode: %w", err)
	}

	handle, err := h.Activate()
	if err != nil {
		return nil, "", fmt.Errorf("activate handle: %w", err)
	}

	dir := "dst"
	if outgoing {
		dir = "src"
	}

	expr := fmt.Sprintf("udp and %s port %d and %s host %s", dir, port, dir, inet)
	if err = handle.SetBPFFilter(expr); err != nil {
		return nil, "", fmt.Errorf("set BPF filter (%s): %w", expr, err)
	}

	return handle, expr, nil
}

//go:build linux && cgo

package netutil

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/google/gopacket/afpacket"
	"golang.org/x/net/bpf"
)

const minReqBytes = 14 + 20 + 8 // Ethernet + IP (min) + UDP headers

var checkTime = time.Second

// ListPotentialUDPPorts returns a list of UDP destination ports observed on the specified network interface
func ListPotentialUDPPorts(netInterface string, excludedPorts []int) ([]int, error) {
	h, err := afpacket.NewTPacket(
		afpacket.OptInterface(netInterface),
		afpacket.OptFrameSize(65536),
		afpacket.OptPollTimeout(50*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create afpacket: %w", err)
	}
	defer h.Close()

	// build BPF program using x/net/bpf
	prog, err := bpf.Assemble([]bpf.Instruction{
		// load IP protocol (byte 23 of Ethernet frame)
		bpf.LoadAbsolute{Off: 23, Size: 1},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 17, SkipTrue: 1},
		// return 0 = drop
		bpf.RetConstant{Val: 0},
		// accept: return up to 64KB
		bpf.RetConstant{Val: 65535},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to assemble BPF program: %w", err)
	}

	// attach filter
	if err = h.SetBPF(prog); err != nil {
		return nil, fmt.Errorf("failed to set BPF program: %w", err)
	}

	ports := make(map[uint16]struct{})

	timeout := time.NewTimer(checkTime)
	defer timeout.Stop()

	for {
		data, _, err := h.ZeroCopyReadPacketData()
		if err != nil {
			continue
		}

		// ensure packet contains minimum required bytes
		if len(data) < minReqBytes {
			continue
		}

		// skip Ethernet header (14 bytes)
		ip := data[14:]

		// extract the Internet Header Length (IHL) from the first IP header byte (low 4 bits)
		// and convert to bytes. Ensure the packet contains the full IP header plus at least
		// the 8-byte UDP header; skip otherwise to avoid reading past buffer on malformed packets.
		ihl := int(ip[0]&0x0F) * 4
		if len(ip) < ihl+8 {
			continue
		}

		udp := ip[ihl:]
		dstPort := binary.BigEndian.Uint16(udp[2:4])

		if _, exists := ports[dstPort]; !exists {
			ports[dstPort] = struct{}{}
		}

		select {
		case <-timeout.C:
			portList := make([]int, 0, len(ports))
			for port := range ports {
				skip := false
				for _, p := range excludedPorts {
					if int(port) == p {
						skip = true
						break
					}
				}
				if skip {
					continue
				}
				portList = append(portList, int(port))
			}

			return portList, nil
		default:
			// continue reading packets
		}
	}
}

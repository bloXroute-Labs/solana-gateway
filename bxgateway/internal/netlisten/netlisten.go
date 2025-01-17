package netlisten

import (
	"context"
	"fmt"
	"net"

	"github.com/bloXroute-Labs/solana-gateway/pkg/bdn"
	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/packet"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
	"github.com/google/gopacket/pcap"
)

const recvChBuf = 1e5

type NetworkListener struct {
	ctx          context.Context
	lg           logger.Logger
	cache        *cache.AlterKey
	stats        *bdn.Stats
	inHandles    []*pcap.Handle
	outHandles   []*pcap.Handle
	statsAddr    *net.UDPAddr
	netInterface string
}

func NewNetworkListener(ctx context.Context, lg logger.Logger, cache *cache.AlterKey, stats *bdn.Stats, netInterface string, inPorts, outPorts []int) (*NetworkListener, error) {
	nl := &NetworkListener{
		ctx:          ctx,
		lg:           lg,
		cache:        cache,
		stats:        stats,
		netInterface: netInterface,
		inHandles:    make([]*pcap.Handle, 0, len(inPorts)),
		outHandles:   make([]*pcap.Handle, 0, len(outPorts)),
	}

	var statsIP net.IP

	for i, ports := range [][]int{inPorts, outPorts} {
		outgoing := i != 0

		handles := &nl.inHandles
		if outgoing {
			handles = &nl.outHandles
		}

		for _, p := range ports {
			handle, ip, err := packet.NewUDPNetworkSniffHandle(netInterface, p, outgoing)
			if err != nil {
				return nil, fmt.Errorf("new network listen handle: %w", err)
			}

			*handles = append(*handles, handle)
			statsIP = ip
		}
	}

	nl.statsAddr = &net.UDPAddr{IP: statsIP}
	return nl, nil
}

func (s *NetworkListener) Recv(ch chan<- *solana.PartialShred) {
	var sniffRecvCh = make(chan *packet.SniffPacket, recvChBuf)
	var done = s.ctx.Done()

	for _, handle := range append(s.inHandles, s.outHandles...) {
		go packet.NewSniffer(s.ctx, s.lg, handle).SniffUDPNetwork(sniffRecvCh)
	}

	localIP := s.statsAddr.IP.String()
	for i := 0; ; i++ {
		select {
		case sp := <-sniffRecvCh:
			shred, err := solana.ParseShredPartial(sp.Packet.Payload)
			if err != nil {
				s.lg.Errorf("partial parse shred: %s", err)
				continue
			}

			s.lg.Tracef("net-listener: recv shred, slot: %d, index: %d", shred.Slot, shred.Index)
			if i == 1e5 {
				s.lg.Debugf("health: NetworkListener.Recv 100K: broadcast buf: %d", len(ch))
				i = 0
			}

			s.stats.RecordNewShred(s.statsAddr)

			if !s.cache.Set(solana.ShredKey(shred.Slot, shred.Index, shred.Variant)) {
				continue
			}

			if len(s.outHandles) != 0 {
				outgoing := sp.Packet.SrcIP == localIP
				if outgoing {
					s.lg.Infof("broadcast shred %d:%d", shred.Slot, shred.Index)
				}
			}
			s.stats.RecordUnseenShred(s.statsAddr, shred)

			select {
			case ch <- shred:
			default:
				s.lg.Warn("net-listener: unable to forward packet: channel is full")
			}
		case <-done:
			close(ch)
			return
		}
	}
}

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
	handles      []*pcap.Handle
	statsAddr    *net.UDPAddr
	netInterface string
}

func NewNetworkListener(ctx context.Context, lg logger.Logger, cache *cache.AlterKey, stats *bdn.Stats, netInterface string, ports []int) (*NetworkListener, error) {
	nl := &NetworkListener{
		ctx:          ctx,
		lg:           lg,
		cache:        cache,
		stats:        stats,
		netInterface: netInterface,
		handles:      make([]*pcap.Handle, 0, len(ports)),
	}

	var statsIP net.IP
	for _, p := range ports {
		handle, ip, err := packet.NewIncomingUDPNetworkSniffHandle(netInterface, p)
		if err != nil {
			return nil, fmt.Errorf("new network listen handle: %w", err)
		}

		nl.handles = append(nl.handles, handle)
		statsIP = ip
	}

	nl.statsAddr = &net.UDPAddr{IP: statsIP}
	return nl, nil
}

func (s *NetworkListener) Recv(ch chan<- *solana.PartialShred) {
	var sniffRecvCh = make(chan *packet.SniffPacket, recvChBuf)
	var done = s.ctx.Done()

	for _, handle := range s.handles {
		go packet.NewSniffer(s.ctx, s.lg, handle).SniffUDPNetwork(sniffRecvCh)
	}

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

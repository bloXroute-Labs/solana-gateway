package netlisten

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"

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
	sniffers     []*packet.Sniffer
	localAddr    netip.Addr
	netInterface string
	listenPorts  map[*pcap.Handle]*portData
}

type portData struct {
	cancel   context.CancelFunc
	outgoing bool
	port     uint16
}

func (pd *portData) LogPrefix() string {
	dir := "in"
	if pd.outgoing {
		dir = "out"
	}
	return fmt.Sprintf("%s port %d", dir, pd.port)
}

func NewNetworkListener(ctx context.Context, lg logger.Logger, cache *cache.AlterKey, stats *bdn.Stats, netInterface string, inPorts, outPorts []int) (*NetworkListener, error) {
	nl := &NetworkListener{
		ctx:          ctx,
		lg:           lg,
		cache:        cache,
		stats:        stats,
		netInterface: netInterface,
		sniffers:     make([]*packet.Sniffer, 0, len(inPorts)+len(outPorts)),
		listenPorts:  make(map[*pcap.Handle]*portData),
	}

	var statsIP net.IP

	packetValidator := NewPacketValidator()
	for i, ports := range [][]int{inPorts, outPorts} {
		outgoing := i != 0

		for _, p := range ports {
			handle, ip, err := packet.NewUDPNetworkSniffHandle(netInterface, p, outgoing)
			if err != nil {
				return nil, fmt.Errorf("new network listen handle: %w", err)
			}

			ctx2, cancel := context.WithCancel(ctx)
			portData := portData{
				cancel:   cancel,
				outgoing: outgoing,
				port:     uint16(p),
			}
			nl.listenPorts[handle] = &portData

			sniffer := packet.NewSniffer(ctx2, nl.lg, handle, packet.WithPacketValidator(packetValidator))
			sniffer.Intolerant = outgoing
			nl.sniffers = append(nl.sniffers, sniffer)

			statsIP = ip
		}
	}

	netipAddr := netip.AddrFrom4([4]byte(statsIP.To4()))
	nl.localAddr = netipAddr
	return nl, nil
}

func (s *NetworkListener) Recv(ch chan<- *packet.SniffPacket) {
	var sniffRecvCh = make(chan *packet.SniffPacket, recvChBuf)
	var done = s.ctx.Done()

	var wg sync.WaitGroup
	for _, sniffer := range s.sniffers {
		wg.Add(1)
		go func(sniffer *packet.Sniffer) {
			sniffer.SniffUDPNetworkNoClose(sniffRecvCh)
			wg.Done()
		}(sniffer)
	}
	go func() {
		wg.Wait()
		close(sniffRecvCh)
	}()

	for i := 0; ; i++ {
		select {
		case sp := <-sniffRecvCh:
			pd, ok := s.listenPorts[sp.SrcHandle]
			if !ok {
				continue
			}

			shred, err := solana.ParseShredPartial(sp.Payload)
			if err != nil {
				sp.Free()
				s.lg.Errorf("%s: partial parse shred: %s", pd.LogPrefix(), err)
				if pd.outgoing {
					delete(s.listenPorts, sp.SrcHandle)
					pd.cancel()
				}
				continue
			}

			if i == 1e5 {
				s.lg.Debugf("health: NetworkListener.Recv 100K: broadcast buf: %d", len(ch))
				i = 0
			}

			s.stats.RecordNewShred(s.localAddr)

			if !s.cache.Set(solana.ShredKey(shred.Slot, shred.Index, shred.Variant)) {
				sp.Free()
				if pd.outgoing {
					s.lg.Errorf("%s: duplicate outgoing shred %d:%d", pd.LogPrefix(), shred.Slot, shred.Index)
					delete(s.listenPorts, sp.SrcHandle)
					pd.cancel()
				}
				continue
			}

			s.stats.RecordUnseenShred(s.localAddr, shred)

			if pd.outgoing && shred.Index == 0 {
				s.lg.Infof("%s: broadcast slot %d", pd.LogPrefix(), shred.Slot)
			}

			select {
			case ch <- sp:
			default:
				sp.Free()
				s.lg.Warn("net-listener: unable to forward packet: channel is full")
			}
		case <-done:
			close(ch)
			return
		}
	}
}

func (s *NetworkListener) Addr() netip.Addr { return s.localAddr }

type packetValidator struct{}

func NewPacketValidator() packet.PacketValidator { return &packetValidator{} }

func (v *packetValidator) Validate(payload []byte) error {
	size := len(payload)
	err := solana.ValidateShredSizeSimple(size)
	if err != nil {
		return fmt.Errorf("not a shred, size: %d: %s", size, err)
	}

	return nil
}

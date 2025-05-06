package netlisten

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/ofr"
	"github.com/bloXroute-Labs/solana-gateway/pkg/packet"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
	"github.com/google/gopacket/pcap"
)

const (
	recvChBuf                  = 1e5
	noOutgoingTrafficThreshold = time.Hour * 6
)

var pktValidator = NewPacketValidator()

type NetworkListener struct {
	ctx           context.Context
	lg            logger.Logger
	cache         *cache.AlterKey
	stats         *ofr.Stats
	sniffers      []*packet.Sniffer
	localAddr     netip.Addr
	netInterface  string
	listenPorts   map[*pcap.Handle]*portData
	portHandleMap map[*portData]*pcap.Handle
}

type portData struct {
	cancel   context.CancelFunc
	outgoing bool
	port     uint16
}

func (pd *portData) String() string {
	dir := "in"
	if pd.outgoing {
		dir = "out"
	}
	return fmt.Sprintf("%s port %d", dir, pd.port)
}

func NewNetworkListener(ctx context.Context, lg logger.Logger, cache *cache.AlterKey, stats *ofr.Stats, netInterface string, inPorts, outPorts []int) (*NetworkListener, error) {
	nl := &NetworkListener{
		ctx:           ctx,
		lg:            lg,
		cache:         cache,
		stats:         stats,
		netInterface:  netInterface,
		sniffers:      make([]*packet.Sniffer, 0, len(inPorts)+len(outPorts)),
		listenPorts:   make(map[*pcap.Handle]*portData),
		portHandleMap: make(map[*portData]*pcap.Handle),
	}

	var statsIP net.IP

	for i, ports := range [][]int{inPorts, outPorts} {
		outgoing := i != 0

		for _, p := range ports {
			portData := portData{
				outgoing: outgoing,
				port:     uint16(p),
			}

			sniffer, ip, err := nl.registerNewSniffer(&portData)
			if err != nil {
				return nil, fmt.Errorf("new sniffer: %w", err)
			}

			nl.sniffers = append(nl.sniffers, sniffer)
			statsIP = ip
		}
	}

	netipAddr := netip.AddrFrom4([4]byte(statsIP.To4()))
	nl.localAddr = netipAddr
	return nl, nil
}

func (s *NetworkListener) Recv(ch chan<- *packet.SniffPacket) {
	sniffRecvCh := make(chan *packet.SniffPacket, recvChBuf)
	done := s.ctx.Done()

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

	noOutgoingTraffic := time.NewTicker(noOutgoingTrafficThreshold)
	defer noOutgoingTraffic.Stop()

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
				s.lg.Tracef("%s: partial parse shred: %s", pd, err)

				if pd.outgoing {
					s.portHandleMap[pd] = nil
					delete(s.listenPorts, sp.SrcHandle)
					pd.cancel()
				}
				continue
			}

			if i == 1e5 {
				s.lg.Debugf("health: NetworkListener.Recv 100K: broadcast buf: %d", len(ch))
				i = 0
			}

			s.stats.RecordNewShred(s.localAddr, "Turbine")

			if !s.cache.Set(solana.ShredKey(shred.Slot, shred.Index, shred.Variant)) {
				sp.Free()
				if pd.outgoing {
					s.lg.Debugf("%s: duplicate outgoing shred %d:%d", pd, shred.Slot, shred.Index)

					// TODO: find out why this is happening and uncomment
					// s.portHandleMap[pd] = nil
					// delete(s.listenPorts, sp.SrcHandle)
					// pd.cancel()
				}
				continue
			}

			if pd.outgoing {
				noOutgoingTraffic.Reset(noOutgoingTrafficThreshold)
			}

			s.stats.RecordUnseenShred(s.localAddr, shred, "Turbine")

			if pd.outgoing && shred.Index == 0 {
				s.lg.Infof("%s: broadcast slot %d", pd, shred.Slot)
			}

			select {
			case ch <- sp:
			default:
				sp.Free()
				s.lg.Warn("net-listener: unable to forward packet: channel is full")
			}
		case <-noOutgoingTraffic.C:
			for pd, handle := range s.portHandleMap {
				// No handler means it is closed
				if handle != nil {
					continue
				}

				sniffer, _, err := s.registerNewSniffer(pd)
				if err != nil {
					s.lg.Errorf("new sniffer: %s: %s", pd, err)
					continue
				}

				wg.Add(1)
				go func(sniffer *packet.Sniffer) {
					sniffer.SniffUDPNetworkNoClose(sniffRecvCh)
					wg.Done()
				}(sniffer)
			}
		case <-done:
			close(ch)
			return
		}
	}
}

func (s *NetworkListener) registerNewSniffer(pd *portData) (*packet.Sniffer, net.IP, error) {
	handle, ip, err := packet.NewUDPNetworkSniffHandle(s.netInterface, int(pd.port), pd.outgoing)
	if err != nil {
		return nil, nil, fmt.Errorf("new network listen handle on iface %s: %w", s.netInterface, err)
	}

	ctx, cancel := context.WithCancel(s.ctx)
	pd.cancel = cancel
	s.listenPorts[handle] = pd
	s.portHandleMap[pd] = handle

	sniffer := packet.NewSniffer(ctx, s.lg, pd.String(), handle, packet.WithPacketValidator(pktValidator))
	sniffer.Intolerant = pd.outgoing

	return sniffer, ip, nil
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

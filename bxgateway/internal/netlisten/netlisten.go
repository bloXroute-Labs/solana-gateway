package netlisten

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/cornelk/hashmap"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/netutil"
	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/ofr"
	"github.com/bloXroute-Labs/solana-gateway/pkg/packet"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
)

const (
	invalidPacketsStatsInterval = 5 * time.Minute
	recvChBuf                   = 1e5
	noOutgoingTrafficThreshold  = time.Hour * 6
	openPortsCheckInterval      = time.Hour * 6
)

var pktValidator = NewPacketValidator()

type NetworkListener struct {
	ctx                context.Context
	lg                 logger.Logger
	cache              *cache.AlterKey
	stats              *ofr.Stats
	sniffers           []*packet.Sniffer
	localAddr          netip.Addr
	netInterface       string
	stakedNodes        bool
	listenPorts        *hashmap.Map[string, *portData]      // handle name -> port data
	portHandleMap      *hashmap.Map[string, portWithHandle] // port info -> pcap handle + port data
	ip                 net.IP
	inPort             int
	outPorts           []int
	serverPort         int
	invalidPcktCounter *ofr.KeyedCounter[netip.Addr]
}

type portData struct {
	cancel   context.CancelFunc
	outgoing bool
	dynamic  bool
	port     uint16
}

type portWithHandle struct {
	pd     *portData
	handle packet.Handler
}

func (pd *portData) String() string {
	dir := "in"
	if pd.outgoing {
		dir = "out"
	}
	return fmt.Sprintf("%s port %d", dir, pd.port)
}

func NewNetworkListener(ctx context.Context, lg logger.Logger, cache *cache.AlterKey, stats *ofr.Stats, netInterface string, stakedNodes bool, inPort int, outPorts []int, serverPort int) (*NetworkListener, error) {
	nl := &NetworkListener{
		ctx:                ctx,
		lg:                 lg,
		cache:              cache,
		stats:              stats,
		netInterface:       netInterface,
		stakedNodes:        stakedNodes,
		listenPorts:        hashmap.New[string, *portData](),
		portHandleMap:      hashmap.New[string, portWithHandle](),
		inPort:             inPort,
		outPorts:           outPorts,
		serverPort:         serverPort,
		invalidPcktCounter: ofr.NewKeyedCounterWithRunner[netip.Addr](ctx, lg, invalidPacketsStatsInterval, "N invalid packets from Turbine"),
	}

	ip, err := inetAddr(netInterface)
	if err != nil {
		return nil, err
	}
	nl.ip = ip
	nl.localAddr = netip.AddrFrom4([4]byte(ip.To4()))

	if err = nl.registerSniffers(); err != nil {
		return nil, err
	}

	return nl, nil
}

type Shred struct {
	Packet packet.SniffPacket
	Shred  solana.PartialShred
}

func (s *NetworkListener) Recv(ch chan<- Shred) {
	sniffRecvCh := make(chan packet.SniffPacket, recvChBuf)
	done := s.ctx.Done()

	var wg sync.WaitGroup
	for _, sniffer := range s.sniffers {
		wg.Add(1)
		go func(sniffer *packet.Sniffer) {
			if sniffer.IsDynamic() {
				defer func() {
					pd, ok := s.listenPorts.Get(sniffer.HandleName)
					if ok && pd != nil {
						s.portHandleMap.Del(pd.String())
						s.listenPorts.Del(sniffer.HandleName)
						if pd.cancel != nil {
							pd.cancel()
						}
					} else {
						s.lg.Warnf("dynamic sniffer cleanup: missing port data for handle %s", sniffer.HandleName)
					}
				}()
			}
			sniffer.SniffUDPNetworkNoClose(sniffRecvCh)
			wg.Done()
		}(sniffer)
	}

	go func() {
		wg.Wait()
		close(sniffRecvCh)
	}()

	if s.stakedNodes {
		// start monitoring open ports in the background
		go s.startMonitoringOpenPorts(&wg, sniffRecvCh)
	}

	noOutgoingTraffic := time.NewTicker(noOutgoingTrafficThreshold)
	defer noOutgoingTraffic.Stop()

	for i := 0; ; i++ {
		select {
		case sp := <-sniffRecvCh:
			pd, ok := s.listenPorts.Get(sp.SrcHandleName)
			if !ok {
				continue
			}

			shred, err := solana.ParseShredPartial(sp.Payload, sp.Length)
			if err != nil {
				s.lg.Tracef("%s: partial parse shred: %s", pd, err)

				if pd.outgoing {
					s.portHandleMap.Set(pd.String(), portWithHandle{pd: pd, handle: nil})
					s.listenPorts.Del(sp.SrcHandleName)
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
				if pd.outgoing {
					s.lg.Infof("%s: duplicate outgoing shred %d:%d", pd, shred.Slot, shred.Index)

					// TODO: find out why this is happening and uncomment
					// s.portHandleMap[pd] = nil
					// delete(s.listenPorts, sp.SrcHandleName)
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
			case ch <- Shred{
				Packet: sp,
				Shred:  shred,
			}:
			default:
				s.lg.Warn("net-listener: unable to forward packet: channel is full")
			}
		case <-noOutgoingTraffic.C:
			s.portHandleMap.Range(func(_ string, pdh portWithHandle) bool {
				// no handler means it is closed
				if pdh.handle != nil {
					return true
				}

				sniffer, err := s.registerNewSniffer(pdh.pd)
				if err != nil {
					s.lg.Errorf("new sniffer: %s: %s", pdh.pd, err)
					return true
				}

				wg.Add(1)
				go func(sniffer *packet.Sniffer) {
					sniffer.SniffUDPNetworkNoClose(sniffRecvCh)
					wg.Done()
				}(sniffer)

				return true
			})
		case <-done:
			// wait for all sniffers to finish
			wg.Wait()

			close(ch)
			return
		}
	}
}

func (s *NetworkListener) registerSniffers() error {
	if len(s.outPorts) > 0 {
		if err := s.registerNewSniffers(true, s.outPorts); err != nil {
			return err
		}
	}

	// the ports are known, register them directly without re-scanning
	if err := s.registerNewSniffers(false, []int{s.inPort}); err != nil {
		return err
	}

	if s.stakedNodes {

		dynamicSniffers := s.createDynamicSniffers()

		s.sniffers = append(s.sniffers, dynamicSniffers...)
	}

	return nil
}

func (s *NetworkListener) registerNewSniffers(outgoing bool, ports []int) error {
	for _, p := range ports {
		pd := portData{
			outgoing: outgoing,
			port:     uint16(p),
		}
		sniffer, err := s.registerNewSniffer(&pd)
		if err != nil {
			return fmt.Errorf("register sniffer on port %d: %w", p, err)
		}
		s.sniffers = append(s.sniffers, sniffer)
	}

	return nil
}

func (s *NetworkListener) registerNewSniffer(pd *portData) (*packet.Sniffer, error) {
	handle, handleName, err := packet.NewUDPNetworkSniffHandle(s.netInterface, s.ip, int(pd.port), pd.outgoing)
	if err != nil {
		return nil, fmt.Errorf("new network listen handle on iface %s: %w", s.netInterface, err)
	}

	s.lg.Tracef("registered sniffer on %s (handle: %s)", pd.String(), handleName)

	ctx, cancel := context.WithCancel(s.ctx)
	pd.cancel = cancel

	s.listenPorts.Set(handleName, pd)
	s.portHandleMap.Set(pd.String(), portWithHandle{pd: pd, handle: handle})

	opts := []packet.Option{packet.WithPacketValidator(pktValidator),
		packet.WithIntolerant(pd.outgoing),
		packet.WithDynamic(pd.dynamic),
	}
	if pd.outgoing {
		opts = append(opts, packet.WithInvalidPacketsCounter(s.invalidPcktCounter))
	}

	sniffer := packet.NewSniffer(ctx, s.lg, pd.String(), handle, handleName, opts...)

	return sniffer, nil
}

func (s *NetworkListener) createDynamicSniffers() []*packet.Sniffer {
	inPorts, err := netutil.ListPotentialUDPPorts(s.netInterface, append(s.outPorts, []int{s.inPort, s.serverPort}...))
	if err != nil {
		s.lg.Errorf("list open udp ports: %s", err)
		return nil
	}

	if len(inPorts) == 0 {
		return nil
	}

	var pds []portData

	for _, p := range inPorts {
		pd := portData{
			outgoing: false,
			port:     uint16(p),
			dynamic:  true,
		}
		if _, ok := s.portHandleMap.Get(pd.String()); ok {
			// already have a sniffer on this port, no need to check again
			continue
		}

		pds = append(pds, pd)
	}

	if len(pds) == 0 {
		s.lg.Info("no new open ports detected")
		return nil
	}

	var res []*packet.Sniffer

	for i := range pds {
		pd := &pds[i]

		sniffer, err := s.registerNewSniffer(pd)
		if err != nil {
			s.lg.Errorf("register sniffer on port %d: %s", pd.port, err)
			continue
		}

		res = append(res, sniffer)
	}

	return res
}

func (s *NetworkListener) startMonitoringOpenPorts(wg *sync.WaitGroup, sniffRecvCh chan<- packet.SniffPacket) {
	monitoringTicker := time.NewTicker(openPortsCheckInterval)
	defer monitoringTicker.Stop()

	for {
		select {
		case <-monitoringTicker.C:
			dynamicSniffers := s.createDynamicSniffers()

			for _, sniffer := range dynamicSniffers {
				wg.Add(1)
				go func(sniffer *packet.Sniffer) {
					defer func() {
						pd, ok := s.listenPorts.Get(sniffer.HandleName)
						if ok && pd != nil {
							s.portHandleMap.Del(pd.String())
							s.listenPorts.Del(sniffer.HandleName)
							if pd.cancel != nil {
								pd.cancel()
							}
						} else {
							s.lg.Warnf("monitoring sniffer cleanup: missing port data for handle %s", sniffer.HandleName)
						}
						wg.Done()
					}()
					sniffer.SniffUDPNetworkNoClose(sniffRecvCh)
				}(sniffer)
			}
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *NetworkListener) Addr() netip.Addr {
	if s == nil {
		return netip.Addr{}
	}

	return s.localAddr
}

type packetValidator struct{}

func NewPacketValidator() packet.Validator { return &packetValidator{} }

func (v *packetValidator) Validate(payload []byte) error {
	size := len(payload)
	err := solana.ValidateShredSizeSimple(size)
	if err != nil {
		return fmt.Errorf("not a shred, size: %d: %w", size, err)
	}

	return nil
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
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4, nil
			}
		}
	}

	return nil, fmt.Errorf("inet addr for interface %s not found", netInterface)
}

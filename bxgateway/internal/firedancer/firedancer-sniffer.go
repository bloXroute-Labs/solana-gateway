package firedancer

/*
#cgo CFLAGS: -I${SRCDIR}/lib
#cgo linux LDFLAGS: -L${SRCDIR}/lib -lfdshredmonitor -lpthread -lrt
#cgo darwin LDFLAGS: -L${SRCDIR}/lib -lfdshredmonitor -lpthread -Wl,-rpath,${SRCDIR}/lib
#include "fd_shred_monitor.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"net/netip"
	"time"
	"unsafe"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/netlisten"
	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/ofr"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
)

const FireDancerNodeName = "Firedancer"

type FiredancerSniffer struct {
	ctx       context.Context
	lg        logger.Logger
	stats     *ofr.Stats
	cache     *cache.AlterKey
	localAddr netip.Addr
	logPrefix string
}

func NewFiredancerSniffer(ctx context.Context, lg logger.Logger, stats *ofr.Stats, cache *cache.AlterKey) (*FiredancerSniffer, error) {
	sniffer := &FiredancerSniffer{
		ctx:       ctx,
		lg:        lg,
		stats:     stats,
		cache:     cache,
		localAddr: netip.AddrFrom4([4]byte{127, 0, 0, 1}),
		logPrefix: "fd_sniffer",
	}

	if C.shred_monitor_init(nil) != 0 {
		return nil, fmt.Errorf("failed to initialize shred monitor")
	}

	return sniffer, nil
}

func (s *FiredancerSniffer) Recv(ch chan<- netlisten.Shred) {
	defer func() {
		C.shred_monitor_stop()
		C.shred_monitor_cleanup()
	}()

	var shredInfo C.shred_info_t

	s.lg.Infof("%s: Starting shred monitoring...", s.logPrefix)

	totalShreds := 0
	lastStatsTime := time.Now()
	lastSecondCount := 0

	for C.shred_monitor_is_running() {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		result := C.shred_monitor_get_next(&shredInfo)

		switch result {
		case 1: // Got a shred
			var pkt netlisten.Shred
			pkt.Packet.ReceiveTime = time.Now()
			pkt.Packet.Payload = *(*[1228]byte)(unsafe.Pointer(shredInfo.raw_data))
			pkt.Packet.Length = int(shredInfo.size)
			pkt.Packet.SrcAddr = netip.Addr{} // No source address for FD
			pkt.Packet.SrcHandle = nil        // No handle for FD

			lastSecondCount++
			if time.Since(lastStatsTime) >= 1*time.Second {
				totalShreds += lastSecondCount
				s.lg.Infof("%s: +%d shreds/sec | Total: %d | Latest: slot=%d idx=%d (batch_seq=%d)",
					s.logPrefix,
					lastSecondCount,
					totalShreds,
					uint64(shredInfo.slot),
					uint32(shredInfo.idx),
					uint64(shredInfo.batch_seq),
				)
				lastSecondCount = 0
				lastStatsTime = time.Now()
			}

			variant, err := solana.ParseShredVariant(uint8(shredInfo.variant))
			if err != nil {
				s.lg.Errorf("%s: parse shred variant: %s", s.logPrefix, err)
				continue
			}

			shred := solana.PartialShred{
				Slot:    uint64(shredInfo.slot),
				Index:   uint32(shredInfo.idx),
				Variant: variant,
			}

			pkt.Shred = shred

			// Use localAddr for stats in both modes
			s.stats.RecordNewShred(s.localAddr, FireDancerNodeName)

			if !s.cache.Set(solana.ShredKey(uint64(shredInfo.slot), uint32(shredInfo.idx), variant)) {
				s.lg.Debugf("%s: duplicate outgoing shred %d:%d", s.logPrefix, shredInfo.slot, shredInfo.idx)
				continue
			}

			s.stats.RecordUnseenShred(s.localAddr, shred, FireDancerNodeName)

			select {
			case ch <- pkt:
			default:
				s.lg.Warnf("%s: forward packet: channel is full", s.logPrefix)
			}

		case 0: // No new shreds available
			time.Sleep(time.Microsecond * 100)
			continue
		case -1: // Error
			s.lg.Errorf("%s: Error getting next shred", s.logPrefix)
			return
		}
	}
}

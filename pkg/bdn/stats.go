package bdn

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
)

type KeyedCounter[T comparable] struct {
	counter map[T]uint64
	mx      sync.Mutex
}

// NewKeyedCounterWithRunner same as NewKeyedCounter but with built-in RunPrettyPrint
func NewKeyedCounterWithRunner[T comparable](l logger.Logger, delay time.Duration, label string) *KeyedCounter[T] {
	counter := NewKeyedCounter[T]()
	go counter.RunPrettyPrint(l, delay, label)
	return counter
}

func NewKeyedCounter[T comparable]() *KeyedCounter[T] {
	return &KeyedCounter[T]{
		counter: make(map[T]uint64),
		mx:      sync.Mutex{},
	}
}

func (m *KeyedCounter[T]) Increment(key T) {
	m.mx.Lock()
	defer m.mx.Unlock()
	m.counter[key] += 1
}

func (m *KeyedCounter[T]) Flush() map[T]uint64 {
	m.mx.Lock()
	defer m.mx.Unlock()
	ret := m.counter
	m.counter = make(map[T]uint64)
	return ret
}

func (m *KeyedCounter[T]) FlushPretty() string {
	mp := m.Flush()

	statsStr := make([]string, 0)
	for k, counter := range mp {
		// note: these are not sorted
		statsStr = append(statsStr, fmt.Sprintf("(%v, %d)", k, counter))
	}

	return fmt.Sprintf("[%s]", strings.Join(statsStr, ", "))
}

func (m *KeyedCounter[T]) RunPrettyPrint(l logger.Logger, label time.Duration, prefix string) {
	nextLogTime := time.Now().Truncate(label).Add(label)
	time.Sleep(time.Until(nextLogTime))

	for {
		l.Infof("stats: %s: %s", prefix, m.FlushPretty())
		nextLogTime = nextLogTime.Add(label)
		time.Sleep(time.Until(nextLogTime))
	}
}

const shredsRecorderChanBuf = 1e4

type Stats struct {
	flushTimeout time.Duration
	lg           logger.Logger
	fluentD      *FluentD
	relays       []string

	totalShredsRecorder  *shredsBySrcRecorder
	unseenShredsRecorder *shredsBySrcRecorder
	firstShredRecorder   *firstShredRecorder
}

type (
	shredsBySrcRecorder struct {
		ch chan *net.UDPAddr
		mp map[string]int
		mx sync.Mutex
	}

	shredsBySrcRecord struct {
		src    string
		shreds int
	}
)

type (
	firstShredRecorder struct {
		mp map[string]time.Time // slot-type: record_time
		mx sync.Mutex
		ch chan *firstShredRecord
		lg logger.Logger
	}

	firstShredRecord struct {
		src   *net.UDPAddr
		slot  uint64
		index uint32
		typ   string
	}
)

func NewStats(lg logger.Logger, timeout time.Duration, opts ...StatsOption) *Stats {
	st := &Stats{
		flushTimeout: timeout,
		lg:           lg,

		totalShredsRecorder:  newShredsBySrcRecorder(),
		unseenShredsRecorder: newShredsBySrcRecorder(),
		firstShredRecorder:   newFirstShredRecorder(lg),
	}

	for _, o := range opts {
		o(st)
	}

	go func() {
		time.Sleep(time.Until(nextZeroSecondWindow()))
		ticker := time.NewTicker(st.flushTimeout)

		go func() {
			for {
				<-ticker.C

				totalShreds := st.totalShredsRecorder.flush()
				unseenShreds := st.unseenShredsRecorder.flush()

				st.lg.Infof("stats: total shreds by source: %s unseen shreds by source: %s",
					totalShreds.String(), unseenShreds.String())

				st.firstShredRecorder.clean()

				if st.fluentD != nil {
					for _, ts := range totalShreds.stats {
						for _, us := range unseenShreds.stats {
							if ts.src != us.src {
								continue
							}

							ipPort := strings.Split(ts.src, ":")
							if len(ipPort) != 2 {
								lg.Errorf("stats: invalid ip-port: %s", ts.src)
								break
							}

							st.fluentD.LogShredStats(ipPort[0], uint32(ts.shreds), uint32(us.shreds))
							break
						}
					}
				}
			}
		}()
	}()

	return st
}

type StatsOption func(*Stats)

// TODO: this is way too highly coupled, stats and fluentd domains must be redesigned!

func StatsWithFluentD(fd *FluentD, relays []*net.UDPAddr) StatsOption {
	return func(s *Stats) {
		s.fluentD = fd
		s.relays = make([]string, 0, len(relays))
		for _, r := range relays {
			s.relays = append(s.relays, r.String())
		}
	}
}

func (s *Stats) RecordNewShred(src *net.UDPAddr) { s.totalShredsRecorder.record(src) }

func (s *Stats) RecordUnseenShred(src *net.UDPAddr, shred *solana.PartialShred) {
	s.unseenShredsRecorder.record(src)
	s.firstShredRecorder.record(src, shred)
}

func (s *Stats) RecordNewGateway(peerIP string, version string, accountID string) {
	s.fluentD.LogConnectedGateway(peerIP, version, accountID)
}

func nextZeroSecondWindow() time.Time {
	t := time.Now()
	secDiff := time.Second * time.Duration(59-t.Second())
	nanoDiff := time.Duration(1e9 - t.Nanosecond())
	return t.Add(secDiff).Add(nanoDiff)
}

func newShredsBySrcRecorder() *shredsBySrcRecorder {
	rec := &shredsBySrcRecorder{
		ch: make(chan *net.UDPAddr, shredsRecorderChanBuf),
		mp: make(map[string]int),
	}

	go rec.run()
	return rec
}

func newFirstShredRecorder(lg logger.Logger) *firstShredRecorder {
	rec := &firstShredRecorder{
		mp: make(map[string]time.Time),
		mx: sync.Mutex{},
		ch: make(chan *firstShredRecord, shredsRecorderChanBuf),
		lg: lg,
	}

	go rec.run()
	return rec
}

func (r *firstShredRecorder) record(src *net.UDPAddr, shred *solana.PartialShred) {
	select {
	case r.ch <- &firstShredRecord{
		src:   src,
		slot:  shred.Slot,
		index: shred.Index,
		typ:   shred.Variant.String(),
	}:
	default:
		r.lg.Warn("firstShredRecorder: channel is full")
	}
}

func (r *firstShredRecorder) run() {
	for shred := range r.ch {
		if shred.index == 0 {
			r.lg.Debugf("stats: ZERO shred from: %s slot: %d type: %s", shred.src.String(), shred.slot, shred.typ)
		}

		if shred.index != 0 && shred.index%200 == 0 {
			r.lg.Debugf("stats: MIDDLE shred from: %s slot: %d index: %d type: %s", shred.src.String(), shred.slot, shred.index, shred.typ)
		}

		key := fmt.Sprintf("%d%s", shred.slot, shred.typ)

		r.mx.Lock()
		if _, ok := r.mp[key]; !ok {
			r.lg.Debugf("stats: FIRST shred from: %s slot: %d index: %d type: %s", shred.src.String(), shred.slot, shred.index, shred.typ)
			r.mp[key] = time.Now()
		}
		r.mx.Unlock()
	}
}

func (r *firstShredRecorder) clean() {
	r.mx.Lock()
	for k, v := range r.mp {
		if time.Since(v) > time.Minute {
			delete(r.mp, k)
		}
	}
	r.mx.Unlock()
}

func (r *shredsBySrcRecorder) record(src *net.UDPAddr) {
	select {
	case r.ch <- src:
	default:
	}
}

func (r *shredsBySrcRecorder) run() {
	for src := range r.ch {
		key := src.String()

		r.mx.Lock()
		if v, ok := r.mp[key]; ok {
			r.mp[key] = v + 1
		} else {
			r.mp[key] = 1
		}
		r.mx.Unlock()
	}
}

func (r *shredsBySrcRecorder) flush() *shredsBySource {
	stats := make([]*shredsBySrcRecord, 0)

	r.mx.Lock()
	for src, shreds := range r.mp {
		stats = append(stats, &shredsBySrcRecord{
			src:    src,
			shreds: shreds,
		})

		delete(r.mp, src)
	}
	r.mx.Unlock()

	sort.Slice(stats, func(i, j int) bool { return stats[i].shreds > stats[j].shreds })
	return &shredsBySource{stats: stats}
}

type shredsBySource struct {
	stats []*shredsBySrcRecord
}

func (s *shredsBySource) String() string {
	statsStr := make([]string, 0)
	for _, st := range s.stats {
		statsStr = append(statsStr, fmt.Sprintf("(%s, %d)", st.src, st.shreds))
	}

	return fmt.Sprintf("[%s]", strings.Join(statsStr, ", "))
}

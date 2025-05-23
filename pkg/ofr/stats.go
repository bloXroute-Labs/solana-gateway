package ofr

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

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

	totalShredsRecorder  *shredsBySrcRecorder
	unseenShredsRecorder *shredsBySrcRecorder
	firstShredRecorder   *firstShredRecorder

	flushUnseenShredsChan chan *ShredsBySource
	regions               sync.Map
}

type nodeInfo struct {
	addr     netip.Addr
	nodeType string
}

type (
	shredsAndInfo struct {
		shreds   int
		nodeType string
	}

	shredsBySrcRecorder struct {
		ch chan nodeInfo
		mp map[netip.Addr]*shredsAndInfo // addr: shredsInfo
		mx sync.Mutex
	}

	ShredsBySrcRecord struct {
		Src      string
		Shreds   int
		NodeType string
	}
)

type (
	firstShredRecorder struct {
		mp map[string]time.Time // slot-type: record_time
		mx sync.Mutex
		ch chan firstShredRecord
		lg logger.Logger
	}

	firstShredRecord struct {
		src      netip.Addr
		slot     uint64
		index    uint32
		typ      string
		typeByte byte
	}

	logEntry struct {
		source              string
		firstSeenShreds     int
		totalShreds         int
		firstSeenPercentage float64
		kpi                 float64
		nodeType            string
		regions             string
	}
)

func NewStats(lg logger.Logger, timeout time.Duration, opts ...StatsOption) *Stats {
	st := &Stats{
		flushTimeout: timeout,
		lg:           lg,

		totalShredsRecorder:  newShredsBySrcRecorder(),
		unseenShredsRecorder: newShredsBySrcRecorder(),
		firstShredRecorder:   newFirstShredRecorder(lg),

		flushUnseenShredsChan: make(chan *ShredsBySource),
	}

	for _, o := range opts {
		o(st)
	}

	go func() {
		now := time.Now()
		time.Sleep(time.Now().Truncate(st.flushTimeout).Add(st.flushTimeout).Sub(now))

		ticker := time.NewTicker(st.flushTimeout)
		defer ticker.Stop()

		for range ticker.C {
			totalShreds := st.totalShredsRecorder.flush()
			unseenShreds := st.unseenShredsRecorder.flush()

			select {
			case st.flushUnseenShredsChan <- unseenShreds:
			default:
			}
			st.logStats(totalShreds, unseenShreds)

			st.firstShredRecorder.clean()

			if st.fluentD != nil {
				for _, ts := range totalShreds.Stats {
					for _, us := range unseenShreds.Stats {
						if ts.Src != us.Src {
							continue
						}

						st.fluentD.LogShredStats(ts.Src, uint32(ts.Shreds), uint32(us.Shreds))
						break
					}
				}
			}
		}
	}()

	return st
}

func (s *Stats) AddRegions(addr string, regions map[string]struct{}) {
	s.regions.Store(addr, regions)
}

func (s *Stats) logStats(totalShreds *ShredsBySource, unseenShreds *ShredsBySource) {
	var totalUnseenShreds int
	logEntries := make([]logEntry, 0)
	for _, unseen := range unseenShreds.Stats {
		totalUnseenShreds += unseen.Shreds
	}

	for _, total := range totalShreds.Stats {
		unseenCount := 0

		for _, unseen := range unseenShreds.Stats {
			if unseen.Src == total.Src {
				unseenCount = unseen.Shreds
				break
			}
		}

		firstSeenPercentage := 0.0
		kpi := 0.0

		if totalUnseenShreds > 0 {
			firstSeenPercentage = (float64(unseenCount) / float64(totalUnseenShreds)) * 100
		}
		if total.Shreds > 0 {
			kpi = float64(unseenCount) / float64(total.Shreds)
		}
		logEntries = append(logEntries, logEntry{
			source: total.Src, firstSeenShreds: unseenCount, totalShreds: total.Shreds,
			firstSeenPercentage: firstSeenPercentage, kpi: kpi, nodeType: total.NodeType, regions: s.mapKeysToString(total.Src, &s.regions),
		})
	}
	sort.Slice(logEntries, func(i, j int) bool {
		return logEntries[i].firstSeenShreds > logEntries[j].firstSeenShreds
	})
	for _, entry := range logEntries {
		if entry.regions == "" {
			s.lg.Infof("%-16s %-7s first seen shreds: %-6d (%-5.2f%%) total shreds: %-6d kpi: %-4.2f", entry.source, entry.nodeType, entry.firstSeenShreds, entry.firstSeenPercentage,
				entry.totalShreds, entry.kpi)
		} else {
			s.lg.Infof("%-16s %-7s first seen shreds: %-6d (%-5.2f%%) total shreds: %-6d kpi: %-4.2f regions: %s", entry.source, entry.nodeType, entry.firstSeenShreds, entry.firstSeenPercentage,
				entry.totalShreds, entry.kpi, entry.regions)
		}
	}
}

func (s *Stats) mapKeysToString(addr string, m *sync.Map) string {
	v, ok := m.Load(addr)
	if !ok {
		return ""
	}
	regions, ok := v.(map[string]struct{})
	if !ok {
		return ""
	}
	keys := make([]string, 0, len(regions))
	for k := range regions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

type StatsOption func(*Stats)

func StatsWithFluentD(fd *FluentD) StatsOption {
	return func(s *Stats) {
		s.fluentD = fd
	}
}

func (s *Stats) RecordNewShred(src netip.Addr, nodeType string) {
	s.totalShredsRecorder.record(src, nodeType)
}

func (s *Stats) RecordUnseenShred(src netip.Addr, shred solana.PartialShred, nodeType string) {
	s.unseenShredsRecorder.record(src, nodeType)
	s.firstShredRecorder.record(src, shred)
}

func (s *Stats) RecordNewGateway(peerIP string, version string, accountID string) {
	if s.fluentD == nil {
		return
	}

	s.fluentD.LogConnectedGateway(peerIP, version, accountID)
}

func (s *Stats) RecordShredGateway(slot uint64, index uint32, variant string, source string, tm time.Time, processTime time.Duration) {
	if s.fluentD == nil {
		return
	}

	s.fluentD.LogShredGateway(slot, index, variant, source, tm, processTime)
}

func (s *Stats) RecordShredRelay(slot uint64, index uint32, variant string, source string, tm time.Time, processTime time.Duration) {
	if s.fluentD == nil {
		return
	}

	s.fluentD.LogShredRelay(slot, index, variant, source, tm, processTime)
}

// RecvFlushUnseenShreds returns a chan which drops UnseenShredsBySource when flushing for additional logging.
func (s *Stats) RecvFlushUnseenShreds() chan *ShredsBySource {
	return s.flushUnseenShredsChan
}

func newShredsBySrcRecorder() *shredsBySrcRecorder {
	rec := &shredsBySrcRecorder{
		ch: make(chan nodeInfo, shredsRecorderChanBuf),
		mp: make(map[netip.Addr]*shredsAndInfo),
	}

	go rec.run()
	return rec
}

func newFirstShredRecorder(lg logger.Logger) *firstShredRecorder {
	rec := &firstShredRecorder{
		mp: make(map[string]time.Time),
		mx: sync.Mutex{},
		ch: make(chan firstShredRecord, shredsRecorderChanBuf),
		lg: lg,
	}

	go rec.run()
	return rec
}

func (r *firstShredRecorder) record(src netip.Addr, shred solana.PartialShred) {
	select {
	case r.ch <- firstShredRecord{
		src:      src,
		slot:     shred.Slot,
		index:    shred.Index,
		typ:      shred.Variant.String(),
		typeByte: shred.Variant.Variant,
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

		key := buildKey(shred.slot, shred.typeByte)

		r.mx.Lock()
		if _, ok := r.mp[key]; !ok {
			r.lg.Debugf("stats: FIRST shred from: %s slot: %d index: %d type: %s", shred.src.String(), shred.slot, shred.index, shred.typ)
			r.mp[key] = time.Now()
		}
		r.mx.Unlock()
	}
}

func buildKey(slot uint64, typ byte) string {
	buf := make([]byte, 0, 10) // one alloc here
	buf = strconv.AppendUint(buf, slot, 10)
	buf = append(buf, typ)
	return unsafe.String(unsafe.SliceData(buf), len(buf))
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

func (r *shredsBySrcRecorder) record(src netip.Addr, nodeType string) {
	select {
	case r.ch <- nodeInfo{src, nodeType}:
	default:
	}
}

func (r *shredsBySrcRecorder) run() {
	for info := range r.ch {
		r.mx.Lock()
		if v, ok := r.mp[info.addr]; ok {
			v.shreds += 1
		} else {
			r.mp[info.addr] = &shredsAndInfo{1, info.nodeType}
		}
		r.mx.Unlock()
	}
}

func (r *shredsBySrcRecorder) flush() *ShredsBySource {
	stats := make([]*ShredsBySrcRecord, 0)

	r.mx.Lock()
	for src, info := range r.mp {
		stats = append(stats, &ShredsBySrcRecord{
			Src:      src.String(),
			Shreds:   info.shreds,
			NodeType: info.nodeType,
		})

		delete(r.mp, src)
	}
	r.mx.Unlock()

	sort.Slice(stats, func(i, j int) bool { return stats[i].Shreds > stats[j].Shreds })
	return &ShredsBySource{Stats: stats}
}

type ShredsBySource struct {
	Stats []*ShredsBySrcRecord
}

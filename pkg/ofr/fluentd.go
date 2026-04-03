package ofr

import (
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/fluent/fluent-logger-golang/fluent"
	"github.com/google/uuid"
)

const (
	fluentDTSFmt = "2006-01-02T15:04:05.000000"

	fluentDShredStatsRecordType       = "OFRPerformance"
	fluentDConnectedGatewayRecordType = "OFRConnectedGateway"
	fluentDUsageRecordType            = "OFRUsage"

	fluentDGatewayShreadPropapagationRecordType = "OFRGatewayShreadPropapagation"
	fluentDRelayShreadPropapagationRecordType   = "OFRRelayShreadPropapagation"

	fluentDShredStatsLogName       = "stats.ofr_performance"
	fluentDConnectedGatewayLogName = "stats.ofr_connected_gateway"
	fluentDShredLogName            = "stats.ofr_shred"
	fluentDUsageLogName            = "stats.ofr_usage"
)

// Record represents a bloxroute style stat type record
type Record struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type (
	FluentDLogRecord struct {
		Level     string      `json:"level"`
		Instance  string      `json:"instance"`
		Name      string      `json:"name"`
		Msg       interface{} `json:"msg"`
		Timestamp string      `json:"timestamp"`
	}

	fluentDShredStatsRecord struct {
		Type         string `json:"type"`
		PeerIP       string `json:"peer_ip"`
		PeerType     string `json:"peer_type"`
		TotalShreds  uint32 `json:"total_shreds"`
		UnseenShreds uint32 `json:"unseen_shreds"`
	}

	fluentDConnectedGatewayStatsRecord struct {
		PeerIP    string `json:"peer_ip"`
		Version   string `json:"version"`
		AccountId string `json:"account_id"`
	}

	fluentDUsageStatsRecord struct {
		AccountID       string `json:"account_id"`
		NumGateways     uint32 `json:"num_gateways"`
		NumShredstreams uint32 `json:"num_shred_streams"`
		NumTxStreams    uint32 `json:"num_tx_streams"`
	}

	// todo: having single package for gateway and relay stats is annoying to maintain,
	// we need to consider moving to separate internal packages

	fluentDShredRecordGateway struct {
		Slot        uint64        `json:"slot"`
		Index       uint32        `json:"index"`
		Variant     string        `json:"variant"`
		Source      string        `json:"source"`
		ReceiveTime time.Time     `json:"receive_time"`
		ProcessTime time.Duration `json:"process_time"`
	}

	fluentDShredRecordRelay struct {
		Slot        uint64        `json:"slot"`
		Index       uint32        `json:"index"`
		Variant     string        `json:"variant"`
		Source      string        `json:"source"`
		SourceType  string        `json:"source_type"`
		ReceiveTime time.Time     `json:"receive_time"`
		ProcessTime time.Duration `json:"process_time"`
	}
)

type FluentD struct {
	FluentD  *fluent.Fluent
	Instance string
	lg       logger.Logger
}

func NewFluentD(lg logger.Logger, host string, port int) (*FluentD, error) {
	cfg := fluent.Config{
		FluentHost:    host,
		FluentPort:    port,
		MarshalAsJSON: true,
		Async:         true,
	}

	f, err := fluent.New(cfg)
	if err != nil {
		return nil, err
	}

	fd := &FluentD{
		FluentD:  f,
		Instance: uuid.New().String(),
		lg:       lg,
	}

	return fd, nil
}

func (f *FluentD) log(record interface{}, t time.Time, logName string) {
	d := FluentDLogRecord{
		Level:     "STATS",
		Instance:  f.Instance,
		Name:      logName,
		Msg:       record,
		Timestamp: t.Format(fluentDTSFmt),
	}

	err := f.FluentD.EncodeAndPostData("bx.solana-bdn.go.log", t, d)
	if err != nil {
		f.lg.Errorf("failed to send stats to fluentd: %s", err)
	}
}

func (f *FluentD) LogShredStats(
	peerIP string,
	peerType string,
	totalShreds uint32,
	unseenShreds uint32,
) {
	record := &fluentDShredStatsRecord{
		Type:         fluentDShredStatsRecordType,
		PeerIP:       peerIP,
		PeerType:     peerType,
		TotalShreds:  totalShreds,
		UnseenShreds: unseenShreds,
	}

	f.log(record, time.Now(), fluentDShredStatsLogName)
}

func (f *FluentD) LogUsageStats(
	accountID string,
	numGateways int,
	numShredStreams int,
	numTxStreams int,
) {
	record := Record{
		Type: fluentDUsageRecordType,
		Data: fluentDUsageStatsRecord{
			AccountID:       accountID,
			NumGateways:     uint32(numGateways),
			NumShredstreams: uint32(numShredStreams),
			NumTxStreams:    uint32(numTxStreams),
		},
	}

	f.log(record, time.Now(), fluentDUsageLogName)
}

func (f *FluentD) LogConnectedGateway(
	peerIP string,
	version string,
	accountID string,
) {
	record := Record{
		Type: fluentDConnectedGatewayRecordType,
		Data: fluentDConnectedGatewayStatsRecord{
			PeerIP:    peerIP,
			Version:   version,
			AccountId: accountID,
		},
	}

	f.log(record, time.Now(), fluentDConnectedGatewayLogName)
}

func (f *FluentD) LogShredGateway(slot uint64, index uint32, variant string, source string, tm time.Time, processTime time.Duration) {
	record := Record{
		Type: fluentDGatewayShreadPropapagationRecordType,
		Data: fluentDShredRecordGateway{
			Slot:        slot,
			Index:       index,
			Variant:     variant,
			Source:      source,
			ReceiveTime: tm,
			ProcessTime: processTime,
		},
	}

	f.log(record, time.Now(), fluentDShredLogName)
}

func (f *FluentD) LogShredRelay(slot uint64, index uint32, variant string, source string, sourceType string, tm time.Time, processTime time.Duration) {
	record := Record{
		Type: fluentDRelayShreadPropapagationRecordType,
		Data: fluentDShredRecordRelay{
			Slot:        slot,
			Index:       index,
			Variant:     variant,
			Source:      source,
			SourceType:  sourceType,
			ReceiveTime: tm,
			ProcessTime: processTime,
		},
	}

	f.log(record, time.Now(), fluentDShredLogName)
}

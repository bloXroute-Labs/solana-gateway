package bdn

import (
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/fluent/fluent-logger-golang/fluent"
	"github.com/google/uuid"
)

const (
	fluentDTSFmt = "2006-01-02T15:04:05.000000"

	fluentDShredStatsRecordType = "SolanaBDNPerformance"
	fluentDShredStatsLogName    = "stats.solana_bdn_performance"
)

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
		TotalShreds  uint32 `json:"total_shreds"`
		UnseenShreds uint32 `json:"unseen_shreds"`
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
	totalShreds uint32,
	unseenShreds uint32,
) {
	record := &fluentDShredStatsRecord{
		Type:         fluentDShredStatsRecordType,
		PeerIP:       peerIP,
		TotalShreds:  totalShreds,
		UnseenShreds: unseenShreds,
	}

	f.log(record, time.Now(), fluentDShredStatsLogName)
}

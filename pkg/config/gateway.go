package config

import "encoding/json"

type Gateway struct {
	RuntimeEnnvironment       RuntimeEnvironment `json:"runtime_environment"`
	LogLevel                  string             `json:"log_level"`
	LogFileLevel              string             `json:"log_file_level"`
	LogMaxSize                int                `json:"log_max_size"`
	LogMaxBackups             int                `json:"log_max_backups"`
	LogMaxAge                 int                `json:"log_max_age"`
	SolanaTVUBroadcastPort    int                `json:"solana_tvu_broadcast_port"`
	SolanaTVUPort             int                `json:"solana_tvu_port"`
	SniffInterface            string             `json:"sniff_interface"`
	OfrHost                   string             `json:"ofr_host"`
	OfrGRPCPort               int                `json:"ofr_grpc_port"`
	UdpServerPort             int                `json:"udp_server_port"`
	AuthHeader                string             `json:"-"`
	ExtraBroadcastAddrs       []string           `json:"extra_broadcast_addrs"`
	ExtraBroadcastFromOFROnly bool               `json:"extra_broadcast_from_ofr_only"`
	NoValidator               bool               `json:"no_validator"`
	SubmissionOnly            bool               `json:"submission_only"`
	StakedNode                bool               `json:"staked_node"`
	RunHttpServer             bool               `json:"run_http_server"`
	HttpPort                  int                `json:"http_port"`
	DynamicPortRangeString    string             `json:"dynamic_port_range_string"`
	LogFluentd                bool               `json:"log_fluentd"`
	LogFluentdHost            string             `json:"log_fluentd_host"`
	FiredancerMode            bool               `json:"firedancer_mode"`
}

type RuntimeEnvironment struct {
	Version    string `json:"version"`
	ServerPort int64  `json:"server_port"`
	Arguments  string `json:"arguments"`
}

func (g *Gateway) Marshal() ([]byte, error) {
	return json.Marshal(g)
}

func (g *Gateway) Unmarshal(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, g)
}

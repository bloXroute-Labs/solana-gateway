package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestGatewayMarshal(t *testing.T) {
	g := &Gateway{
		LogLevel:                  "info",
		LogFileLevel:              "warn",
		LogMaxSize:                10,
		LogMaxBackups:             5,
		LogMaxAge:                 7,
		SolanaTVUBroadcastPort:    8001,
		SolanaTVUPort:             8002,
		SniffInterface:            "eth0",
		OfrHost:                   "localhost",
		OfrGRPCPort:               9000,
		UdpServerPort:             10000,
		AuthHeader:                "Bearer token",
		ExtraBroadcastAddrs:       []string{"127.0.0.1"},
		ExtraBroadcastFromOFROnly: true,
		NoValidator:               false,
		SubmissionOnly:            true,
		StakedNode:                false,
		RunHttpServer:             true,
		HttpPort:                  8080,
		DynamicPortRangeString:    "10000-10100",
		LogFluentd:                true,
		LogFluentdHost:            "fluentd.local",
	}

	data, err := g.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var g2 Gateway
	if err := json.Unmarshal(data, &g2); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if g2.AuthHeader != "" {
		t.Errorf("Unmarshaled Gateway AuthHeader should be empty, got: %s", g.AuthHeader)
	}

	// ensure we perform deep equal against sanitized struct
	g.AuthHeader = ""
	if !reflect.DeepEqual(*g, g2) {
		t.Errorf("Unmarshaled Gateway does not match original.\nGot: %+v\nWant: %+v", g2, *g)
	}
}

func TestGatewayUnmarshal(t *testing.T) {
	orig := Gateway{
		LogLevel:                  "debug",
		LogFileLevel:              "error",
		LogMaxSize:                20,
		LogMaxBackups:             2,
		LogMaxAge:                 3,
		SolanaTVUBroadcastPort:    9001,
		SolanaTVUPort:             9002,
		SniffInterface:            "eth1",
		OfrHost:                   "remotehost",
		OfrGRPCPort:               9100,
		UdpServerPort:             11000,
		AuthHeader:                "ApiKey xyz",
		ExtraBroadcastAddrs:       []string{"10.0.0.1", "10.0.0.2"},
		ExtraBroadcastFromOFROnly: false,
		NoValidator:               true,
		SubmissionOnly:            false,
		StakedNode:                true,
		RunHttpServer:             false,
		HttpPort:                  9090,
		DynamicPortRangeString:    "11000-11100",
		LogFluentd:                false,
		LogFluentdHost:            "fluentd.remote",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var g Gateway
	if err := g.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if g.AuthHeader != "" {
		t.Errorf("Unmarshaled Gateway AuthHeader should be empty, got: %s", g.AuthHeader)
	}

	// ensure we perform deep equal against sanitized struct
	orig.AuthHeader = ""
	if !reflect.DeepEqual(orig, g) {
		t.Errorf("Unmarshaled Gateway does not match original.\nGot: %+v\nWant: %+v", g, orig)
	}
}

func TestGatewayUnmarshalPtr(t *testing.T) {
	orig := Gateway{
		LogLevel:                  "debug",
		LogFileLevel:              "error",
		LogMaxSize:                20,
		LogMaxBackups:             2,
		LogMaxAge:                 3,
		SolanaTVUBroadcastPort:    9001,
		SolanaTVUPort:             9002,
		SniffInterface:            "eth1",
		OfrHost:                   "remotehost",
		OfrGRPCPort:               9100,
		UdpServerPort:             11000,
		AuthHeader:                "ApiKey xyz",
		ExtraBroadcastAddrs:       []string{"10.0.0.1", "10.0.0.2"},
		ExtraBroadcastFromOFROnly: false,
		NoValidator:               true,
		SubmissionOnly:            false,
		StakedNode:                true,
		RunHttpServer:             false,
		HttpPort:                  9090,
		DynamicPortRangeString:    "11000-11100",
		LogFluentd:                false,
		LogFluentdHost:            "fluentd.remote",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	g := &Gateway{}
	if err := g.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if g.AuthHeader != "" {
		t.Errorf("Unmarshaled Gateway AuthHeader should be empty, got: %s", g.AuthHeader)
	}

	// ensure we perform deep equal against sanitized struct
	orig.AuthHeader = ""
	if !reflect.DeepEqual(orig, *g) {
		t.Errorf("Unmarshaled Gateway does not match original.\nGot: %+v\nWant: %+v", g, orig)
	}
}

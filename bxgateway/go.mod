module github.com/bloXroute-Labs/solana-gateway/bxgateway

go 1.21

replace github.com/bloXroute-Labs/solana-gateway/pkg => ../pkg

require (
	github.com/bloXroute-Labs/solana-gateway/pkg v0.0.0-00010101000000-000000000000
	github.com/google/gopacket v1.1.19
	google.golang.org/grpc v1.60.1
)

require (
	github.com/bloXroute-Labs/gateway/v2 v2.128.86 // indirect
	github.com/evalphobia/logrus_fluent v0.5.4 // indirect
	github.com/fluent/fluent-logger-golang v1.9.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/orandin/lumberjackrus v1.0.1 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/tinylib/msgp v1.1.9 // indirect
	golang.org/x/net v0.20.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240108191215-35c7eff3a6b1 // indirect
	google.golang.org/protobuf v1.32.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

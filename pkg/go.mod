module github.com/bloXroute-Labs/solana-gateway/pkg

go 1.23.3

require (
	github.com/bloXroute-Labs/gateway/v2 v2.129.74
	github.com/fluent/fluent-logger-golang v1.9.0
	github.com/google/gopacket v1.1.19
	github.com/google/uuid v1.6.0
	github.com/slack-go/slack v0.16.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.5
)

replace google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 => google.golang.org/grpc v1.70.0

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/tinylib/msgp v1.2.4 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250219182151-9fdb1cabc7b2 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

module github.com/bloXroute-Labs/solana-gateway/bxgateway

go 1.23

replace github.com/bloXroute-Labs/solana-gateway/pkg => ../pkg

require (
	github.com/bloXroute-Labs/solana-gateway/pkg v0.0.0-00010101000000-000000000000
	github.com/google/gopacket v1.1.19
	github.com/urfave/cli/v2 v2.27.5
	google.golang.org/grpc v1.68.0
)

require (
	github.com/bloXroute-Labs/gateway/v2 v2.129.47 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/fluent/fluent-logger-golang v1.9.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/tinylib/msgp v1.2.4 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	golang.org/x/net v0.31.0 // indirect
	golang.org/x/sys v0.27.0 // indirect
	golang.org/x/text v0.20.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241104194629-dd2ea8efbc28 // indirect
	google.golang.org/protobuf v1.35.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

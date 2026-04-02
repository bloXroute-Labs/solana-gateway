module github.com/bloXroute-Labs/solana-gateway/bxgateway

go 1.25.1

replace github.com/bloXroute-Labs/solana-gateway/pkg => ../pkg

require (
	github.com/bloXroute-Labs/solana-gateway/pkg v0.0.0-00010101000000-000000000000
	github.com/cornelk/hashmap v1.0.8
	github.com/google/gopacket v1.1.19
	github.com/stretchr/testify v1.11.1
	github.com/urfave/cli/v2 v2.27.7
	golang.org/x/net v0.49.0
	google.golang.org/grpc v1.79.3
)

require (
	github.com/HdrHistogram/hdrhistogram-go v1.2.0 // indirect
	github.com/bloXroute-Labs/bxcommon-go v1.2.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fluent/fluent-logger-golang v1.10.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/tinylib/msgp v1.4.0 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

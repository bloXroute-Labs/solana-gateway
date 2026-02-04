module github.com/bloXroute-Labs/solana-gateway/pkg

go 1.25.1

require (
	github.com/HdrHistogram/hdrhistogram-go v1.2.0
	github.com/bloXroute-Labs/bxcommon-go v1.1.9
	github.com/fluent/fluent-logger-golang v1.10.1
	github.com/google/gopacket v1.1.19
	github.com/google/uuid v1.6.0
	github.com/stretchr/testify v1.11.1
	golang.org/x/net v0.49.0
	golang.org/x/sys v0.40.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/tinylib/msgp v1.4.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// This explicitly replaces the old genproto with the new one to avoid ambiguous imports
replace google.golang.org/genproto => google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda

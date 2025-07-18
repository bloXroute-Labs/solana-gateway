module github.com/bloXroute-Labs/solana-gateway/pkg

go 1.24.2

require (
	github.com/bloXroute-Labs/bxcommon-go v1.0.3
	github.com/fluent/fluent-logger-golang v1.9.0
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/google/gopacket v1.1.19
	github.com/google/uuid v1.6.0
	github.com/stretchr/testify v1.10.0
	golang.org/x/net v0.39.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/tinylib/msgp v1.2.5 // indirect
	golang.org/x/sys v0.32.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// This explicitly replaces the old genproto with the new one to avoid ambiguous imports
replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20230526203410-71b5a4ffd15e

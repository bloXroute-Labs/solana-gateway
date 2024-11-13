package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/netlisten"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/gateway"
	"github.com/bloXroute-Labs/solana-gateway/pkg/bdn"
	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	pb "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
	"github.com/bloXroute-Labs/solana-gateway/pkg/udp"
)

const (
	appName        = "solana-gateway"
	defaultVersion = "0.2.0h"
	localhost      = "127.0.0.1"

	// environment variable to run gateway in passive mode
	//
	// "passive mode" means to be passive in relation to the solana-validator
	// it only forwards [Solana -> BDN] traffic, but not [BDN -> Solana]
	passiveModeEnv    = "SG_MODE_PASSIVE"
	gatewayVersionEnv = "SG_VERSION"
)

const (
	logLevelFlag       = "log-level"
	logFileLevelFlag   = "log-file-level"
	logMaxSizeFlag     = "log-max-size"
	logMaxBackupsFlag  = "log-max-backups"
	logMaxAgeFlag      = "log-max-age"
	solanaTVUPortFlag  = "tvu-port"
	sniffInterfaceFlag = "network-interface"
	bdnHostFlag        = "bdn-host"
	bdnPortFlag        = "bdn-port"
	bdnGRPCPortFlag    = "bdn-grpc-port"
	udpServerPortFlag  = "port"
	authHeaderFlag     = "auth-header"
)

func main() {
	var app = cli.App{
		Name: "solana-gateway",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: logLevelFlag, Value: "info", Usage: "stdout log level"},
			&cli.StringFlag{Name: logFileLevelFlag, Value: "debug", Usage: "logfile log level"},
			&cli.IntFlag{Name: logMaxSizeFlag, Value: 100, Usage: "max logfile size MB"},
			&cli.IntFlag{Name: logMaxBackupsFlag, Value: 10, Usage: "max logfile backups"},
			&cli.IntFlag{Name: logMaxAgeFlag, Value: 10, Usage: "logfile max age"},
			&cli.IntFlag{Name: solanaTVUPortFlag, Value: 8001, Usage: "Solana Validator TVU Port"},
			&cli.StringFlag{Name: sniffInterfaceFlag, Required: true, Usage: "Outbound network interface"},
			&cli.StringFlag{Name: bdnHostFlag, Required: true, Usage: "Closest bdn relay's host, see https://docs.bloxroute.com/solana/solana-bdn/startup-arguments"},
			&cli.IntFlag{Name: bdnPortFlag, Value: 8888, Usage: "Closest bdn relay's UDP port"},
			&cli.IntFlag{Name: bdnGRPCPortFlag, Value: 5005, Usage: "Closest bdn relay's GRPC port"},
			&cli.IntFlag{Name: udpServerPortFlag, Value: 18888, Usage: "Localhost UDP port used to run a server for communication with bdn - should be open for inbound and outbound traffic"},
			&cli.StringFlag{Name: authHeaderFlag, Required: true, Usage: "Auth header issued by bloXroute"},
		},
		Action: func(c *cli.Context) error {
			run(
				c.String(logLevelFlag),
				c.String(logFileLevelFlag),
				c.Int(logMaxSizeFlag),
				c.Int(logMaxBackupsFlag),
				c.Int(logMaxAgeFlag),
				c.Int(solanaTVUPortFlag),
				c.String(sniffInterfaceFlag),
				c.String(bdnHostFlag),
				c.Int(bdnPortFlag),
				c.Int(bdnGRPCPortFlag),
				c.Int(udpServerPortFlag),
				c.String(authHeaderFlag),
			)

			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalln("run solana-gateway:", err)
	}
}

func run(
	logLevel string,
	logFileLevel string,
	logMaxSize int,
	logMaxBackups int,
	logMaxAge int,
	solanaTVUPort int,
	sniffInterface string,
	bdnHost string,
	bdnUDPPort int,
	bdnGRPCPort int,
	udpServerPort int,
	authHeader string,
) {
	var version = os.Getenv(gatewayVersionEnv)
	if version == "" {
		version = defaultVersion
	}

	lg, closeLogger, err := logger.New(&logger.Config{
		AppName:    appName,
		Level:      logLevel,
		FileLevel:  logFileLevel,
		MaxSize:    logMaxSize,
		MaxBackups: logMaxBackups,
		MaxAge:     logMaxAge,
		Port:       udpServerPort,
		Version:    version,
		Fluentd:    false,
	})
	if err != nil {
		log.Fatalln("init service logger:", err)
	}

	var (
		ctx, cancel = context.WithCancel(context.Background())
		sig         = make(chan os.Signal)
	)

	signal.Notify(sig, os.Interrupt)

	server, err := udp.NewServer(udpServerPort)
	if err != nil {
		lg.Errorf("init udp server: %s", err)
		closeLogger()
	}

	gatewayModePassive := os.Getenv(passiveModeEnv) != ""

	// set gateway options
	var gwOpts = make([]gateway.Option, 0)
	if gatewayModePassive {
		gwOpts = append(gwOpts, gateway.PassiveMode())
		lg.Warn("gateway is running in passive mode (packets from BDN are not forwarded to validator)")
	}

	solanaAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", localhost, solanaTVUPort))
	if err != nil {
		lg.Errorf("resolve solana udp addr: %s", err)
		closeLogger()
	}

	var alterKeyCache = cache.NewAlterKey(time.Second * 5)
	var stats = bdn.NewStats(lg, time.Minute)

	nl, err := netlisten.NewNetworkListener(ctx, lg, alterKeyCache, stats, sniffInterface, []int{solanaTVUPort})
	if err != nil {
		lg.Errorf("init network listener: %s", err)
		closeLogger()
	}

	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", bdnHost, bdnGRPCPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		lg.Errorf("grpc dial: %s", err)
		closeLogger()
	}

	registrar := gateway.NewBDNRegistrar(ctx, pb.NewRelayClient(conn), authHeader, version)

	gw, err := gateway.New(ctx, lg, alterKeyCache, server, solanaAddr, bdnHost, bdnUDPPort, stats, nl, registrar, gwOpts...)
	if err != nil {
		lg.Errorf("init gateway: %s", err)
		closeLogger()
	}

	gw.Start()

	<-sig
	lg.Info("main: received interrupt signal")
	cancel()
	<-time.After(time.Millisecond)
}

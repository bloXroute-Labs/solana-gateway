package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/netlisten"
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
	appName   = "solana-gateway"
	version   = "0.2.0"
	localhost = "127.0.0.1"

	// environment variable to run gateway in passive mode
	//
	// "passive mode" means to be passive in relation to the solana-validator
	// it only forwards [Solana -> BDN] traffic, but not [BDN -> Solana]
	passiveModeEnv = "SG_MODE_PASSIVE"
)

func main() {
	var (
		logLevel      string
		logFileLevel  string
		logMaxSize    int
		logMaxBackups int
		logMaxAge     int

		solanaTVUPortArg  int
		sniffInterfaceArg string
		bdnHostArg        string
		bdnPortArg        int
		bdnGRPCPortArg    int
		udpServerPortArg  int
		authHeaderArg     string
	)

	flag.StringVar(&logLevel, "log-level", "info", "stdout log level")
	flag.StringVar(&logFileLevel, "log-file-level", "debug", "logfile log level")
	flag.IntVar(&logMaxSize, "log-max-size", 100, "max logfile size MB")
	flag.IntVar(&logMaxBackups, "log-max-backups", 10, "max logfile backups")
	flag.IntVar(&logMaxAge, "log-max-age", 10, "logfile max age")

	flag.IntVar(&solanaTVUPortArg, "tvu-port", 8001, "specifies Solana Validator TVU Port")
	flag.StringVar(&sniffInterfaceArg, "network-interface", "", "outbound network interface")
	flag.StringVar(&bdnHostArg, "bdn-host", "", "specifies host of closest relay of the bdn")
	flag.IntVar(&bdnPortArg, "bdn-port", 8888, "specifies port of closest relay of the bdn")
	flag.IntVar(&bdnGRPCPortArg, "bdn-grpc-port", 5001, "specifies grpc port of closest relay of the bdn")
	flag.IntVar(&udpServerPortArg, "port", 18888, "UDP port used to run a server for communication with bdn - should be open for inbound and outbound traffic")
	flag.StringVar(&authHeaderArg, "auth-header", "", "auth header issued by bloxroute")

	flag.Parse()

	lg, err := logger.New(&logger.Config{
		AppName:    appName,
		Level:      logLevel,
		FileLevel:  logFileLevel,
		MaxSize:    logMaxSize,
		MaxBackups: logMaxBackups,
		MaxAge:     logMaxAge,
		Port:       udpServerPortArg,
		Version:    version,
		Fluentd:    false,
	})
	if err != nil {
		log.Fatalln("init service logger:", err)
	}

	if authHeaderArg == "" {
		lg.Error("auth header is required")
		lg.Exit(1)
	}

	if bdnHostArg == "" {
		lg.Error("bdn host is required")
		lg.Exit(1)
	}

	var (
		ctx, cancel = context.WithCancel(context.Background())
		sig         = make(chan os.Signal)
	)

	signal.Notify(sig, os.Interrupt)

	server, err := udp.NewServer(udpServerPortArg)
	if err != nil {
		lg.Errorf("init udp server: %s", err)
		lg.Exit(1)
	}

	gatewayModePassive := os.Getenv(passiveModeEnv) != ""

	// set gateway options
	var gwOpts = make([]gateway.Option, 0)
	if gatewayModePassive {
		gwOpts = append(gwOpts, gateway.PassiveMode())
		lg.Warn("gateway is running in passive mode (packets from BDN are not forwarded to validator)")
	}

	solanaAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", localhost, solanaTVUPortArg))
	if err != nil {
		lg.Errorf("resolve solana udp addr: %s", err)
		lg.Exit(1)
	}

	bdnAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", bdnHostArg, bdnPortArg))
	if err != nil {
		lg.Errorf("resolve solana udp addr: %s", err)
		lg.Exit(1)
	}

	var alterKeyCache = cache.NewAlterKey(time.Second * 5)
	var stats = bdn.NewStats(lg, time.Minute)

	nl, err := netlisten.NewNetworkListener(ctx, lg, alterKeyCache, stats, sniffInterfaceArg, []int{solanaTVUPortArg})
	if err != nil {
		lg.Errorf("init network listener: %s", err)
		lg.Exit(1)
	}

	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", bdnHostArg, bdnGRPCPortArg), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		lg.Errorf("grpc dial: %s", err)
		lg.Exit(1)
	}
	client := pb.NewRelayClient(conn)
	bdnRegister := func() error {
		_, err := client.Register(context.Background(), &pb.RegisterRequest{
			AuthHeader: authHeaderArg,
		})

		return err
	}

	gateway.New(ctx, lg, alterKeyCache, server, solanaAddr, bdnAddr, stats, nl, bdnRegister, gwOpts...).Start()

	<-sig
	lg.Info("main: received interrupt signal")
	cancel()
	<-time.After(time.Millisecond)
}

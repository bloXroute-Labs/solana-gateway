package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/firedancer"
	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/gateway"
	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/http_server"
	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/netlisten"
	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/txfwd"
	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	"github.com/bloXroute-Labs/solana-gateway/pkg/config"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/ofr"
	pb "github.com/bloXroute-Labs/solana-gateway/pkg/protobuf"
	"github.com/bloXroute-Labs/solana-gateway/pkg/udp"
)

func init() {
	go func() { // pprof
		fmt.Println(http.ListenAndServe("localhost:8081", nil))
	}()
}

const (
	appName        = "solana-gateway"
	defaultVersion = "0.2.0h"
	localhost      = "127.0.0.1"

	gatewayVersionEnv     = "SG_VERSION"
	numOfTraderAPISToSend = 2
)

const (
	logLevelFlag               = "log-level"
	logFileLevelFlag           = "log-file-level"
	logMaxSizeFlag             = "log-max-size"
	logMaxBackupsFlag          = "log-max-backups"
	logMaxAgeFlag              = "log-max-age"
	logFluentdFlag             = "log-fluentd"
	logFluentHostFlag          = "log-fluentd-host"
	solanaTVUBroadcastPortFlag = "tvu-broadcast-port"
	solanaTVUPortFlag          = "tvu-port"
	sniffInterfaceFlag         = "network-interface"
	ofrHostFlag                = "ofr-host"
	ofrPortFlag                = "ofr-port"
	ofrGRPCPortFlag            = "ofr-grpc-port"
	udpServerPortFlag          = "port"
	authHeaderFlag             = "auth-header"
	broadcastAddressesFlag     = "broadcast-addresses"
	broadcastFromOfrOnlyFlag   = "broadcast-from-ofr-only"
	noValidatorFlag            = "no-validator"
	submissionOnlyFlag         = "tx-submission-only"
	stakedNodeFlag             = "staked-node"
	runHttpServerFlag          = "run-http-server"
	httpPortFlag               = "http-port"
	dynamicPortRangeFlag       = "dynamic-port-range"
	firedancerModeFlag         = "firedancer"
)

func main() {
	app := cli.App{
		Name: "solana-gateway",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: logLevelFlag, Value: "info", Usage: "Stdout log level"},
			&cli.StringFlag{Name: logFileLevelFlag, Value: "debug", Usage: "Logfile log level"},
			&cli.IntFlag{Name: logMaxSizeFlag, Value: 100, Usage: "Max logfile size MB"},
			&cli.IntFlag{Name: logMaxBackupsFlag, Value: 10, Usage: "Max logfile backups"},
			&cli.IntFlag{Name: logMaxAgeFlag, Value: 10, Usage: "Logfile max age"},
			&cli.BoolFlag{Name: logFluentdFlag, Value: false, Hidden: true, Usage: "enable fluentd"},
			&cli.StringFlag{Name: logFluentHostFlag, Value: "127.0.0.1", Hidden: true, Usage: "fluentd flag"},
			&cli.IntFlag{Name: solanaTVUBroadcastPortFlag, Value: 0, Usage: "Solana Validator TVU Broadcast Port"},
			&cli.IntFlag{Name: solanaTVUPortFlag, Value: 8001, Usage: "Solana Validator TVU Port"},
			&cli.StringFlag{Name: sniffInterfaceFlag, Usage: "Outbound network interface"},
			&cli.StringFlag{Name: ofrHostFlag, Aliases: []string{"bdn-host"}, Required: true, Usage: "Closest ofr relay's host, see https://docs.bloxroute.com/solana/solana-bdn/startup-arguments"},
			&cli.IntFlag{Name: ofrPortFlag, Aliases: []string{"bdn-port"}, Value: 8888, Usage: "DEPRECATED - kept to not to crash existing configurations"},
			&cli.IntFlag{Name: ofrGRPCPortFlag, Aliases: []string{"bdn-grpc-port"}, Value: 5005, Usage: "Closest ofr relay's GRPC port"},
			&cli.IntFlag{Name: udpServerPortFlag, Value: 18888, Usage: "Localhost UDP port used to run a server for communication with ofr - should be open for inbound and outbound traffic"},
			&cli.StringFlag{Name: authHeaderFlag, Required: true, Usage: "Auth header issued by bloXroute"},
			&cli.StringSliceFlag{Name: broadcastAddressesFlag, Usage: "Sets extra addresses to send shreds received from OFR and Solana Node"},
			&cli.BoolFlag{Name: broadcastFromOfrOnlyFlag, Aliases: []string{"broadcast-from-bdn-only"}, Usage: "Do not send traffic from Solana Node to extra addresses specified with --broadcast-addresses"},
			&cli.BoolFlag{Name: noValidatorFlag, Value: false, Usage: "Run gw without node, only for elite/ultra accounts"},
			&cli.BoolFlag{Name: submissionOnlyFlag, Value: false, Usage: "Disable all gw functionality other than tx submission, only for authorized accounts."},
			&cli.BoolFlag{Name: stakedNodeFlag, Value: true, Usage: "Run as a stacked node (true by default)"},
			&cli.BoolFlag{Name: runHttpServerFlag, Value: false, Usage: "Run http server to submit txs to trader api"},
			&cli.IntFlag{Name: httpPortFlag, Value: 8080, Required: false, Usage: "HTTP port for submitting txs to trader api"},
			&cli.StringFlag{Name: dynamicPortRangeFlag, Value: "18889-19888", Usage: "<MIN_PORT-MAX_PORT> Range to use for dynamically assigned ports for shreds propagation over UDP, should not conflict with solana/agave dynamic port range"},
			&cli.BoolFlag{Name: firedancerModeFlag, Value: false, Usage: "Run in firedancer mode"},
		},
		Action: func(c *cli.Context) error {
			return run(
				&config.Gateway{
					LogLevel:                  c.String(logLevelFlag),
					LogFileLevel:              c.String(logFileLevelFlag),
					LogMaxSize:                c.Int(logMaxSizeFlag),
					LogMaxBackups:             c.Int(logMaxBackupsFlag),
					LogMaxAge:                 c.Int(logMaxAgeFlag),
					SolanaTVUBroadcastPort:    c.Int(solanaTVUBroadcastPortFlag),
					SolanaTVUPort:             c.Int(solanaTVUPortFlag),
					SniffInterface:            c.String(sniffInterfaceFlag),
					OfrHost:                   c.String(ofrHostFlag),
					OfrGRPCPort:               c.Int(ofrGRPCPortFlag),
					UdpServerPort:             c.Int(udpServerPortFlag),
					AuthHeader:                c.String(authHeaderFlag),
					ExtraBroadcastAddrs:       c.StringSlice(broadcastAddressesFlag),
					ExtraBroadcastFromOFROnly: c.Bool(broadcastFromOfrOnlyFlag),
					NoValidator:               c.Bool(noValidatorFlag),
					SubmissionOnly:            c.Bool(submissionOnlyFlag),
					StakedNode:                c.Bool(stakedNodeFlag),
					RunHttpServer:             c.Bool(runHttpServerFlag),
					HttpPort:                  c.Int(httpPortFlag),
					DynamicPortRangeString:    c.String(dynamicPortRangeFlag),
					LogFluentd:                c.Bool(logFluentdFlag),
					LogFluentdHost:            c.String(logFluentHostFlag),
					FiredancerMode:            c.Bool(firedancerModeFlag),
				},
			)
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalln("run solana-gateway:", err)
	}
}

func run(
	cfg *config.Gateway,
) error {
	if !cfg.NoValidator && !cfg.SubmissionOnly && cfg.SniffInterface == "" {
		log.Fatalln("network-interface can't be empty")
	}
	version := os.Getenv(gatewayVersionEnv)
	if version == "" {
		version = defaultVersion
	}

	ports := strings.Split(cfg.DynamicPortRangeString, "-")
	dynamicPortRangeMin, err := strconv.Atoi(ports[0])
	if err != nil {
		return fmt.Errorf("convert min-port: %s", err)
	}

	dynamicPortRangeMax, err := strconv.Atoi(ports[1])
	if err != nil {
		return fmt.Errorf("convert max-port: %s", err)
	}

	if dynamicPortRangeMin < 0 || dynamicPortRangeMax < 0 {
		return errors.New("dynamic port range values cannot be lower than zero")
	}

	if dynamicPortRangeMax < dynamicPortRangeMin {
		return errors.New("dynamic port range max cannot be lower than min")
	}

	lg, closeLogger, err := logger.New(&logger.Config{
		AppName:    appName,
		Level:      cfg.LogLevel,
		FileLevel:  cfg.LogFileLevel,
		MaxSize:    cfg.LogMaxSize,
		MaxBackups: cfg.LogMaxBackups,
		MaxAge:     cfg.LogMaxAge,
		Port:       cfg.UdpServerPort,
		Version:    version,
		Fluentd:    false,
	})
	if err != nil {
		log.Fatalln("init service logger:", err)
	}

	gwopts := make([]gateway.Option, 0)

	if cfg.RunHttpServer {
		bxfwd := txfwd.NewBxForwarder(context.Background(), lg)
		traderapifwd := txfwd.NewTraderAPIForwarder(lg, numOfTraderAPISToSend, cfg.AuthHeader)
		gwopts = append(gwopts, gateway.WithTxForwarders(bxfwd, traderapifwd))
		http_server.Run(lg, cfg.HttpPort, bxfwd, traderapifwd)
	}

	defer closeLogger()

	var (
		ctx, cancel = context.WithCancel(context.Background())
		sig         = make(chan os.Signal, 1)
	)

	defer cancel()

	lg.Debugf("dynamic port range %d-%d", dynamicPortRangeMin, dynamicPortRangeMax)

	signal.Notify(sig, os.Interrupt)

	solanaNodeTVUAddrUDP, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", localhost, cfg.SolanaTVUPort))
	if err != nil {
		lg.Errorf("resolve solana udp addr: %s", err)
		return err
	}

	solanaNodeTVUAddr, err := udp.SockAddrFromNetUDPAddr(solanaNodeTVUAddrUDP)
	if err != nil {
		lg.Errorf("convert solana tvu udp addr to sockaddr: %s", err)
		return err
	}

	alterKeyCache := cache.NewAlterKey(time.Second * 5)

	ofrStatsOptions := make([]ofr.StatsOption, 0)
	if cfg.LogFluentd {
		fluentd, err := ofr.NewFluentD(lg, cfg.LogFluentdHost, 24224)
		if err != nil {
			lg.Errorf("init fluentd: %s", err)
			closeLogger()
		}

		ofrStatsOptions = append(ofrStatsOptions, ofr.StatsWithFluentD(fluentd))
		lg.Info("enabled ofr stats fluentd")
	}

	stats := ofr.NewStats(lg, time.Minute, ofrStatsOptions...)

	var nl *netlisten.NetworkListener
	// assign net listener only when running with validator
	if !cfg.NoValidator && !cfg.SubmissionOnly {
		var outPorts []int
		if cfg.StakedNode {
			// If the TVU broadcast port is not specified we start listening to
			// a range of ports where the TVU broadcast port is likely to be in.
			if cfg.SolanaTVUBroadcastPort == 0 {
				// The minimal offset of the TVU broadcast port from the TVU port.
				// This value is derived from the validator's codebase.
				const TVUBroadcastOffset = 11
				for i := range 10 {
					outPorts = append(outPorts, cfg.SolanaTVUPort+TVUBroadcastOffset+i)
				}
			} else {
				outPorts = append(outPorts, cfg.SolanaTVUBroadcastPort)
			}
		}
		nl, err = netlisten.NewNetworkListener(ctx, lg, alterKeyCache, stats, cfg.SniffInterface, []int{cfg.SolanaTVUPort}, outPorts)
		if err != nil {
			lg.Errorf("init network listener: %s", err)
			return err
		}
	}

	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", cfg.OfrHost, cfg.OfrGRPCPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		lg.Errorf("grpc dial: %s", err)
		return err
	}

	fdset := udp.NewFDSet(lg, int64(dynamicPortRangeMin), int64(dynamicPortRangeMax))
	serverFd, err := udp.Server(cfg.UdpServerPort)
	if err != nil {
		lg.Errorf("new server fd: port: %d: %s", cfg.UdpServerPort, err)
		return err
	}

	cfg.RuntimeEnnvironment = config.RuntimeEnvironment{
		Version:    version,
		ServerPort: serverFd.Port,
		Arguments:  strings.Join(os.Args[1:], " "),
	}

	// Ensure we sanitize the auth header before logging it anywhere.
	re := regexp.MustCompile(`(?mi)-{1,2}auth-header[\s=]{1}(\S*)`)
	for _, match := range re.FindAllString(cfg.RuntimeEnnvironment.Arguments, -1) {
		fmt.Println("Match:", match)
		cfg.RuntimeEnnvironment.Arguments = strings.ReplaceAll(cfg.RuntimeEnnvironment.Arguments, match, "-auth-header=REDACTED")
	}

	registrar := gateway.NewOFRRegistrar(ctx, pb.NewRelayClient(conn), cfg)

	if cfg.SubmissionOnly {
		gwopts = append(gwopts, gateway.PassiveMode())
		lg.Warn("gateway is starting in passive mode (packets from OFR are not forwarded to validator)")
	}

	if cfg.NoValidator || cfg.SubmissionOnly {
		gwopts = append(gwopts, gateway.WithoutSolanaNode())
		lg.Info("gateway is starting without solana node connection")
	}

	if len(cfg.ExtraBroadcastAddrs) != 0 {
		opt, err := gateway.WithBroadcastAddrs(cfg.ExtraBroadcastAddrs, cfg.ExtraBroadcastFromOFROnly)
		if err != nil {
			lg.Errorf("set broadcast addrs: %s", err)
			return err
		}

		gwopts = append(gwopts, opt)
		lg.Infof("gateway is starting with additional broadcast addrs: %v", cfg.ExtraBroadcastAddrs)
	}

	if cfg.FiredancerMode {
		firedancerSniffer, err := firedancer.NewFiredancerSniffer(ctx, lg, stats, alterKeyCache)
		if err != nil {
			lg.Errorf("init firedancer sniffer: %s", err)
			return err
		}
		gwopts = append(gwopts, gateway.WithFiredancerSniffer(firedancerSniffer))
	}

	gw, err := gateway.New(ctx, lg, alterKeyCache, stats, nl, serverFd, fdset, solanaNodeTVUAddr, registrar, gwopts...)
	if err != nil {
		lg.Errorf("init gateway: %s", err)
		return err
	}

	gw.Start()

	<-sig
	lg.Info("main: received interrupt signal")
	cancel()
	<-time.After(time.Millisecond)
	return nil
}

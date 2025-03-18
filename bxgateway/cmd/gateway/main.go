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
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/gateway"
	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/http_server"
	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/netlisten"
	"github.com/bloXroute-Labs/solana-gateway/pkg/bdn"
	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
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

	// environment variable to run gateway in passive mode
	//
	// "passive mode" means to be passive in relation to the solana-validator
	// it only forwards [Solana -> BDN] traffic, but not [BDN -> Solana]
	passiveModeEnv        = "SG_MODE_PASSIVE"
	gatewayVersionEnv     = "SG_VERSION"
	numOfTraderAPISToSend = 2
)

const (
	logLevelFlag               = "log-level"
	logFileLevelFlag           = "log-file-level"
	logMaxSizeFlag             = "log-max-size"
	logMaxBackupsFlag          = "log-max-backups"
	logMaxAgeFlag              = "log-max-age"
	solanaTVUBroadcastPortFlag = "tvu-broadcast-port"
	solanaTVUPortFlag          = "tvu-port"
	sniffInterfaceFlag         = "network-interface"
	bdnHostFlag                = "bdn-host"
	bdnPortFlag                = "bdn-port"
	bdnGRPCPortFlag            = "bdn-grpc-port"
	udpServerPortFlag          = "port"
	authHeaderFlag             = "auth-header"
	broadcastAddressesFlag     = "broadcast-addresses"
	broadcastFromBdnOnlyFlag   = "broadcast-from-bdn-only"
	noValidatorFlag            = "no-validator"
	stakedNodeFlag             = "staked-node"
	runHttpServerFlag          = "run-http-server"
	httpPortFlag               = "http-port"
	dynamicPortRangeFlag       = "dynamic-port-range"
)

func main() {
	var app = cli.App{
		Name: "solana-gateway",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: logLevelFlag, Value: "info", Usage: "Stdout log level"},
			&cli.StringFlag{Name: logFileLevelFlag, Value: "debug", Usage: "Logfile log level"},
			&cli.IntFlag{Name: logMaxSizeFlag, Value: 100, Usage: "Max logfile size MB"},
			&cli.IntFlag{Name: logMaxBackupsFlag, Value: 10, Usage: "Max logfile backups"},
			&cli.IntFlag{Name: logMaxAgeFlag, Value: 10, Usage: "Logfile max age"},
			&cli.IntFlag{Name: solanaTVUBroadcastPortFlag, Value: 0, Usage: "Solana Validator TVU Broadcast Port"},
			&cli.IntFlag{Name: solanaTVUPortFlag, Value: 8001, Usage: "Solana Validator TVU Port"},
			&cli.StringFlag{Name: sniffInterfaceFlag, Usage: "Outbound network interface"},
			&cli.StringFlag{Name: bdnHostFlag, Required: true, Usage: "Closest bdn relay's host, see https://docs.bloxroute.com/solana/solana-bdn/startup-arguments"},
			&cli.IntFlag{Name: bdnPortFlag, Value: 8888, Usage: "DEPRECATED - kept to not to crash existing configurations"},
			&cli.IntFlag{Name: bdnGRPCPortFlag, Value: 5005, Usage: "Closest bdn relay's GRPC port"},
			&cli.IntFlag{Name: udpServerPortFlag, Value: 18888, Usage: "Localhost UDP port used to run a server for communication with bdn - should be open for inbound and outbound traffic"},
			&cli.StringFlag{Name: authHeaderFlag, Required: true, Usage: "Auth header issued by bloXroute"},
			&cli.StringSliceFlag{Name: broadcastAddressesFlag, Usage: "Sets extra addresses to send shreds received from BDN and Solana Node"},
			&cli.BoolFlag{Name: broadcastFromBdnOnlyFlag, Usage: "Do not send traffic from Solana Node to extra addresses specified with --broadcast-addresses"},
			&cli.BoolFlag{Name: noValidatorFlag, Value: false, Usage: "Run gw without node, only for elite/ultra accounts"},
			&cli.BoolFlag{Name: stakedNodeFlag, Value: false, Usage: "Run as a stacked node"},
			&cli.BoolFlag{Name: runHttpServerFlag, Value: false, Usage: "Run http server to submit txs to trader api"},
			&cli.IntFlag{Name: httpPortFlag, Value: 8080, Required: false, Usage: "HTTP port for submitting txs to trader api"},
			&cli.StringFlag{Name: dynamicPortRangeFlag, Value: "18889-19888", Usage: "<MIN_PORT-MAX_PORT> Range to use for dynamically assigned ports for shreds propagation over UDP, should not conflict with solana/agave dynamic port range"},
		},
		Action: func(c *cli.Context) error {
			return run(
				c.String(logLevelFlag),
				c.String(logFileLevelFlag),
				c.Int(logMaxSizeFlag),
				c.Int(logMaxBackupsFlag),
				c.Int(logMaxAgeFlag),
				c.Int(solanaTVUBroadcastPortFlag),
				c.Int(solanaTVUPortFlag),
				c.String(sniffInterfaceFlag),
				c.String(bdnHostFlag),
				c.Int(bdnGRPCPortFlag),
				c.Int(udpServerPortFlag),
				c.String(authHeaderFlag),
				c.StringSlice(broadcastAddressesFlag),
				c.Bool(broadcastFromBdnOnlyFlag),
				c.Bool(noValidatorFlag),
				c.Bool(stakedNodeFlag),
				c.Bool(runHttpServerFlag),
				c.Int(httpPortFlag),
				c.String(dynamicPortRangeFlag),
			)
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
	solanaTVUBroadcastPort int,
	solanaTVUPort int,
	sniffInterface string,
	bdnHost string,
	bdnGRPCPort int,
	udpServerPort int,
	authHeader string,
	extraBroadcastAddrs []string,
	extraBroadcastFromBDNOnly bool,
	noValidator bool,
	stakedNode bool,
	runHttpServer bool,
	httpPort int,
	dynamicPortRangeString string,
) error {
	if !noValidator && sniffInterface == "" {
		log.Fatalln("network-interface can't be empty")
	}
	var version = os.Getenv(gatewayVersionEnv)
	if version == "" {
		version = defaultVersion
	}

	ports := strings.Split(dynamicPortRangeString, "-")
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
	if runHttpServer {
		httpServer := http_server.NewHTTPServer(lg, httpPort, numOfTraderAPISToSend, authHeader)
		go func() {
			err := httpServer.Start()
			if err != nil {
				lg.Errorf("error starting http server: %v", err)
			}
		}()
	}

	defer closeLogger()

	var (
		ctx, cancel        = context.WithCancel(context.Background())
		sig                = make(chan os.Signal, 1)
		gatewayModePassive = os.Getenv(passiveModeEnv) != ""
	)

	defer cancel()

	lg.Infof("dynamic port range %d-%d", dynamicPortRangeMin, dynamicPortRangeMax)

	signal.Notify(sig, os.Interrupt)

	solanaNodeTVUAddrUDP, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", localhost, solanaTVUPort))
	if err != nil {
		lg.Errorf("resolve solana udp addr: %s", err)
		return err
	}

	solanaNodeTVUAddr, err := udp.SockAddrFromNetUDPAddr(solanaNodeTVUAddrUDP)
	if err != nil {
		lg.Errorf("convert solana tvu udp addr to sockaddr: %s", err)
		return err
	}

	var alterKeyCache = cache.NewAlterKey(time.Second * 5)
	var stats = bdn.NewStats(lg, time.Minute)

	var nl *netlisten.NetworkListener
	// assign net listener only when running with validator
	if !noValidator {
		var outPorts []int
		if stakedNode {
			// If the TVU broadcast port is not specified we start listening to
			// a range of ports where the TVU broadcast port is likely to be in.
			if solanaTVUBroadcastPort == 0 {
				// The minimal offset of the TVU broadcast port from the TVU port.
				// This value is derived from the validator's codebase.
				const TVUBroadcastOffset = 11
				for i := range 10 {
					outPorts = append(outPorts, solanaTVUPort+TVUBroadcastOffset+i)
				}
			} else {
				outPorts = append(outPorts, solanaTVUBroadcastPort)
			}
		}
		nl, err = netlisten.NewNetworkListener(ctx, lg, alterKeyCache, stats, sniffInterface, []int{solanaTVUPort}, outPorts)
		if err != nil {
			lg.Errorf("init network listener: %s", err)
			return err
		}
	}

	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", bdnHost, bdnGRPCPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		lg.Errorf("grpc dial: %s", err)
		return err
	}

	fdset := udp.NewFDSet(lg, int64(dynamicPortRangeMin), int64(dynamicPortRangeMax))
	serverFd, err := udp.Server(udpServerPort)
	if err != nil {
		lg.Errorf("new server fd: port: %d: %s", udpServerPort, err)
		return err
	}

	registrar := gateway.NewBDNRegistrar(ctx, pb.NewRelayClient(conn), authHeader, version, serverFd.Port)

	// set gateway options
	var opts = make([]gateway.Option, 0)
	if gatewayModePassive {
		opts = append(opts, gateway.PassiveMode())
		lg.Warn("gateway is starting in passive mode (packets from BDN are not forwarded to validator)")
	}

	if noValidator {
		opts = append(opts, gateway.WithoutSolanaNode())
		lg.Info("gateway is starting without solana node connection")
	}

	if len(extraBroadcastAddrs) != 0 {
		opt, err := gateway.WithBroadcastAddrs(extraBroadcastAddrs, extraBroadcastFromBDNOnly)
		if err != nil {
			lg.Errorf("set broadcast addrs: %s", err)
			return err
		}

		opts = append(opts, opt)
		lg.Infof("gateway is starting with additional broadcast addrs: %v", extraBroadcastAddrs)
	}

	gw, err := gateway.New(ctx, lg, alterKeyCache, stats, nl, serverFd, fdset, solanaNodeTVUAddr, registrar, opts...)
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

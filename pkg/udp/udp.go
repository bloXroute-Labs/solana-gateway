package udp

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
)

type FDSet struct {
	lg logger.Logger

	min     int64
	max     int64
	current int64
	mx      sync.Mutex
}

func NewFDSet(lg logger.Logger, min, max int64) *FDSet {
	return &FDSet{
		lg:      lg,
		min:     min,
		max:     max,
		current: min,
		mx:      sync.Mutex{},
	}
}

// NextFD returns file descriptor bound to the next available port within min-max port range
func (f *FDSet) NextFD() (*FDConn, error) {
	f.mx.Lock()
	defer f.mx.Unlock()

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, err
	}

	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, 7500000); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("setsockopt: %s", err)
	}

	var boundPort int64
	startPort := f.current

	for i := 0; ; i++ {
		port := f.current
		if i != 0 && startPort == port {
			// we went through all available ports and found nothing
			break
		}

		err = syscall.Bind(fd, &syscall.SockaddrInet4{
			Addr: [4]byte{0, 0, 0, 0},
			Port: int(port),
		})

		switch err {
		case nil:
			boundPort = port
		default:
			f.lg.Debugf("unable to bind port: %d", port)
		}

		if port == f.max {
			f.current = f.min
		} else {
			f.current += 1
		}

		if boundPort != 0 {
			var open atomic.Bool
			open.Store(true)
			return &FDConn{
				fd:   fd,
				Port: boundPort,
				open: &open,
			}, nil
		}
	}

	syscall.Close(fd)
	return nil, fmt.Errorf("no available ports in range: %d-%d, try to extend --dynamic-port-range", f.min, f.max)
}

func Server(port int) (*FDConn, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, fmt.Errorf("socket: %s", err)
	}

	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 7500000); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("setsockopt: %s", err)
	}

	err = syscall.Bind(fd, &syscall.SockaddrInet4{
		Addr: [4]byte{0, 0, 0, 0},
		Port: port,
	})
	if err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("bind: %s", err)
	}

	var open atomic.Bool
	open.Store(true)

	return &FDConn{
		fd:   fd,
		Port: int64(port),
		open: &open,
	}, nil
}

type FDConn struct {
	fd   int
	Port int64
	open *atomic.Bool
}

func (c *FDConn) UnsafeReadFrom(b []byte) (int, Addr, error) {
	n, _, _, a, err := syscall.Recvmsg(c.fd, b, nil, 0)
	if err != nil {
		return 0, Addr{}, fmt.Errorf("recvmsg: %s", err)
	}

	addr, err := NewAddr(a)
	return n, addr, err
}

func (c *FDConn) UnsafeWrite(b []byte, socketAddr syscall.Sockaddr) error {
	if err := syscall.Sendmsg(c.fd, b, nil, socketAddr, 0); err != nil {
		return fmt.Errorf("sendmsg: %s", err)
	}

	return nil
}

func (c *FDConn) Close() {
	c.open.Store(false)
	syscall.Close(c.fd)
}

func (c *FDConn) IsOpen() bool { return c.open.Load() }

type Addr struct {
	// SockAddr is non-comparable addr + port directly used for communication via syscall library
	SockAddr syscall.Sockaddr
	// NetipAddr is comparable 24 bytes representation of ip (without the port)
	NetipAddr netip.Addr
}

func (a Addr) IsZero() bool {
	return a == Addr{}
}

func NewAddr(sa syscall.Sockaddr) (Addr, error) {
	switch addr := sa.(type) {
	case *syscall.SockaddrInet4:
		return Addr{
			SockAddr:  sa,
			NetipAddr: netip.AddrFrom4(addr.Addr),
		}, nil
	case *syscall.SockaddrInet6:
		return Addr{
			SockAddr:  sa,
			NetipAddr: netip.AddrFrom16(addr.Addr),
		}, nil
	default:
		return Addr{}, errors.New("unknown address format")
	}
}

func SockaddrPort(sa syscall.Sockaddr) int {
	if sa == nil {
		return 0
	}

	switch a := sa.(type) {
	case *syscall.SockaddrInet4:
		return a.Port
	case *syscall.SockaddrInet6:
		return a.Port
	default:
		return 0
	}
}

// EqualSockaddrs compares two syscall.Sockaddr implementations
// also returns false if any or both addrs are nil
func EqualSockaddrs(a1, a2 syscall.Sockaddr) bool {
	if a1 == nil || a2 == nil {
		return false
	}

	switch addr1 := a1.(type) {
	case *syscall.SockaddrInet4:
		switch addr2 := a2.(type) {
		case *syscall.SockaddrInet4:
			return addr1.Addr == addr2.Addr && addr1.Port == addr2.Port
		default:
			return false
		}
	case *syscall.SockaddrInet6:
		switch addr2 := a2.(type) {
		case *syscall.SockaddrInet6:
			return addr1.Addr == addr2.Addr && addr1.Port == addr2.Port
		default:
			return false
		}
	default:
		return false
	}
}

// EqualSockaddrsIPs compares two syscall.Sockaddr implementations
// returns true/false if IPs are equal without comparing ports
func EqualSockaddrsIPs(a1, a2 syscall.Sockaddr) bool {
	if a1 == nil || a2 == nil {
		return false
	}

	switch addr1 := a1.(type) {
	case *syscall.SockaddrInet4:
		switch addr2 := a2.(type) {
		case *syscall.SockaddrInet4:
			return addr1.Addr == addr2.Addr
		default:
			return false
		}
	case *syscall.SockaddrInet6:
		switch addr2 := a2.(type) {
		case *syscall.SockaddrInet6:
			return addr1.Addr == addr2.Addr
		default:
			return false
		}
	default:
		return false
	}
}

// SockaddrString allocates string, so should be used carefully
func SockaddrString(sa syscall.Sockaddr) string {
	if sa == nil {
		return ""
	}

	switch a := sa.(type) {
	case *syscall.SockaddrInet4:
		addr := netip.AddrFrom4(a.Addr)
		return fmt.Sprintf("%s:%d", addr, a.Port)
	case *syscall.SockaddrInet6:
		addr := netip.AddrFrom16(a.Addr)
		return fmt.Sprintf("%s:%d", addr, a.Port)
	default:
		return ""
	}
}

func SockAddrFromUDPString(addr string) (syscall.Sockaddr, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	return SockAddrFromNetUDPAddr(udpAddr)
}

func SockAddrFromNetUDPAddr(addr *net.UDPAddr) (syscall.Sockaddr, error) {
	ip := addr.IP.To4()
	return &syscall.SockaddrInet4{Port: addr.Port, Addr: [4]byte{ip[0], ip[1], ip[2], ip[3]}}, nil
}

func SockAddrFromNetipAddrPort(addr netip.AddrPort) (syscall.Sockaddr, error) {
	ip := addr.Addr().As4()
	return &syscall.SockaddrInet4{Port: int(addr.Port()), Addr: [4]byte{ip[0], ip[1], ip[2], ip[3]}}, nil
}

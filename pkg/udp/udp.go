package udp

import (
	"fmt"
	"net"
	"sync/atomic"
	"syscall"
)

var portIter int64

const (
	local    = "0.0.0.0"
	portBase = 8000
)

func NewServer(port int) (*net.UDPConn, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP(local),
		Port: port,
	})
	if err != nil {
		return nil, err
	}

	err = conn.SetReadBuffer(10 * 1024 * 1024)
	if err != nil {
		return nil, err
	}

	err = conn.SetWriteBuffer(10 * 1024 * 1024)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func Server(port int) (*Conn, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, fmt.Errorf("socket: %s", err)
	}

	err = syscall.Bind(fd, &syscall.SockaddrInet4{
		Addr: [4]byte{0, 0, 0, 0},
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("bind: %s", err)
	}

	return &Conn{fd}, nil
}

func Dail(raddr *net.UDPAddr) (*Conn, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, err
	}

	// Ensure the socket is closed on error
	defer func() {
		if err != nil {
			syscall.Close(fd)
		}
	}()

	for i := 0; i < 100; i++ {
		err = syscall.Bind(fd, &syscall.SockaddrInet4{
			Addr: [4]byte{0, 0, 0, 0},
			Port: int(portBase + atomic.AddInt64(&portIter, 1)%1000),
		})

		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("bind: %s", err)
	}

	addr := udpAddrToSockaddr(raddr)

	err = syscall.Connect(fd, addr)
	if err != nil {
		return nil, fmt.Errorf("connect: %s", err)
	}

	return &Conn{fd}, nil
}

type Conn struct {
	fd int
}

func (c *Conn) UnsafeReadFrom(b []byte) (int, *net.UDPAddr, error) {
	n, _, _, a, err := syscall.Recvmsg(c.fd, b, nil, 0)
	if err != nil {
		return 0, nil, fmt.Errorf("recvmsg: %s", err)
	}

	addr, err := sockaddrToUDPAddr(a)
	if err != nil {
		return 0, nil, fmt.Errorf("sockaddr to udp addr: %s", err)
	}

	return n, addr, nil
}

func (c *Conn) UnsafeWrite(b []byte, addr *net.UDPAddr) error {
	if err := syscall.Sendmsg(c.fd, b, nil, udpAddrToSockaddr(addr), 0); err != nil {
		return fmt.Errorf("sendmsg: %s", err)
	}

	return nil
}

func (c *Conn) Close() {
	syscall.Close(c.fd)
}

func udpAddrToSockaddr(addr *net.UDPAddr) *syscall.SockaddrInet4 {
	return &syscall.SockaddrInet4{Port: addr.Port, Addr: [4]byte{addr.IP[12], addr.IP[13], addr.IP[14], addr.IP[15]}}
}

func sockaddrToUDPAddr(sa syscall.Sockaddr) (*net.UDPAddr, error) {
	switch addr := sa.(type) {
	case *syscall.SockaddrInet4:
		return &net.UDPAddr{
			IP:   net.IPv4(addr.Addr[0], addr.Addr[1], addr.Addr[2], addr.Addr[3]),
			Port: addr.Port,
		}, nil
	case *syscall.SockaddrInet6:
		return &net.UDPAddr{
			IP:   net.IP(addr.Addr[:]),
			Port: addr.Port,
			Zone: fmt.Sprintf("%d", addr.ZoneId),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported sockaddr type %T", sa)
	}
}

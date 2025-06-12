package udp

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
	"golang.org/x/net/ipv4"
)

// pool of *Packets, each pre-alloc’d with batchSize slots & 1500-byte buffers.
// A batch is recycled when all its Packets have been freed.
var readBatchPool = sync.Pool{
	New: func() any {
		msgs := make([]ipv4.Message, batchSize)
		for i := range msgs {
			buf := make([]byte, solana.UDPShredSize)
			msgs[i].Buffers = [][]byte{buf}
		}
		return &Packets{msgs: msgs}
	},
}

type Conn struct {
	lg           logger.Logger
	conn         *ipv4.PacketConn
	logReadTimer *time.Ticker
}

type UDPAddr struct {
	UDPAddr   net.UDPAddr
	NetipAddr netip.Addr
}

// ListenUDPInRange will try to bind a UDP socket on any port
// between startPort and endPort (inclusive) and return the first
// successful one (and the port it chose).
func ListenUDPInRange(startPort, endPort int) (*net.UDPConn, int, error) {
	if startPort < 1 || endPort > 0xFFFF || startPort > endPort {
		return nil, 0, fmt.Errorf("invalid port range %d–%d", startPort, endPort)
	}

	// optional: shuffle the slice so you don't always pick low numbers first
	ports := make([]int, endPort-startPort+1)
	for i := range ports {
		ports[i] = startPort + i
	}

	for _, p := range ports {
		addr := &net.UDPAddr{IP: net.IPv4zero, Port: p}
		conn, err := net.ListenUDP("udp4", addr)
		if err == nil {
			return conn, p, nil
		}
		// if it fails because “address already in use” you just try the next port
		// if it fails for some other reason you might want to bail out:
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Err.Error() != "bind: address already in use" {
			return nil, 0, fmt.Errorf("error binding to %d: %w", p, err)
		}
	}

	return nil, 0, fmt.Errorf("no free port in range %d–%d", startPort, endPort)
}

func NewServer(lg logger.Logger, port int) (*Conn, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port})
	if err != nil {
		return nil, fmt.Errorf("could not create client: %v", conn)
	}

	if err := conn.SetReadBuffer(7500000); err != nil {
		return nil, fmt.Errorf("SetReadBuffer: %w", err)
	}

	if err := conn.SetWriteBuffer(7500000); err != nil {
		return nil, fmt.Errorf("SetWriteBuffer: %w", err)
	}

	c := &Conn{
		lg:           lg,
		conn:         ipv4.NewPacketConn(conn),
		logReadTimer: time.NewTicker(1 * time.Second),
	}

	return c, nil
}

func NewClient(lg logger.Logger, startPort, endPort int) (*Conn, error) {
	conn, _, err := ListenUDPInRange(startPort, endPort)
	if err != nil {
		return nil, err
	}

	if err := conn.SetReadBuffer(7500000); err != nil {
		return nil, fmt.Errorf("SetReadBuffer: %w", err)
	}

	if err := conn.SetWriteBuffer(7500000); err != nil {
		return nil, fmt.Errorf("SetWriteBuffer: %w", err)
	}

	c := &Conn{
		lg:           lg,
		conn:         ipv4.NewPacketConn(conn),
		logReadTimer: time.NewTicker(10 * time.Second),
	}

	return c, nil
}

func (c *Conn) Write(msgs []ipv4.Message) error {
	_, err := c.conn.WriteBatch(msgs, 0)

	return err
}

// Conn.Read pulls up to batchSize packets in one recvmmsg call.
// Each Packet returned by At or ForEach must be freed via Packet.Free().
func (c *Conn) Read() (*Packets, error) {
	p := readBatchPool.Get().(*Packets)

	// allow ReadBatch to fill all slots
	p.msgs = p.msgs[:cap(p.msgs)]

	n, err := c.conn.ReadBatch(p.msgs, 0)
	if err != nil {
		readBatchPool.Put(p)
		return nil, fmt.Errorf("readBatch error: %w", err)
	}
	p.msgs = p.msgs[:n] // trim to actual number of packets read

	return p, nil
}

func NewUDPAddr(addr net.UDPAddr) (UDPAddr, error) {
	netIPAddr, err := udpAddrToNetip(&addr)
	if err != nil {
		return UDPAddr{}, err
	}

	return UDPAddr{
		UDPAddr:   addr,
		NetipAddr: netIPAddr,
	}, nil
}

func udpAddrToNetip(udpAddr *net.UDPAddr) (netip.Addr, error) {
	ip := udpAddr.IP

	// IPv4?  ip.To4() just returns a 4-byte slice pointing
	// into the original data (no new heap allocation).
	if v4 := ip.To4(); v4 != nil {
		var a [4]byte
		copy(a[:], v4) // copies into a stack-allocated [4]byte
		return netip.AddrFrom4(a), nil
	}

	// IPv6?  ip.To16() returns ip itself if it’s already a 16-byte slice
	// (again, no alloc). Only if ip were a 4-byte slice would To16 make a new slice,
	// but we never hit that branch because we already checked To4 above.
	if v6 := ip.To16(); v6 != nil {
		var a [16]byte
		copy(a[:], v6) // copies into a stack-allocated [16]byte
		return netip.AddrFrom16(a), nil
	}

	return netip.Addr{}, fmt.Errorf("invalid IP: %v", ip)
}

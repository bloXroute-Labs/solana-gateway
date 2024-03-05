package udp

import (
	"net"
)

const local = "0.0.0.0"

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

package packet

import "github.com/google/gopacket"

type Handler interface {
	ZeroCopyReadPacketData() (data []byte, ci gopacket.CaptureInfo, err error)
	Close()
}

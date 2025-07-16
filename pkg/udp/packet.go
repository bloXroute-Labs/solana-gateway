package udp

import (
	"net"

	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
	"golang.org/x/net/ipv4"
)

const batchSize = 100

// Packet is what you consume, with a Free method to recycle buffers.
type Packet struct {
	Data [solana.UDPShredSize]byte
	Addr *net.UDPAddr
	Len  int
}

// Packets holds a batch of ipv4.Messages and tracks how many remain.
type Packets struct {
	msgs []ipv4.Message
}

// ForEach calls fn on every Packet in this batch. Each Packet.Free() must
// be invoked for automatic recycling; you can defer pkt.Free() inside fn.
func (p *Packets) ForEach(fn func(Packet)) {
	for i := range p.msgs {
		pkt := Packet{
			Data: ([solana.UDPShredSize]byte)(p.msgs[i].Buffers[0]),
			Addr: p.msgs[i].Addr.(*net.UDPAddr),
			Len:  p.msgs[i].N,
		}
		fn(pkt)
	}
}

// Len returns the number of packets in this batch.
func (p *Packets) Len() int {
	return len(p.msgs)
}

// At returns the packet at the given index.
func (p *Packets) At(i int) Packet {
	return Packet{
		Data: ([solana.UDPShredSize]byte)(p.msgs[i].Buffers[0]),
		Addr: p.msgs[i].Addr.(*net.UDPAddr),
		Len:  p.msgs[i].N,
	}
}

// Free immediately returns the entire batch to the pool, regardless of
// how many Packet.Free() calls have occurred.
func (p *Packets) Free() {
	readBatchPool.Put(p)
}

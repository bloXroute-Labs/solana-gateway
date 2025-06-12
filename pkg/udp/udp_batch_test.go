package udp

import (
	"bytes"
	"net"
	"testing"

	"golang.org/x/net/ipv4"
)

// benchPackets is a pre-populated Packets for benchmark.
var benchPackets *Packets

func init() {
	msgs := make([]ipv4.Message, batchSize)
	for i := range msgs {
		buf := make([]byte, 1500)
		msgs[i].Buffers = [][]byte{buf}
		// Simulate a packet of length 100 bytes
		msgs[i].N = 100
		msgs[i].Addr = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234}
	}
	benchPackets = &Packets{msgs: msgs}
}

func BenchmarkPacketsLen(b *testing.B) {
	p := benchPackets
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Len()
	}
}

func BenchmarkPacketsAt(b *testing.B) {
	p := benchPackets
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.At(i % p.Len())
	}
}

func BenchmarkPacketsForEach(b *testing.B) {
	p := benchPackets
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.ForEach(func(pkt Packet) {
			// no-op
		})
	}
}

// TestPacketsAtDifferentLengths ensures At returns exact Data slices.
func TestPacketsAtDifferentLengths(t *testing.T) {
	lengths := []int{0, 10, 1500}
	msgs := make([]ipv4.Message, len(lengths))
	for i, l := range lengths {
		buf := make([]byte, 1500)
		for j := 0; j < l; j++ {
			buf[j] = byte(i*10 + j)
		}
		msgs[i].Buffers = [][]byte{buf}
		msgs[i].N = l
		msgs[i].Addr = &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 1234 + i}
	}
	p := &Packets{msgs: msgs}

	for i, l := range lengths {
		pkt := p.At(i)
		if len(pkt.Data) != l {
			t.Errorf("At(%d): expected length %d, got %d", i, l, len(pkt.Data))
		}
		expected := msgs[i].Buffers[0][:l]
		if !bytes.Equal(pkt.Data, expected) {
			t.Errorf("At(%d): data mismatch", i)
		}
		if pkt.Addr.Port != 1234+i {
			t.Errorf("At(%d): expected port %d, got %d", i, 1234+i, pkt.Addr.Port)
		}
	}
}

// TestPacketsForEach ensures ForEach iterates correctly over packets.
func TestPacketsForEach(t *testing.T) {
	lengths := []int{5, 0, 20}
	msgs := make([]ipv4.Message, len(lengths))
	for i, l := range lengths {
		fullBuf := make([]byte, 1500)
		for j := 0; j < l; j++ {
			fullBuf[j] = byte(j)
		}
		msgs[i].Buffers = [][]byte{fullBuf}
		msgs[i].N = l
		msgs[i].Addr = &net.UDPAddr{IP: net.IPv4(5, 6, 7, 8), Port: 2000 + i}
	}
	p := &Packets{msgs: msgs}

	index := 0
	p.ForEach(func(pkt Packet) {
		l := lengths[index]
		if len(pkt.Data) != l {
			t.Errorf("ForEach pkt %d: expected length %d, got %d", index, l, len(pkt.Data))
		}
		expected := msgs[index].Buffers[0][:l]
		if !bytes.Equal(pkt.Data, expected) {
			t.Errorf("ForEach pkt %d: data mismatch", index)
		}
		if pkt.Addr.Port != 2000+index {
			t.Errorf("ForEach pkt %d: expected port %d, got %d", index, 2000+index, pkt.Addr.Port)
		}
		index++
	})
	if index != len(lengths) {
		t.Errorf("ForEach: expected %d calls, got %d", len(lengths), index)
	}
}

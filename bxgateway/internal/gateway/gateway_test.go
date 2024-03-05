package gateway

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// go test -bench=. -benchtime 5s
// goos: darwin
// goarch: arm64
// pkg: github.com/bloXroute-Labs/solana-gateway/gateway/internal/bdn
// BenchmarkStringBuilder-8        98966148                78.23 ns/op
// BenchmarkSprintf-8              43882196               137.9 ns/op

func BenchmarkStringBuilder(b *testing.B) {
	var slot uint64 = 413783688
	var index uint32 = 1022
	var typ = "Code"

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		var buffer strings.Builder
		buffer.WriteString(strconv.Itoa(int(slot)))
		buffer.WriteString(strconv.Itoa(int(index)))
		buffer.WriteString(typ)
		_ = buffer.String()
	}
}

func BenchmarkSprintf(b *testing.B) {
	var slot uint64 = 413783688
	var index uint32 = 1022
	var typ = "Code"

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		_ = fmt.Sprintf("%d:%d:%s", slot, index, typ)
	}
}

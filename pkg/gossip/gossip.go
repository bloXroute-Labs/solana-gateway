package gossip

import (
	"crypto/ed25519"
	"fmt"
	"net"
	"time"
	"unsafe"
)

/*
#cgo LDFLAGS: -L./ -lgossip
#include <stdlib.h>
#include <stdint.h>

void run_gossip(
	const char* public_ip,
    uint16_t public_tvu_port,
    uint16_t public_gossip_port,
	const uint8_t* private_key_pointer,
	uintptr_t private_key_len,
	uint16_t shred_version,
	char** gossip_entrypoint_addrs,
	uintptr_t gossip_entrypoint_count
);
*/
import "C"

const gossipTimeout = time.Second

func Run(
	entrypoints []string,
	publicIP string,
	gossipPort uint16,
	tvuPort uint16,
	shredVersion uint64,
	privateKey ed25519.PrivateKey,
) error {
	if len(entrypoints) == 0 {
		return fmt.Errorf("entrypoints are required")
	}

	ip := net.ParseIP(publicIP)
	if ip == nil {
		return fmt.Errorf("unable to parse public IP: %s", publicIP)
	}

	// Convert to C array of string pointers
	entrypoints_C := make([]*C.char, len(entrypoints))
	for i, addr := range entrypoints {
		entrypoints_C[i] = C.CString(addr)
	}

	// Ensure we free the allocated C strings
	defer func() {
		for _, addr := range entrypoints_C {
			C.free(unsafe.Pointer(addr))
		}
	}()

	var shredVersion_C C.uint16_t = C.uint16_t(shredVersion)
	var publicIP_C = C.CString(publicIP)
	defer C.free(unsafe.Pointer(publicIP_C))
	var tvuPort_C C.uint16_t = C.uint16_t(tvuPort)
	var gossipPort_C C.uint16_t = C.uint16_t(gossipPort)

	C.run_gossip(
		publicIP_C,
		tvuPort_C,
		gossipPort_C,
		(*C.uint8_t)(unsafe.Pointer(&privateKey[0])),
		C.uintptr_t(len(privateKey)),
		shredVersion_C,
		(**C.char)(unsafe.Pointer(&entrypoints_C[0])),
		C.uintptr_t(len(entrypoints)),
	)

	return nil
}

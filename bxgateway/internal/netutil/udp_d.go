//go:build darwin

package netutil

import (
	"errors"
)

// ListPotentialUDPPorts is not implemented on this platform
func ListPotentialUDPPorts(_ string, _ []int) ([]int, error) {
	return nil, errors.New("ListOpenUDPPorts not implemented on this platform")
}

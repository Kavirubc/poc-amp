//go:build !linux || ignore_ebpf

// This file provides stub implementations for non-Linux builds or when eBPF is not available.
// On Linux with proper eBPF support, this will be replaced by generated code from bpf2go.

package ebpf

import (
	"fmt"

	"github.com/cilium/ebpf"
)

// bpfObjects contains all objects after they have been loaded into the kernel.
type bpfObjects struct {
	bpfPrograms
	bpfMaps
}

type bpfPrograms struct {
	CaptureHttp     *ebpf.Program
	TraceTcpSendmsg *ebpf.Program
	TraceTcpRecvmsg *ebpf.Program
}

type bpfMaps struct {
	AmpContainers *ebpf.Map
	Rb            *ebpf.Map
}

func (o *bpfObjects) Close() error {
	return nil
}

func loadBpfObjects(obj *bpfObjects, opts *ebpf.CollectionOptions) error {
	return fmt.Errorf("eBPF not available: run 'go generate ./...' on Linux to generate bindings")
}

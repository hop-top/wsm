package irc

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

// busySet returns a free-func that reports every port except those listed.
func busySet(busy ...int) func(int) bool {
	set := make(map[int]bool, len(busy))
	for _, p := range busy {
		set[p] = true
	}
	return func(p int) bool { return !set[p] }
}

func TestAllocatePorts(t *testing.T) {
	tests := []struct {
		name    string
		rng     [2]int
		free    func(int) bool
		want    Ports
		wantErr string
	}{
		{
			name: "first triple when all free",
			rng:  [2]int{6700, 6799},
			free: busySet(),
			want: Ports{Plain: 6700, Remote: 6701, API: 6702},
		},
		{
			name: "skips a blocked triple",
			rng:  [2]int{6700, 6799},
			free: busySet(6701),
			want: Ports{Plain: 6703, Remote: 6704, API: 6705},
		},
		{
			name: "skips several blocked triples",
			rng:  [2]int{6700, 6799},
			free: busySet(6700, 6704, 6708),
			want: Ports{Plain: 6709, Remote: 6710, API: 6711},
		},
		{
			name: "exact fit",
			rng:  [2]int{6700, 6702},
			free: busySet(),
			want: Ports{Plain: 6700, Remote: 6701, API: 6702},
		},
		{
			name:    "no free triple",
			rng:     [2]int{6700, 6702},
			free:    busySet(6701),
			wantErr: "no free consecutive 3-port block",
		},
		{
			name:    "range too narrow",
			rng:     [2]int{6700, 6701},
			free:    busySet(),
			wantErr: "must span at least 3 ports",
		},
		{
			name:    "inverted range",
			rng:     [2]int{6800, 6700},
			free:    busySet(),
			wantErr: "start must not exceed end",
		},
		{
			name:    "out of bounds",
			rng:     [2]int{0, 70000},
			free:    busySet(),
			wantErr: "must be 1-65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AllocatePorts(tt.rng, tt.free)
			assertErr(t, err, tt.wantErr)
			if tt.wantErr != "" {
				return
			}
			if got != tt.want {
				t.Fatalf("AllocatePorts(%v) = %+v, want %+v", tt.rng, got, tt.want)
			}
			if !got.Valid() {
				t.Errorf("allocated triple %+v is not consecutive", got)
			}
		})
	}
}

func TestAllocatePorts_ScansInTriples(t *testing.T) {
	// The scan must step by 3, never hand out overlapping triples to two
	// workspaces: a stride of 1 would let ws A take 6700-6702 and ws B
	// 6701-6703.
	var probed []int
	free := func(p int) bool {
		probed = append(probed, p)
		return false
	}

	if _, err := AllocatePorts([2]int{6700, 6708}, free); err == nil {
		t.Fatal("expected exhaustion error")
	}

	// Every triple is abandoned at its first busy port, so only the bases
	// are probed.
	want := []int{6700, 6703, 6706}
	if len(probed) != len(want) {
		t.Fatalf("probed %v, want the triple bases %v", probed, want)
	}
	for i, p := range want {
		if probed[i] != p {
			t.Fatalf("probed %v, want the triple bases %v", probed, want)
		}
	}
}

func TestPortsFreeFunc(t *testing.T) {
	p := Ports{Plain: 6700, Remote: 6701, API: 6702}

	if !PortsFreeFunc(p, busySet()) {
		t.Error("all-free triple must report free")
	}
	for _, busy := range []int{6700, 6701, 6702} {
		if PortsFreeFunc(p, busySet(busy)) {
			t.Errorf("triple with %d busy must report not free", busy)
		}
	}
}

func TestBindTest(t *testing.T) {
	// A real listener makes its port unbindable; closing it frees the port
	// again. This is the fixture the start/restart re-check relies on.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listenerPort(t, ln)

	if BindTest(port) {
		t.Errorf("port %d is held by a listener but reported free", port)
	}

	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !BindTest(port) {
		t.Errorf("port %d was released but reported busy", port)
	}
}

func TestPortsFree_RealListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := listenerPort(t, ln)

	// The held port sits in the middle of the triple, so the whole triple
	// must be rejected.
	if PortsFree(Ports{Plain: port - 1, Remote: port, API: port + 1}) {
		t.Error("a triple containing a held port must not report free")
	}
}

func TestPorts_Valid(t *testing.T) {
	tests := []struct {
		name string
		p    Ports
		want bool
	}{
		{name: "consecutive", p: Ports{6700, 6701, 6702}, want: true},
		{name: "zero", p: Ports{}, want: false},
		{name: "gap", p: Ports{6700, 6702, 6703}, want: false},
		{name: "unset remote", p: Ports{Plain: 6700}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Valid(); got != tt.want {
				t.Errorf("Valid(%+v) = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}

func TestPorts_Slice(t *testing.T) {
	p := Ports{Plain: 6700, Remote: 6701, API: 6702}
	got := p.Slice()

	want := []int{6700, 6701, 6702}
	if len(got) != len(want) {
		t.Fatalf("Slice() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Slice() = %v, want %v", got, want)
		}
	}
}

// listenerPort extracts the port a listener bound to.
func listenerPort(t *testing.T, ln net.Listener) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr %q: %v", ln.Addr(), err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	if strings.TrimSpace(portStr) == "" || port == 0 {
		t.Fatalf("listener reported no port: %q", ln.Addr())
	}
	return port
}

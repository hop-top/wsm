package irc

import (
	"fmt"
	"net"
	"strconv"
)

// AllocatePorts finds the first consecutive triple inside rng where all three
// ports are free, scanning in steps of PortsPerWorkspace.
//
// free reports whether one port can be bound; pass nil for the real
// bind test (BindTest). The check is inherently TOCTOU: another process may
// take a port between the test and ergo's own bind. That race is accepted
// rather than locked against, because the alternative (holding all three
// listeners open until handoff) cannot survive the exec. The allocation is
// recorded in profile.yaml and re-checked on every start; a conflict there is
// reported as a failed prerequisite instead of being silently reallocated, so
// a workspace never drifts to a different port behind the operator's back.
func AllocatePorts(rng [2]int, free func(int) bool) (Ports, error) {
	if err := validatePortRange(rng); err != nil {
		return Ports{}, err
	}
	if free == nil {
		free = BindTest
	}

	for base := rng[0]; base+PortsPerWorkspace-1 <= rng[1]; base += PortsPerWorkspace {
		p := Ports{Plain: base, Remote: base + 1, API: base + 2}
		if PortsFreeFunc(p, free) {
			return p, nil
		}
	}

	return Ports{}, fmt.Errorf(
		"irc.port_range [%d %d]: no free consecutive %d-port block available",
		rng[0], rng[1], PortsPerWorkspace,
	)
}

// PortsFree reports whether every port in the triple is bindable now. Used on
// start and restart to re-check a recorded allocation.
func PortsFree(p Ports) bool { return PortsFreeFunc(p, BindTest) }

// PortsFreeFunc is PortsFree with an injectable per-port check, for tests and
// for callers that probe a specific interface.
func PortsFreeFunc(p Ports, free func(int) bool) bool {
	if free == nil {
		free = BindTest
	}
	for _, port := range []int{p.Plain, p.Remote, p.API} {
		if !free(port) {
			return false
		}
	}
	return true
}

// BindTest reports whether port can be bound on the loopback interface right
// now. It closes the listener immediately; see AllocatePorts on the race.
func BindTest(port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// Slice returns the triple in listener order: plain, remote, API.
func (p Ports) Slice() []int { return []int{p.Plain, p.Remote, p.API} }

// Valid reports whether the triple is populated and consecutive.
func (p Ports) Valid() bool {
	return p.Plain > 0 && p.Remote == p.Plain+1 && p.API == p.Plain+2
}

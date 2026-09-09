package probe

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// testDNS is a minimal UDP DNS responder. It exists so the four outcomes wlink
// must tell apart can be produced deliberately rather than hoped for from a
// real network.
type testDNS struct {
	rcode  int    // 0 NOERROR, 2 SERVFAIL, 3 NXDOMAIN, 5 REFUSED
	answer string // dotted-quad to return for A queries; "" means NODATA
	addr   string
	conn   *net.UDPConn
}

func startTestDNS(t *testing.T, rcode int, answer string) *testDNS {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	d := &testDNS{rcode: rcode, answer: answer, conn: conn, addr: conn.LocalAddr().String()}
	go d.serve()
	t.Cleanup(func() { _ = conn.Close() })
	return d
}

func (d *testDNS) serve() {
	buf := make([]byte, 512)
	for {
		n, from, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		resp := d.respond(buf[:n])
		if resp != nil {
			_, _ = d.conn.WriteToUDP(resp, from)
		}
	}
}

// respond echoes the query and sets QR + rcode, appending an A record when one
// is configured and the question was for type A.
func (d *testDNS) respond(q []byte) []byte {
	if len(q) < 12 {
		return nil
	}
	// Walk the QNAME to find the question's type.
	i := 12
	for i < len(q) && q[i] != 0 {
		i += int(q[i]) + 1
	}
	i++ // the zero label
	if i+4 > len(q) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(q[i : i+2])
	qEnd := i + 4

	resp := make([]byte, qEnd)
	copy(resp, q[:qEnd])
	binary.BigEndian.PutUint16(resp[2:4], uint16(0x8180|d.rcode)) // QR|RD|RA|rcode
	binary.BigEndian.PutUint16(resp[4:6], 1)                      // QDCOUNT

	if d.rcode == 0 && d.answer != "" && qtype == 1 { // A
		ip := net.ParseIP(d.answer).To4()
		rr := []byte{0xC0, 0x0C, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4}
		rr = append(rr, ip...)
		resp = append(resp, rr...)
		binary.BigEndian.PutUint16(resp[6:8], 1) // ANCOUNT
	}
	return resp
}

func probeAgainst(t *testing.T, server, name string) Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return (&Resolver{Timeout: 2 * time.Second}).LookupA(ctx, server, name)
}

// EC-8: the four outcomes must stay distinct. Collapsing any two breaks either
// the recursion guard (which needs "answered but gave no address") or the
// readiness diagnosis (which needs "said nothing at all").
func TestLookupA_DistinguishesTheFourOutcomes(t *testing.T) {
	t.Run("resolved", func(t *testing.T) {
		d := startTestDNS(t, 0, "10.10.0.21")
		got := probeAgainst(t, d.addr, "lab-pi")
		if got.Outcome != Resolved {
			t.Fatalf("Outcome = %q, want %q", got.Outcome, Resolved)
		}
		if len(got.Addrs) != 1 || got.Addrs[0] != "10.10.0.21" {
			t.Errorf("Addrs = %v, want [10.10.0.21]", got.Addrs)
		}
		if !got.HasAddress() || !got.Reachable() {
			t.Error("a resolved lookup must report both HasAddress and Reachable")
		}
	})

	t.Run("nxdomain is answered-without-an-address", func(t *testing.T) {
		d := startTestDNS(t, 3, "")
		got := probeAgainst(t, d.addr, "nope")
		if got.Outcome != NoAddress {
			t.Fatalf("Outcome = %q, want %q", got.Outcome, NoAddress)
		}
		if !got.Reachable() {
			t.Error("NXDOMAIN proves the server is reachable; it answered")
		}
		if got.HasAddress() {
			t.Error("NXDOMAIN yields no address")
		}
	})

	t.Run("nodata is answered-without-an-address", func(t *testing.T) {
		// NOERROR with zero answers: the name exists, just not as an A record.
		d := startTestDNS(t, 0, "")
		got := probeAgainst(t, d.addr, "aaaa-only")
		if got.Outcome != NoAddress {
			t.Fatalf("Outcome = %q, want %q", got.Outcome, NoAddress)
		}
		if !got.Reachable() {
			t.Error("NODATA proves reachability")
		}
	})

	t.Run("servfail is reachable-but-unhelpful, NOT silent", func(t *testing.T) {
		d := startTestDNS(t, 2, "")
		got := probeAgainst(t, d.addr, "lab-pi")
		if got.Outcome != Unhelpful {
			t.Fatalf("Outcome = %q, want %q", got.Outcome, Unhelpful)
		}
		if !got.Reachable() {
			t.Error("SERVFAIL came from the server, so it is reachable — treating it as silent would misdiagnose a working tunnel as not-ready")
		}
	})

	t.Run("no response at all is silent", func(t *testing.T) {
		// A TEST-NET address nothing routes to: the query times out.
		got := probeAgainst(t, "198.51.100.53", "lab-pi")
		if got.Outcome != Silent {
			t.Fatalf("Outcome = %q, want %q", got.Outcome, Silent)
		}
		if got.Reachable() {
			t.Error("a timeout means unreachable")
		}
	})
}

// A bare address must work: callers hold resolver IPs, not host:port.
func TestLookupA_AcceptsServerWithoutPort(t *testing.T) {
	d := startTestDNS(t, 0, "10.10.0.21")
	host, port, _ := net.SplitHostPort(d.addr)
	r := &Resolver{Timeout: 2 * time.Second, Port: port}
	got := r.LookupA(context.Background(), host, "lab-pi")
	if got.Outcome != Resolved {
		t.Fatalf("Outcome = %q, want %q (bare host must get the default port appended)", got.Outcome, Resolved)
	}
}

// The timeout is what keeps a dead resolver from stalling an install, so it is
// asserted rather than assumed.
func TestLookupA_HonoursItsTimeout(t *testing.T) {
	start := time.Now()
	got := (&Resolver{Timeout: 300 * time.Millisecond}).
		LookupA(context.Background(), "198.51.100.53", "lab-pi")
	elapsed := time.Since(start)
	if got.Outcome != Silent {
		t.Fatalf("Outcome = %q, want %q", got.Outcome, Silent)
	}
	if elapsed > 3*time.Second {
		t.Errorf("took %s, want it bounded near the 300ms timeout", elapsed)
	}
}

// A cancelled context must stop the probe promptly — `wlink status` in a status
// line cannot hang.
func TestLookupA_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := (&Resolver{Timeout: 5 * time.Second}).LookupA(ctx, "198.51.100.53", "lab-pi")
	if got.Outcome != Silent {
		t.Errorf("Outcome = %q, want %q on a cancelled context", got.Outcome, Silent)
	}
}

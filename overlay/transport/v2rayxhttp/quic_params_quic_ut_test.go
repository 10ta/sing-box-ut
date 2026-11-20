//go:build with_xhttp && with_quic

package v2rayxhttp

import (
	"context"
	stdTLS "crypto/tls"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/quic-go/http3"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// uploadOverHTTP3 sends size bytes up a stream-one XHTTP connection to an
// in-process http3 server and returns how long it took until the server
// confirmed it had all of them.
func uploadOverHTTP3(t *testing.T, quicParams *option.V2RayXHTTPQUICParamsOptions, size int) time.Duration {
	t.Helper()
	certificate, hash := testCertificate(t)
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http3.Server{
		TLSConfig: http3.ConfigureTLSConfig(&stdTLS.Config{Certificates: []stdTLS.Certificate{*certificate}}),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			n, _ := io.CopyN(io.Discard, r.Body, int64(size))
			w.Write([]byte(strconv.FormatInt(n, 10) + "\n"))
			w.(http.Flusher).Flush()
			io.Copy(io.Discard, r.Body)
		}),
	}
	go server.Serve(packetConn)
	t.Cleanup(func() { server.Close() })
	// Brutal sizes its window from the RTT: on bare loopback the window is below
	// one packet and every packet waits for a delayed ACK.
	serverAddr := delayedUDPRelay(t, packetConn.LocalAddr(), 20*time.Millisecond)

	tlsConfig, err := tls.NewClient(context.Background(), log.NewNOPFactory().Logger(), "localhost", option.OutboundTLSOptions{
		Enabled:                    true,
		ServerName:                 "localhost",
		ALPN:                       []string{"h3"},
		CertificatePublicKeySHA256: [][]byte{hash},
	})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewClient(context.Background(), N.SystemDialer, M.SocksaddrFromNet(serverAddr), option.V2RayXHTTPOptions{Mode: modeStreamOne, Path: "/xhttp/", QUICParams: quicParams}, tlsConfig)
	if err != nil {
		t.Fatal(err)
	}
	client := transport.(*Client)
	t.Cleanup(func() { client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := client.DialContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start := time.Now()
	go conn.Write(make([]byte, size))
	reply := make([]byte, len(strconv.Itoa(size))+1)
	if _, err := io.ReadFull(conn, reply); err != nil || string(reply) != strconv.Itoa(size)+"\n" {
		t.Fatalf("reply = %q, %v", reply, err)
	}
	return time.Since(start)
}

// delayedUDPRelay forwards UDP between one client and target, holding every
// datagram for delay each way.
func delayedUDPRelay(t *testing.T, target net.Addr, delay time.Duration) net.Addr {
	t.Helper()
	front, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	back, err := net.DialUDP("udp", nil, target.(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		front.Close()
		back.Close()
	})
	var client atomic.Pointer[net.Addr]
	go func() {
		buffer := make([]byte, 65535)
		for {
			n, addr, err := front.ReadFrom(buffer)
			if err != nil {
				return
			}
			client.Store(&addr)
			packet := append([]byte(nil), buffer[:n]...)
			time.AfterFunc(delay, func() { back.Write(packet) })
		}
	}()
	go func() {
		buffer := make([]byte, 65535)
		for {
			n, err := back.Read(buffer)
			if err != nil {
				return
			}
			addr := client.Load()
			if addr == nil {
				continue
			}
			packet := append([]byte(nil), buffer[:n]...)
			time.AfterFunc(delay, func() { front.WriteTo(packet, *addr) })
		}
	}()
	return front.LocalAddr()
}

// TestHTTP3QUICParams: every congestion carries the upload, and brutal holds
// it to up_mbps: 2 MB at 8 Mbps (1 MB/s) over a 40 ms RTT takes about 2 s,
// while the other controllers are not held to any rate.
func TestHTTP3QUICParams(t *testing.T) {
	const size = 2 << 20
	for _, test := range []struct {
		name       string
		quicParams *option.V2RayXHTTPQUICParamsOptions
	}{
		{"default", nil},
		{"bbr", &option.V2RayXHTTPQUICParamsOptions{Congestion: "bbr"}},
		// brutal_debug without a log factory must not panic.
		{"brutal", &option.V2RayXHTTPQUICParamsOptions{Congestion: "brutal", UpMbps: 8, BrutalDebug: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			elapsed := uploadOverHTTP3(t, test.quicParams, size)
			t.Logf("%s: %d bytes in %s", test.name, size, elapsed)
			if test.name == "brutal" && (elapsed < 1500*time.Millisecond || elapsed > 6*time.Second) {
				t.Fatalf("brutal at 8 Mbps sent 2 MB in %s, want about 2 s", elapsed)
			}
		})
	}
}

//go:build with_quic

package masque

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	nethttp "net/http"
	"net/netip"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/quic-go"
	"github.com/sagernet/quic-go/http3"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/transport/http"
	"github.com/sagernet/sing-box/transport/masque"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"
)

// fakeWARPEdge behaves like Cloudflare's WARP edge as far as usque and Xray
// describe it: it announces HTTP Datagrams only with the draft-00 setting,
// requires a client certificate, serves CONNECT-IP as "cf-connect-ip", and
// assigns no addresses. It records what the client sent and echoes each IP
// packet back with source and destination swapped.
type fakeWARPEdge struct {
	t        *testing.T
	address  string
	clientCA *ecdsa.PublicKey
	report   chan edgeReport
}

type edgeReport struct {
	method, protocol, host, path string
	userAgent                    []string
	draftSetting                 uint64
	clientKeyMatches             bool
	capsuleBytes                 int
	err                          error
}

func startFakeWARPEdge(t *testing.T, serverKey *ecdsa.PrivateKey, clientKey *ecdsa.PublicKey) *fakeWARPEdge {
	t.Helper()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &serverKey.PublicKey, serverKey)
	if err != nil {
		t.Fatal(err)
	}
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig := http3.ConfigureTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{certificate}, PrivateKey: serverKey}},
		ClientAuth:   tls.RequireAnyClientCert,
	})
	listener, err := quic.ListenEarly(packetConn, tlsConfig, &quic.Config{EnableDatagrams: true})
	if err != nil {
		t.Fatal(err)
	}
	edge := &fakeWARPEdge{t: t, address: packetConn.LocalAddr().String(), clientCA: clientKey, report: make(chan edgeReport, 1)}
	server := &http3.Server{
		Handler:            edge,
		AdditionalSettings: map[uint64]uint64{http.SettingDatagramDraft00: 1},
	}
	go server.ServeListener(listener)
	t.Cleanup(func() {
		server.Close()
		listener.Close()
		packetConn.Close()
	})
	return edge
}

func (e *fakeWARPEdge) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	report := edgeReport{method: r.Method, protocol: r.Proto, host: r.Host, path: r.URL.Path, userAgent: r.Header.Values("User-Agent")}
	settings := w.(http3.Settingser)
	<-settings.ReceivedSettings()
	report.draftSetting = settings.Settings().Other[http.SettingDatagramDraft00]
	if r.TLS != nil && len(r.TLS.PeerCertificates) == 1 {
		key, isECDSA := r.TLS.PeerCertificates[0].PublicKey.(*ecdsa.PublicKey)
		report.clientKeyMatches = isECDSA && key.Equal(e.clientCA)
	}
	w.WriteHeader(200)
	w.(nethttp.Flusher).Flush()
	stream := w.(http3.HTTPStreamer).HTTPStream()
	// A WARP client sends no capsule (no ADDRESS_REQUEST) before its first
	// packet: read what arrives on the stream while waiting for a datagram.
	capsules := make(chan int, 1)
	go func() {
		var total int
		buffer := make([]byte, 512)
		for {
			n, err := stream.Read(buffer)
			total += n
			if err != nil {
				capsules <- total
				return
			}
		}
	}()
	// quic-go's server reads datagrams itself once the client announces
	// H3_DATAGRAM, so take them from the stream; the client side, which is
	// what this test is about, reads them off the QUIC connection.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	datagram, err := stream.ReceiveDatagram(ctx)
	if err != nil {
		report.err = err
		e.report <- report
		return
	}
	if len(datagram) < 21 || datagram[0] != 0 {
		report.err = io.ErrUnexpectedEOF
		e.report <- report
		return
	}
	packet := append([]byte(nil), datagram[1:]...)
	copy(packet[12:16], datagram[1+16:1+20])
	copy(packet[16:20], datagram[1+12:1+16])
	report.err = stream.SendDatagram(append([]byte{0}, packet...))
	time.Sleep(200 * time.Millisecond)
	stream.CancelRead(0)
	report.capsuleBytes = <-capsules
	e.report <- report
}

type packetSink struct {
	configured chan masque.Configuration
	packets    chan []byte
	once       sync.Once
}

func (s *packetSink) UpdateConfiguration(configuration masque.Configuration) error {
	s.once.Do(func() { s.configured <- configuration })
	return nil
}

func (s *packetSink) WriteInboundBuffers(buffers []*buf.Buffer) error {
	return s.WaitWriteInboundBuffers(buffers)
}

func (s *packetSink) WaitWriteInboundBuffers(buffers []*buf.Buffer) error {
	for _, buffer := range buffers {
		s.packets <- append([]byte(nil), buffer.Bytes()...)
		buffer.Release()
	}
	return nil
}

func (s *packetSink) FrontHeadroom() int { return 0 }

// testIPv4UDP builds an IPv4/UDP packet; checksums are not checked on this path.
func testIPv4UDP(source, destination netip.Addr, payload string) []byte {
	packet := make([]byte, 28+len(payload))
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:], uint16(len(packet)))
	packet[8] = 64
	packet[9] = 17
	copy(packet[12:16], source.AsSlice())
	copy(packet[16:20], destination.AsSlice())
	binary.BigEndian.PutUint16(packet[20:], 40000)
	binary.BigEndian.PutUint16(packet[22:], 53)
	binary.BigEndian.PutUint16(packet[24:], uint16(8+len(payload)))
	copy(packet[28:], payload)
	return packet
}

// TestWARPAgainstFakeEdge runs the WARP mode of masque-client end to end over
// HTTP/3 against fakeWARPEdge: mTLS with the generated certificate, the public
// key pin, cf-connect-ip, the draft datagram setting both ways, static
// addresses without ADDRESS_REQUEST, and one packet each way.
func TestWARPAgainstFakeEdge(t *testing.T) {
	clientKey, clientValue, serverKey, serverPublic := testWARPKeys(t)
	edge := startFakeWARPEdge(t, serverKey, &clientKey.PublicKey)
	host, portText, _ := net.SplitHostPort(edge.address)
	port, _ := strconv.Atoi(portText)
	local := netip.MustParseAddr("100.96.0.1")
	options := option.MASQUEClientEndpointOptions{
		ServerOptions: option.ServerOptions{Server: host, ServerPort: uint16(port)},
		WARP: &option.MASQUEWARPOptions{
			PrivateKey: clientValue,
			PublicKey:  serverPublic,
			Address:    []string{local.String()},
		},
		QUICParams: &option.QUICParamsOptions{Congestion: "bbr"},
	}
	logger := log.StdLogger()
	ut, err := newClientUT(logger, &options, 3)
	if err != nil {
		t.Fatal(err)
	}
	headers := options.Headers.Build()
	authority := headers.Get("Host")
	headers.Del("Host")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	httpClient, err := http.NewClientWithTLS(ctx, logger, N.SystemDialer, options.ServerOptions, common.PtrValueOrDefault(options.TLS), http.ClientOptions{
		Authority:              authority,
		Headers:                headers,
		Version:                3,
		DisableVersionFallback: options.DisableVersionFallback,
		HTTP3Options:           option.QUICOptions{InitialPacketSize: 1331},
		HTTP3UT:                ut.http3,
	})
	if err != nil {
		t.Fatal(err)
	}
	sink := &packetSink{configured: make(chan masque.Configuration, 1), packets: make(chan []byte, 4)}
	client, err := masque.NewClient(masque.ClientOptions{
		Context:    ctx,
		Logger:     logger,
		HTTPClient: httpClient,
		Path:       options.Path,
		Handler:    sink,
		UT:         ut.masque,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.Start()
	defer client.Close()

	select {
	case configuration := <-sink.configured:
		if len(configuration.Address) != 1 || configuration.Address[0] != netip.PrefixFrom(local, 32) {
			t.Fatalf("configuration = %+v", configuration)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("tunnel not configured")
	}
	remote := netip.MustParseAddr("1.1.1.1")
	packet := testIPv4UDP(local, remote, "ping")
	buffer := buf.NewSize(masque.PacketHeadroom + len(packet))
	buffer.Resize(masque.PacketHeadroom, 0)
	common.Must1(buffer.Write(packet))
	if err := client.WritePacketBuffers([]*buf.Buffer{buffer}, false); err != nil {
		t.Fatal(err)
	}
	select {
	case echoed := <-sink.packets:
		if len(echoed) != len(packet) || string(echoed[28:]) != "ping" ||
			netip.AddrFrom4([4]byte(echoed[16:20])) != local || netip.AddrFrom4([4]byte(echoed[12:16])) != remote {
			t.Fatalf("echoed packet % x", echoed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no packet back through the draft datagram path")
	}
	report := <-edge.report
	if report.err != nil {
		t.Fatal("edge: ", report.err)
	}
	if report.method != "CONNECT" || report.protocol != "cf-connect-ip" || report.host != "cloudflareaccess.com" || report.path != "/" {
		t.Fatalf("request %s %s %s %s", report.method, report.protocol, report.host, report.path)
	}
	if len(report.userAgent) != 0 {
		t.Fatalf("User-Agent sent: %q", report.userAgent)
	}
	if report.draftSetting != 1 {
		t.Fatal("client did not send SETTINGS 0x276")
	}
	if !report.clientKeyMatches {
		t.Fatal("client certificate does not carry warp.private_key")
	}
	if report.capsuleBytes != 0 {
		t.Fatalf("client sent %d capsule bytes (ADDRESS_REQUEST?)", report.capsuleBytes)
	}
}

// TestWARPPinRejectsOtherServer: a server with another key fails the pin.
func TestWARPPinRejectsOtherServer(t *testing.T) {
	clientKey, clientValue, _, serverPublic := testWARPKeys(t)
	_, _, otherServer, _ := testWARPKeys(t)
	edge := startFakeWARPEdge(t, otherServer, &clientKey.PublicKey)
	host, portText, _ := net.SplitHostPort(edge.address)
	port, _ := strconv.Atoi(portText)
	options := option.MASQUEClientEndpointOptions{
		ServerOptions: option.ServerOptions{Server: host, ServerPort: uint16(port)},
		WARP:          &option.MASQUEWARPOptions{PrivateKey: clientValue, PublicKey: serverPublic, Address: []string{"100.96.0.1"}},
	}
	logger := log.StdLogger()
	ut, err := newClientUT(logger, &options, 3)
	if err != nil {
		t.Fatal(err)
	}
	headers := options.Headers.Build()
	authority := headers.Get("Host")
	headers.Del("Host")
	httpClient, err := http.NewClientWithTLS(context.Background(), logger, N.SystemDialer, options.ServerOptions, *options.TLS, http.ClientOptions{
		Authority: authority, Headers: headers, Version: 3, DisableVersionFallback: true, HTTP3UT: ut.http3,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer httpClient.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := httpClient.OpenTunnel(ctx, masque.CloudflareProtocol, "/")
	if err == nil {
		stream.Close()
		t.Fatal("tunnel opened to a server with another key")
	}
}

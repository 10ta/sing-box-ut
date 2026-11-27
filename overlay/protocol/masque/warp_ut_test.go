package masque

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/netip"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/common/quicparams"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/transport/http"
	"github.com/sagernet/sing-box/transport/masque"
	"github.com/sagernet/sing/common/json/badoption"
)

// testWARPKeys returns a client key (base64 SEC 1, as usque writes it) and a
// server key whose public half is base64 PKIX (usque's endpoint_pub_key).
func testWARPKeys(t *testing.T) (client *ecdsa.PrivateKey, clientValue string, server *ecdsa.PrivateKey, serverPublicValue string) {
	t.Helper()
	client, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientDER, err := x509.MarshalECPrivateKey(client)
	if err != nil {
		t.Fatal(err)
	}
	server, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverPublicDER, err := x509.MarshalPKIXPublicKey(&server.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return client, base64.StdEncoding.EncodeToString(clientDER), server, base64.StdEncoding.EncodeToString(serverPublicDER)
}

func testWARPOptions(t *testing.T) option.MASQUEClientEndpointOptions {
	t.Helper()
	_, clientKey, _, serverPublic := testWARPKeys(t)
	return option.MASQUEClientEndpointOptions{
		ServerOptions: option.ServerOptions{Server: "162.159.197.2", ServerPort: 443},
		WARP: &option.MASQUEWARPOptions{
			PrivateKey: clientKey,
			PublicKey:  serverPublic,
			Address:    badoption.Listable[string]{"100.96.0.1", "2606:4700:11:1000::1/128"},
		},
	}
}

func TestWARPOptionsRewrite(t *testing.T) {
	options := testWARPOptions(t)
	options.Headers = badoption.HTTPHeader{"X-Test": {"1"}}
	ut, err := newClientUT(log.StdLogger(), &options, 3)
	if err != nil {
		t.Fatal(err)
	}
	if options.Path != "/" || !options.DisableVersionFallback {
		t.Fatalf("path %q, disable_version_fallback %v", options.Path, options.DisableVersionFallback)
	}
	if host := options.Headers.Build().Get("Host"); host != "cloudflareaccess.com" {
		t.Fatalf("Host = %q", host)
	}
	if options.Headers.Build().Get("X-Test") != "1" {
		t.Fatal("user header lost")
	}
	tlsOptions := options.TLS
	if !tlsOptions.Enabled || !tlsOptions.Insecure || !tlsOptions.DisableSNI ||
		len(tlsOptions.CertificatePublicKeySHA256) != 1 || len(tlsOptions.ClientCertificate) != 1 || len(tlsOptions.ClientKey) != 1 {
		t.Fatalf("tls = %+v", tlsOptions)
	}
	if _, err := tls.X509KeyPair([]byte(tlsOptions.ClientCertificate[0]), []byte(tlsOptions.ClientKey[0])); err != nil {
		t.Fatal("client certificate and key do not match: ", err)
	}
	wantAddress := []netip.Prefix{netip.MustParsePrefix("100.96.0.1/32"), netip.MustParsePrefix("2606:4700:11:1000::1/128")}
	if ut.masque.Protocol != "cf-connect-ip" || len(ut.masque.Address) != 2 || ut.masque.Address[0] != wantAddress[0] || ut.masque.Address[1] != wantAddress[1] {
		t.Fatalf("masque = %+v", ut.masque)
	}
	if ut.http3.ConnectionIDLength != 20 || ut.http3.AdditionalSettings[http.SettingDatagramDraft00] != 1 || !ut.http3.DraftDatagrams {
		t.Fatalf("http3 = %+v", ut.http3)
	}
	if ut.http3.Congestion.Enabled() {
		t.Fatal("congestion without quic_params")
	}
}

func TestWARPServerNameKeepsSNI(t *testing.T) {
	options := testWARPOptions(t)
	options.TLS = &option.OutboundTLSOptions{ServerName: "www.example.com", ALPN: badoption.Listable[string]{"h3"}}
	options.WARP.Host = "team.example.com"
	if _, err := newClientUT(log.StdLogger(), &options, 3); err != nil {
		t.Fatal(err)
	}
	if options.TLS.DisableSNI || options.TLS.ServerName != "www.example.com" {
		t.Fatalf("tls = %+v", options.TLS)
	}
	if host := options.Headers.Build().Get("Host"); host != "team.example.com" {
		t.Fatalf("Host = %q", host)
	}
}

func TestWARPPublicKeyPin(t *testing.T) {
	_, _, server, serverPublic := testWARPKeys(t)
	hash, err := parseWARPPublicKey(serverPublic)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&server.PublicKey)
	want := sha256.Sum256(der)
	if string(hash) != string(want[:]) {
		t.Fatal("pin differs from the SHA-256 of the PKIX key")
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	if pemHash, err := parseWARPPublicKey(pemValue); err != nil || string(pemHash) != string(hash) {
		t.Fatalf("PEM form: %v", err)
	}
}

func TestWARPPrivateKeyForms(t *testing.T) {
	client, sec1, _, _ := testWARPKeys(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(client)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"sec1":  sec1,
		"pkcs8": base64.StdEncoding.EncodeToString(pkcs8),
		"pem":   string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
	} {
		key, err := parseWARPPrivateKey(value)
		if err != nil || !key.Equal(client) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p384DER, _ := x509.MarshalECPrivateKey(p384)
	if _, err := parseWARPPrivateKey(base64.StdEncoding.EncodeToString(p384DER)); err == nil || !strings.Contains(err.Error(), "P-256") {
		t.Fatalf("P-384 key: %v", err)
	}
}

func TestWARPRefused(t *testing.T) {
	for _, test := range []struct {
		name    string
		version int
		edit    func(*option.MASQUEClientEndpointOptions)
		err     string
	}{
		{"version 1", 1, func(o *option.MASQUEClientEndpointOptions) { o.Version = 1 }, "version 1 (HTTP/1.1) is not supported"},
		{"version 2", 2, func(o *option.MASQUEClientEndpointOptions) { o.Version = 2 }, "version 2 (HTTP/2) is not supported yet"},
		{"no quic", 2, func(o *option.MASQUEClientEndpointOptions) {}, "this build has no QUIC"},
		{"username", 3, func(o *option.MASQUEClientEndpointOptions) { o.Username = "u" }, "username and password"},
		{"path", 3, func(o *option.MASQUEClientEndpointOptions) { o.Path = "/x" }, "path is fixed"},
		{"advertise_routes", 3, func(o *option.MASQUEClientEndpointOptions) {
			o.AdvertiseRoutes = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
		}, "advertise_routes"},
		{"host header", 3, func(o *option.MASQUEClientEndpointOptions) { o.Headers = badoption.HTTPHeader{"host": {"x"}} }, "warp.host"},
		{"capsule header", 3, func(o *option.MASQUEClientEndpointOptions) {
			o.Headers = badoption.HTTPHeader{"Capsule-Protocol": {"?1"}}
		}, "Capsule-Protocol"},
		{"insecure", 3, func(o *option.MASQUEClientEndpointOptions) { o.TLS = &option.OutboundTLSOptions{Insecure: true} }, "tls.insecure"},
		{"pin", 3, func(o *option.MASQUEClientEndpointOptions) {
			o.TLS = &option.OutboundTLSOptions{CertificatePublicKeySHA256: badoption.Listable[[]byte]{{1}}}
		}, "tls.certificate*"},
		{"client cert", 3, func(o *option.MASQUEClientEndpointOptions) { o.TLS = &option.OutboundTLSOptions{ClientKeyPath: "k"} }, "tls.client_*"},
		{"disable_sni", 3, func(o *option.MASQUEClientEndpointOptions) { o.TLS = &option.OutboundTLSOptions{DisableSNI: true} }, "tls.disable_sni"},
		{"alpn h2", 3, func(o *option.MASQUEClientEndpointOptions) {
			o.TLS = &option.OutboundTLSOptions{ALPN: badoption.Listable[string]{"h2"}}
		}, "tls.alpn"},
		{"utls", 3, func(o *option.MASQUEClientEndpointOptions) {
			o.TLS = &option.OutboundTLSOptions{UTLS: &option.OutboundUTLSOptions{Enabled: true}}
		}, "tls.utls"},
		{"no address", 3, func(o *option.MASQUEClientEndpointOptions) { o.WARP.Address = nil }, "address is required"},
		{"two ipv4", 3, func(o *option.MASQUEClientEndpointOptions) {
			o.WARP.Address = badoption.Listable[string]{"100.96.0.1", "100.96.0.2"}
		}, "at most one IPv4"},
		{"bad address", 3, func(o *option.MASQUEClientEndpointOptions) { o.WARP.Address = badoption.Listable[string]{"x"} }, "invalid address"},
		{"no private key", 3, func(o *option.MASQUEClientEndpointOptions) { o.WARP.PrivateKey = "" }, "private_key: missing"},
		{"bad public key", 3, func(o *option.MASQUEClientEndpointOptions) { o.WARP.PublicKey = "AAAA" }, "public_key: not a PKIX public key"},
		{"bad host", 3, func(o *option.MASQUEClientEndpointOptions) { o.WARP.Host = "a/b" }, "invalid host"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := testWARPOptions(t)
			test.edit(&options)
			_, err := newClientUT(log.StdLogger(), &options, test.version)
			if err == nil || !strings.Contains(err.Error(), test.err) || !strings.HasPrefix(err.Error(), "masque-client: warp: ") {
				t.Fatalf("err = %v, want %q", err, test.err)
			}
		})
	}
}

func TestMASQUEQUICParams(t *testing.T) {
	brutal := &option.QUICParamsOptions{Congestion: "brutal", UpMbps: 60}
	options := option.MASQUEClientEndpointOptions{QUICParams: brutal}
	ut, err := newClientUT(log.StdLogger(), &options, 3)
	if err != nil || ut.http3.Congestion.Kind() != quicparams.Brutal || ut.http3.Logger == nil {
		t.Fatalf("h3: %+v, %v", ut.http3, err)
	}
	if ut.masque.Protocol != "" || ut.http3.ConnectionIDLength != 0 {
		t.Fatal("warp settings without warp")
	}
	ut, err = newClientUT(log.StdLogger(), &options, 2)
	if err != nil || ut.http3.Congestion.Enabled() {
		t.Fatalf("h2: %+v, %v", ut.http3, err)
	}
	options.QUICParams = &option.QUICParamsOptions{Congestion: "brutal"}
	if _, err = newClientUT(log.StdLogger(), &options, 2); err == nil || !strings.HasPrefix(err.Error(), "masque-client: quic_params: ") {
		t.Fatalf("bad quic_params over h2: %v", err)
	}
	// Without warp and quic_params nothing changes.
	options = option.MASQUEClientEndpointOptions{Path: "/x", Version: 2}
	ut, err = newClientUT(log.StdLogger(), &options, 2)
	if err != nil || ut.masque.Protocol != "" || ut.masque.Address != nil || ut.http3.ConnectionIDLength != 0 ||
		ut.http3.AdditionalSettings != nil || ut.http3.DraftDatagrams || ut.http3.Congestion.Enabled() ||
		options.Path != "/x" || options.TLS != nil || options.Headers != nil || options.DisableVersionFallback {
		t.Fatalf("plain: %+v, %v", ut, err)
	}
}

var _ = masque.CloudflareProtocol

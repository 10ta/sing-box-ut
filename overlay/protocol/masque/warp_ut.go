package masque

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/sagernet/sing-box/common/quicparams"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/transport/http"
	"github.com/sagernet/sing-box/transport/masque"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/json/badoption"
)

// ut: sing-box-ut's additions to masque-client: quic_params, and the
// Cloudflare WARP mode (option.MASQUEWARPOptions).
//
// WARP mode, HTTP/3 only for now:
//   - CONNECT-IP with :protocol "cf-connect-ip" on path "/", authority
//     warp.host (default cloudflareaccess.com), no ADDRESS_REQUEST: the tunnel
//     uses warp.address at once;
//   - SETTINGS 0x276 (draft HTTP Datagrams) sent and accepted, QUIC connection
//     IDs of 20 bytes, no User-Agent unless headers set one;
//   - TLS: a client certificate made from warp.private_key, the server
//     certificate checked against warp.public_key only (no CA check); no SNI
//     when tls.server_name is empty;
//   - refused: version 1 and 2, username/password, path, advertise_routes, a
//     Host or Capsule-Protocol header, and TLS fields that the keys replace or
//     that cannot work here (insecure, certificate*, client_*, disable_sni,
//     alpn other than h3, ech, utls, reality);
//   - disable_version_fallback is forced on: there is no HTTP/2 to fall back
//     to.

const defaultWARPHost = "cloudflareaccess.com"

type clientUT struct {
	http3  http.HTTP3UTOptions
	masque masque.ClientUTOptions
}

// newClientUT is called once the HTTP version is resolved. It may rewrite
// options (tls, headers, path, disable_version_fallback) for WARP mode.
func newClientUT(logger log.ContextLogger, options *option.MASQUEClientEndpointOptions, version int) (clientUT, error) {
	var ut clientUT
	congestion, err := quicparams.Parse("masque-client", options.QUICParams)
	if err != nil {
		return ut, err
	}
	if congestion.Enabled() {
		if version == 3 {
			ut.http3.Congestion = congestion
			ut.http3.Logger = logger
		} else {
			logger.Warn("masque-client: quic_params only applies to HTTP/3 (version 3), ignored")
		}
	}
	if options.WARP == nil {
		return ut, nil
	}
	err = prepareWARP(options, version, &ut)
	if err != nil {
		return ut, E.Cause(err, "masque-client: warp")
	}
	return ut, nil
}

func prepareWARP(options *option.MASQUEClientEndpointOptions, version int, ut *clientUT) error {
	warp := options.WARP
	switch {
	case options.Version == 1:
		return E.New("version 1 (HTTP/1.1) is not supported, use 3")
	case options.Version == 2:
		return E.New("version 2 (HTTP/2) is not supported yet, use 3")
	case version != 3:
		return E.New("needs HTTP/3, and this build has no QUIC (build tag with_quic)")
	}
	if options.Username != "" || options.Password != "" {
		return E.New("username and password cannot be used with warp")
	}
	if options.Path != "" {
		return E.New("path is fixed to \"/\" in warp mode, remove it")
	}
	if len(options.AdvertiseRoutes) > 0 {
		return E.New("advertise_routes cannot be used with warp")
	}
	for name := range options.Headers {
		switch strings.ToLower(name) {
		case "host":
			return E.New("headers: use warp.host instead of a Host header")
		case "capsule-protocol":
			return E.New("headers: Capsule-Protocol is set by the client")
		}
	}
	privateKey, err := parseWARPPrivateKey(warp.PrivateKey)
	if err != nil {
		return E.Cause(err, "private_key")
	}
	publicKeyHash, err := parseWARPPublicKey(warp.PublicKey)
	if err != nil {
		return E.Cause(err, "public_key")
	}
	addresses, err := parseWARPAddress(warp.Address)
	if err != nil {
		return err
	}
	host := warp.Host
	if host == "" {
		host = defaultWARPHost
	}
	if parsed, parseErr := url.Parse("https://" + host); parseErr != nil || parsed.Host != host {
		return E.New("invalid host: ", host)
	}
	tlsOptions, err := warpTLSOptions(common.PtrValueOrDefault(options.TLS), privateKey, publicKeyHash)
	if err != nil {
		return err
	}
	options.TLS = &tlsOptions
	options.Path = "/"
	options.DisableVersionFallback = true
	headers := make(badoption.HTTPHeader, len(options.Headers)+1)
	for name, values := range options.Headers {
		headers[name] = values
	}
	headers["Host"] = badoption.Listable[string]{host}
	options.Headers = headers
	ut.masque = masque.ClientUTOptions{Protocol: masque.CloudflareProtocol, Address: addresses}
	ut.http3.ConnectionIDLength = 20
	ut.http3.AdditionalSettings = map[uint64]uint64{http.SettingDatagramDraft00: 1}
	ut.http3.DraftDatagrams = true
	return nil
}

func warpTLSOptions(tlsOptions option.OutboundTLSOptions, privateKey *ecdsa.PrivateKey, publicKeyHash []byte) (option.OutboundTLSOptions, error) {
	switch {
	case tlsOptions.Insecure:
		return tlsOptions, E.New("tls.insecure: the server is checked against warp.public_key, remove it")
	case len(tlsOptions.Certificate) > 0 || tlsOptions.CertificatePath != "" ||
		len(tlsOptions.CertificateSHA256) > 0 || len(tlsOptions.CertificatePublicKeySHA256) > 0:
		return tlsOptions, E.New("tls.certificate*: the server is checked against warp.public_key, remove them")
	case len(tlsOptions.ClientCertificate) > 0 || tlsOptions.ClientCertificatePath != "" ||
		len(tlsOptions.ClientKey) > 0 || tlsOptions.ClientKeyPath != "":
		return tlsOptions, E.New("tls.client_*: the client certificate is made from warp.private_key, remove them")
	case tlsOptions.DisableSNI:
		return tlsOptions, E.New("tls.disable_sni: leave tls.server_name empty to send no SNI")
	case len(tlsOptions.ALPN) > 0 && !(len(tlsOptions.ALPN) == 1 && tlsOptions.ALPN[0] == "h3"):
		return tlsOptions, E.New("tls.alpn: HTTP/3 only, remove it or set [\"h3\"]")
	case tlsOptions.ECH != nil && tlsOptions.ECH.Enabled:
		return tlsOptions, E.New("tls.ech is not supported with warp")
	case tlsOptions.UTLS != nil && tlsOptions.UTLS.Enabled:
		return tlsOptions, E.New("tls.utls is not supported with warp (HTTP/3)")
	case tlsOptions.Reality != nil && tlsOptions.Reality.Enabled:
		return tlsOptions, E.New("tls.reality is not supported with warp")
	}
	certificatePEM, keyPEM, err := warpClientCertificate(privateKey)
	if err != nil {
		return tlsOptions, err
	}
	tlsOptions.Enabled = true
	// No CA check: Cloudflare's WARP certificate is not publicly trusted. The
	// pin below still runs (VerifyPeerCertificate), and insecure also keeps
	// disable_sni from adding a host name check.
	tlsOptions.Insecure = true
	tlsOptions.CertificatePublicKeySHA256 = badoption.Listable[[]byte]{publicKeyHash}
	tlsOptions.ClientCertificate = badoption.Listable[string]{certificatePEM}
	tlsOptions.ClientKey = badoption.Listable[string]{keyPEM}
	if tlsOptions.ServerName == "" {
		tlsOptions.DisableSNI = true
	}
	return tlsOptions, nil
}

// decodeWARPKey takes PEM or base64 DER (usque writes base64 DER).
func decodeWARPKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, E.New("missing")
	}
	if block, _ := pem.Decode([]byte(value)); block != nil {
		return block.Bytes, nil
	}
	der, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, E.New("neither PEM nor base64")
	}
	return der, nil
}

func parseWARPPrivateKey(value string) (*ecdsa.PrivateKey, error) {
	der, err := decodeWARPKey(value)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(der)
	if err != nil {
		parsed, pkcs8Err := x509.ParsePKCS8PrivateKey(der)
		if pkcs8Err != nil {
			return nil, E.New("neither a SEC 1 nor a PKCS #8 private key")
		}
		var isECDSA bool
		key, isECDSA = parsed.(*ecdsa.PrivateKey)
		if !isECDSA {
			return nil, E.New("not an ECDSA key")
		}
	}
	if key.Curve != elliptic.P256() {
		return nil, E.New("not a P-256 key")
	}
	return key, nil
}

// parseWARPPublicKey returns the SHA-256 of the key in the form tls'
// certificate_public_key_sha256 compares (re-marshalled PKIX).
func parseWARPPublicKey(value string) ([]byte, error) {
	der, err := decodeWARPKey(value)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, E.New("not a PKIX public key")
	}
	normalized, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(normalized)
	return hash[:], nil
}

func parseWARPAddress(values []string) ([]netip.Prefix, error) {
	if len(values) == 0 {
		return nil, E.New("address is required")
	}
	var has4, has6 bool
	addresses := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addrErr := netip.ParseAddr(value)
			if addrErr != nil {
				return nil, E.New("invalid address: ", value)
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		is4 := prefix.Addr().Unmap().Is4()
		if is4 && has4 || !is4 && has6 {
			return nil, E.New("address takes at most one IPv4 and one IPv6 address")
		}
		has4 = has4 || is4
		has6 = has6 || !is4
		addresses = append(addresses, prefix)
	}
	return addresses, nil
}

// warpClientCertificate makes the self-signed certificate Cloudflare's edge
// asks for; it only checks that the key is enrolled. Valid for a year from
// start-up, as usque's.
func warpClientCertificate(key *ecdsa.PrivateKey) (string, string, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(365 * 24 * time.Hour),
	}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", E.Cause(err, "create client certificate")
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return string(certificatePEM), string(keyPEM), nil
}

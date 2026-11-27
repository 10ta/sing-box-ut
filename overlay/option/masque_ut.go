package option

import "github.com/sagernet/sing/common/json/badoption"

// ut: MASQUEWARPOptions turns a masque-client endpoint into a Cloudflare WARP
// client (Xray masqueSettings.warp). The keys and addresses come from a WARP
// or Zero Trust registration, e.g. usque's config.json:
//
//	"warp": {
//	  "private_key": "<usque private_key>",
//	  "public_key":  "<usque endpoint_pub_key>",
//	  "address":     ["<usque ipv4>/32", "<usque ipv6>/128"]
//	}
//
// PrivateKey is an ECDSA P-256 key, base64 DER (SEC 1 or PKCS #8) or PEM; it
// signs the client certificate Cloudflare checks (mTLS). PublicKey is the
// endpoint's key, base64 DER (PKIX) or PEM; the server certificate must carry
// it, in place of a CA check. Address holds at most one IPv4 and one IPv6
// address, a bare address meaning /32 or /128; the tunnel uses them at once,
// without waiting for the server to assign any. Host is the CONNECT-IP
// authority, default "cloudflareaccess.com".
//
// HTTP/3 only for now; see protocol/masque/warp_ut.go for what is fixed or
// refused in this mode.
type MASQUEWARPOptions struct {
	PrivateKey string                     `json:"private_key,omitempty"`
	PublicKey  string                     `json:"public_key,omitempty"`
	Address    badoption.Listable[string] `json:"address,omitempty"`
	Host       string                     `json:"host,omitempty"`
}

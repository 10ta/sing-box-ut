package v2rayxhttp

import (
	"github.com/sagernet/sing-box/common/quicparams"
	"github.com/sagernet/sing-box/option"
)

// ut: quic_params, the QUIC congestion control of XHTTP over HTTP/3 (see
// option.QUICParamsOptions). Parsed at load for every HTTP version so a bad
// value fails the config; installed on each QUIC connection by
// quicCongestion.install (quic_params_quic_ut.go). The parsing and the
// controllers live in common/quicparams, shared with masque-client.
type quicCongestion struct {
	quicparams.Congestion
}

// quicCongestionFor parses quic_params; when the transport does not use HTTP/3
// a set congestion is ignored with a warning.
func quicCongestionFor(version httpVersion, options *option.V2RayXHTTPQUICParamsOptions, warn func(string)) (quicCongestion, error) {
	congestion, err := quicparams.Parse("v2ray-xhttp", options)
	if err != nil {
		return quicCongestion{}, err
	}
	if congestion.Enabled() && version != httpVersion3 {
		warn(`quic_params only applies to HTTP/3 (tls.alpn ["h3"]), ignored`)
		return quicCongestion{}, nil
	}
	return quicCongestion{congestion}, nil
}

package v2rayxhttp

import (
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
)

// ut: quic_params, the QUIC congestion control of XHTTP over HTTP/3 (see
// option.V2RayXHTTPQUICParamsOptions). Parsed at load for every HTTP version so
// a bad value fails the config; installed on each QUIC connection by
// quicCongestion.install (quic_params_quic_ut.go).

const (
	quicCongestionDefault = ""
	quicCongestionBBR     = "bbr"
	quicCongestionBrutal  = "brutal"

	// quicMbpsToBps is Hysteria's MbpsToBps: the brutal sender counts bytes.
	quicMbpsToBps = 125000
)

type quicCongestion struct {
	kind       string
	bbrProfile string
	sendBPS    uint64
	debug      bool
}

// quicCongestionFor parses quic_params; when the transport does not use HTTP/3
// a set congestion is ignored with a warning.
func quicCongestionFor(version httpVersion, options *option.V2RayXHTTPQUICParamsOptions, warn func(string)) (quicCongestion, error) {
	congestion, err := parseQUICParams(options)
	if err != nil {
		return quicCongestion{}, err
	}
	if congestion.kind != quicCongestionDefault && version != httpVersion3 {
		warn(`quic_params only applies to HTTP/3 (tls.alpn ["h3"]), ignored`)
		return quicCongestion{}, nil
	}
	return congestion, nil
}

func parseQUICParams(options *option.V2RayXHTTPQUICParamsOptions) (quicCongestion, error) {
	if options == nil {
		return quicCongestion{}, nil
	}
	switch options.Congestion {
	case "":
		if options.UpMbps != 0 || options.BBRProfile != "" || options.BrutalDebug {
			return quicCongestion{}, E.New("v2ray-xhttp: quic_params: up_mbps, bbr_profile and brutal_debug need a congestion")
		}
		return quicCongestion{}, nil
	case quicCongestionBBR:
		if options.UpMbps != 0 || options.BrutalDebug {
			return quicCongestion{}, E.New("v2ray-xhttp: quic_params: up_mbps and brutal_debug only apply to congestion brutal")
		}
		profile := options.BBRProfile
		switch profile {
		case "":
			profile = "standard"
		case "standard", "conservative", "aggressive":
		default:
			return quicCongestion{}, E.New("v2ray-xhttp: quic_params: unsupported bbr_profile: ", profile)
		}
		return quicCongestion{kind: quicCongestionBBR, bbrProfile: profile}, nil
	case quicCongestionBrutal, "force-brutal":
		if options.BBRProfile != "" {
			return quicCongestion{}, E.New("v2ray-xhttp: quic_params: bbr_profile only applies to congestion bbr")
		}
		if options.UpMbps <= 0 {
			return quicCongestion{}, E.New("v2ray-xhttp: quic_params: congestion brutal needs up_mbps")
		}
		return quicCongestion{kind: quicCongestionBrutal, sendBPS: uint64(options.UpMbps) * quicMbpsToBps, debug: options.BrutalDebug}, nil
	default:
		return quicCongestion{}, E.New("v2ray-xhttp: quic_params: unsupported congestion: ", options.Congestion)
	}
}

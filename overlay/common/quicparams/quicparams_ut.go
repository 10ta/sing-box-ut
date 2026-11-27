// Package quicparams parses option.QUICParamsOptions (quic_params) and
// installs the chosen congestion control on a QUIC connection, the way
// sing-quic's Hysteria 2 client does after its handshake.
//
// ut: shared by the XHTTP transport and the masque-client endpoint.
package quicparams

import (
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
)

const (
	Default = ""
	BBR     = "bbr"
	Brutal  = "brutal"

	// mbpsToBps is Hysteria's MbpsToBps: the brutal sender counts bytes.
	mbpsToBps = 125000
)

// Congestion is a parsed quic_params. The zero value keeps quic-go's own
// controller.
type Congestion struct {
	kind       string
	bbrProfile string
	sendBPS    uint64
	debug      bool
}

func (c Congestion) Kind() string       { return c.kind }
func (c Congestion) BBRProfile() string { return c.bbrProfile }
func (c Congestion) SendBPS() uint64    { return c.sendBPS }
func (c Congestion) Debug() bool        { return c.debug }

// Enabled reports whether a congestion control other than quic-go's default
// was chosen.
func (c Congestion) Enabled() bool { return c.kind != Default }

// Parse checks options; errors start with prefix ("v2ray-xhttp",
// "masque-client"). nil options are the default.
func Parse(prefix string, options *option.QUICParamsOptions) (Congestion, error) {
	if options == nil {
		return Congestion{}, nil
	}
	fail := func(message ...any) (Congestion, error) {
		return Congestion{}, E.New(append([]any{prefix, ": quic_params: "}, message...)...)
	}
	switch options.Congestion {
	case "":
		if options.UpMbps != 0 || options.BBRProfile != "" || options.BrutalDebug {
			return fail("up_mbps, bbr_profile and brutal_debug need a congestion")
		}
		return Congestion{}, nil
	case BBR:
		if options.UpMbps != 0 || options.BrutalDebug {
			return fail("up_mbps and brutal_debug only apply to congestion brutal")
		}
		profile := options.BBRProfile
		switch profile {
		case "":
			profile = "standard"
		case "standard", "conservative", "aggressive":
		default:
			return fail("unsupported bbr_profile: ", profile)
		}
		return Congestion{kind: BBR, bbrProfile: profile}, nil
	case Brutal, "force-brutal":
		if options.BBRProfile != "" {
			return fail("bbr_profile only applies to congestion bbr")
		}
		if options.UpMbps <= 0 {
			return fail("congestion brutal needs up_mbps")
		}
		return Congestion{kind: Brutal, sendBPS: uint64(options.UpMbps) * mbpsToBps, debug: options.BrutalDebug}, nil
	default:
		return fail("unsupported congestion: ", options.Congestion)
	}
}

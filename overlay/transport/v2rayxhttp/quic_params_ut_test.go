package v2rayxhttp

import (
	"strings"
	"testing"

	"github.com/sagernet/sing-box/common/quicparams"
	"github.com/sagernet/sing-box/option"
)

// The values themselves are tested in common/quicparams. Here: the XHTTP
// prefix, and what happens outside HTTP/3.

func TestQUICCongestionForErrorPrefix(t *testing.T) {
	_, err := quicCongestionFor(httpVersion3, &option.V2RayXHTTPQUICParamsOptions{Congestion: "cubic"}, func(string) {})
	if err == nil || !strings.HasPrefix(err.Error(), "v2ray-xhttp: quic_params: unsupported congestion") {
		t.Fatalf("err = %v", err)
	}
}

// TestQUICCongestionForOtherHTTPVersions: outside HTTP/3 a set congestion is
// dropped with one warning; a bad value still fails.
func TestQUICCongestionForOtherHTTPVersions(t *testing.T) {
	brutal := &option.V2RayXHTTPQUICParamsOptions{Congestion: "brutal", UpMbps: 60}
	for _, version := range []httpVersion{httpVersion11, httpVersion2} {
		var warnings []string
		got, err := quicCongestionFor(version, brutal, func(message string) { warnings = append(warnings, message) })
		if err != nil || got != (quicCongestion{}) || len(warnings) != 1 {
			t.Fatalf("%s: got %+v, %v, warnings %q", version, got, err, warnings)
		}
		if _, err := quicCongestionFor(version, &option.V2RayXHTTPQUICParamsOptions{Congestion: "brutal"}, func(string) {}); err == nil {
			t.Fatalf("%s: a bad quic_params loaded", version)
		}
	}
	var warnings []string
	got, err := quicCongestionFor(httpVersion3, brutal, func(message string) { warnings = append(warnings, message) })
	if err != nil || got.Kind() != quicparams.Brutal || got.SendBPS() != 60*125000 || len(warnings) != 0 {
		t.Fatalf("h3: got %+v, %v, warnings %q", got, err, warnings)
	}
}

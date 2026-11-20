package v2rayxhttp

import (
	"strings"
	"testing"

	"github.com/sagernet/sing-box/option"
)

func TestParseQUICParams(t *testing.T) {
	for _, test := range []struct {
		name    string
		options *option.V2RayXHTTPQUICParamsOptions
		want    quicCongestion
		err     string
	}{
		{name: "absent"},
		{name: "empty", options: &option.V2RayXHTTPQUICParamsOptions{}},
		{name: "bbr", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "bbr"}, want: quicCongestion{kind: quicCongestionBBR, bbrProfile: "standard"}},
		{name: "bbr profile", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "bbr", BBRProfile: "aggressive"}, want: quicCongestion{kind: quicCongestionBBR, bbrProfile: "aggressive"}},
		{name: "brutal", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "brutal", UpMbps: 60, BrutalDebug: true}, want: quicCongestion{kind: quicCongestionBrutal, sendBPS: 60 * 125000, debug: true}},
		{name: "xray name", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "force-brutal", UpMbps: 8}, want: quicCongestion{kind: quicCongestionBrutal, sendBPS: 1000000}},
		{name: "brutal without rate", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "brutal"}, err: "needs up_mbps"},
		{name: "negative rate", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "brutal", UpMbps: -1}, err: "needs up_mbps"},
		{name: "rate without brutal", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "bbr", UpMbps: 60}, err: "only apply to congestion brutal"},
		{name: "rate alone", options: &option.V2RayXHTTPQUICParamsOptions{UpMbps: 60}, err: "need a congestion"},
		{name: "profile with brutal", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "brutal", UpMbps: 60, BBRProfile: "standard"}, err: "only applies to congestion bbr"},
		{name: "bad profile", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "bbr", BBRProfile: "turbo"}, err: "unsupported bbr_profile"},
		{name: "bad congestion", options: &option.V2RayXHTTPQUICParamsOptions{Congestion: "cubic"}, err: "unsupported congestion"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseQUICParams(test.options)
			if test.err != "" {
				if err == nil || !strings.Contains(err.Error(), test.err) {
					t.Fatalf("err = %v, want %q", err, test.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
		})
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
	if err != nil || got.kind != quicCongestionBrutal || len(warnings) != 0 {
		t.Fatalf("h3: got %+v, %v, warnings %q", got, err, warnings)
	}
}

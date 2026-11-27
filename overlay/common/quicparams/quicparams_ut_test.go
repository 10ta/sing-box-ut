package quicparams

import (
	"strings"
	"testing"

	"github.com/sagernet/sing-box/option"
)

func TestParse(t *testing.T) {
	for _, test := range []struct {
		name    string
		options *option.QUICParamsOptions
		want    Congestion
		err     string
	}{
		{name: "absent"},
		{name: "empty", options: &option.QUICParamsOptions{}},
		{name: "bbr", options: &option.QUICParamsOptions{Congestion: "bbr"}, want: Congestion{kind: BBR, bbrProfile: "standard"}},
		{name: "bbr profile", options: &option.QUICParamsOptions{Congestion: "bbr", BBRProfile: "aggressive"}, want: Congestion{kind: BBR, bbrProfile: "aggressive"}},
		{name: "brutal", options: &option.QUICParamsOptions{Congestion: "brutal", UpMbps: 60, BrutalDebug: true}, want: Congestion{kind: Brutal, sendBPS: 60 * 125000, debug: true}},
		{name: "xray name", options: &option.QUICParamsOptions{Congestion: "force-brutal", UpMbps: 8}, want: Congestion{kind: Brutal, sendBPS: 1000000}},
		{name: "brutal without rate", options: &option.QUICParamsOptions{Congestion: "brutal"}, err: "needs up_mbps"},
		{name: "negative rate", options: &option.QUICParamsOptions{Congestion: "brutal", UpMbps: -1}, err: "needs up_mbps"},
		{name: "rate without brutal", options: &option.QUICParamsOptions{Congestion: "bbr", UpMbps: 60}, err: "only apply to congestion brutal"},
		{name: "rate alone", options: &option.QUICParamsOptions{UpMbps: 60}, err: "need a congestion"},
		{name: "profile with brutal", options: &option.QUICParamsOptions{Congestion: "brutal", UpMbps: 60, BBRProfile: "standard"}, err: "only applies to congestion bbr"},
		{name: "bad profile", options: &option.QUICParamsOptions{Congestion: "bbr", BBRProfile: "turbo"}, err: "unsupported bbr_profile"},
		{name: "bad congestion", options: &option.QUICParamsOptions{Congestion: "cubic"}, err: "unsupported congestion"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Parse("test", test.options)
			if test.err != "" {
				if err == nil || !strings.Contains(err.Error(), test.err) || !strings.HasPrefix(err.Error(), "test: quic_params: ") {
					t.Fatalf("err = %v, want %q with the prefix", err, test.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
			if got.Enabled() != (test.want.kind != Default) {
				t.Fatalf("Enabled() = %v", got.Enabled())
			}
		})
	}
}

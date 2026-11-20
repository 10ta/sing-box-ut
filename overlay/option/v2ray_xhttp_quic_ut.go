package option

// ut: V2RayXHTTPQUICParamsOptions picks the QUIC congestion control of XHTTP
// over HTTP/3, the congestion part of Xray's finalmask.quicParams. It only
// changes how fast this client sends, so on the upload side it speeds up the
// upload and nothing else; on a download endpoint the client only sends
// requests and acknowledgements. The server needs no change.
//
//	"transport": {
//	  "type": "xhttp", ...,
//	  "quic_params": { "congestion": "brutal", "up_mbps": 60 }
//	}
//
// Congestion is empty (quic-go's default, as before), "bbr" or "brutal"
// ("force-brutal", Xray's name, is accepted too). Brutal sends at UpMbps
// whatever the loss, like Hysteria 2; set it to the real upload bandwidth.
type V2RayXHTTPQUICParamsOptions struct {
	Congestion  string `json:"congestion,omitempty" enum:"bbr,brutal,force-brutal"`
	BBRProfile  string `json:"bbr_profile,omitempty" enum:"standard,conservative,aggressive"`
	UpMbps      int    `json:"up_mbps,omitempty"`
	BrutalDebug bool   `json:"brutal_debug,omitempty"`
}

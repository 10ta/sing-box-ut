package option

// ut: QUICParamsOptions picks the QUIC congestion control of a client
// connection over HTTP/3, the congestion part of Xray's finalmask.quicParams.
// Used by the XHTTP transport and the masque-client endpoint. It only changes
// how fast this client sends, so it speeds up the upload and nothing else. The
// server needs no change.
//
//	"quic_params": { "congestion": "brutal", "up_mbps": 60 }
//
// Congestion is empty (quic-go's default), "bbr" or "brutal" ("force-brutal",
// Xray's name, is accepted too). Brutal sends at UpMbps whatever the loss, like
// Hysteria 2; set it to the real upload bandwidth.
type QUICParamsOptions struct {
	Congestion  string `json:"congestion,omitempty" enum:"bbr,brutal,force-brutal"`
	BBRProfile  string `json:"bbr_profile,omitempty" enum:"standard,conservative,aggressive"`
	UpMbps      int    `json:"up_mbps,omitempty"`
	BrutalDebug bool   `json:"brutal_debug,omitempty"`
}

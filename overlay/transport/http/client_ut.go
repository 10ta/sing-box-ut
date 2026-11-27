package http

import (
	"github.com/sagernet/sing-box/common/quicparams"
	"github.com/sagernet/sing/common/logger"
)

// ut: HTTP3UTOptions are the HTTP/3 additions of sing-box-ut, set by
// masque-client (quic_params, and Cloudflare WARP). The zero value is
// upstream's behaviour.
type HTTP3UTOptions struct {
	// ConnectionIDLength of the client's QUIC connection IDs; 0 = quic-go's
	// default (4). Cloudflare's WARP edge wants 20 (usque, Xray), or it
	// sometimes closes the connection with PROTOCOL_VIOLATION.
	ConnectionIDLength int
	// AdditionalSettings go into the client's HTTP/3 SETTINGS frame.
	AdditionalSettings map[uint64]uint64
	// DraftDatagrams accepts a server that announces HTTP Datagrams only with
	// the draft-00 setting (0x276), as Cloudflare does, instead of
	// SETTINGS_H3_DATAGRAM; datagrams of a tunnel are then read straight off
	// the QUIC connection.
	DraftDatagrams bool
	// Congestion is installed on every QUIC connection (quic_params).
	Congestion quicparams.Congestion
	// Logger serves brutal_debug; may be nil.
	Logger logger.Logger
}

// SettingDatagramDraft00 is SETTINGS_H3_DATAGRAM from
// draft-ietf-masque-h3-datagram-00, which Cloudflare's WARP edge and official
// client still exchange.
const SettingDatagramDraft00 uint64 = 0x276

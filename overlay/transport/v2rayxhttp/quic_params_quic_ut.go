//go:build with_xhttp && with_quic

package v2rayxhttp

import (
	"github.com/sagernet/quic-go"
	congestion_meta2 "github.com/sagernet/sing-quic/congestion_meta2"
	hyCC "github.com/sagernet/sing-quic/hysteria/congestion"
	"github.com/sagernet/sing/common/logger"
)

// install sets the congestion control on a freshly dialed QUIC connection, the
// way sing-quic's Hysteria 2 client does after its handshake. The default
// leaves quic-go's own controller in place.
func (c quicCongestion) install(conn *quic.Conn, logger logger.Logger) {
	switch c.kind {
	case quicCongestionBBR:
		profile, err := congestion_meta2.ParseProfile(c.bbrProfile)
		if err != nil {
			// parseQUICParams already accepted only known profiles.
			profile = congestion_meta2.ProfileStandard
		}
		conn.SetCongestionControl(congestion_meta2.NewBbrSenderWithProfile(conn.InitialPacketSize(), profile))
	case quicCongestionBrutal:
		// brutal_debug writes through the logger; a transport built without a log
		// factory (tests, some embedders) has none.
		conn.SetCongestionControl(hyCC.NewBrutalSender(c.sendBPS, conn.InitialPacketSize(), c.debug && logger != nil, logger))
	}
}

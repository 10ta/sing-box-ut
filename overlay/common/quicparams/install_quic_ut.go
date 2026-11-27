//go:build with_quic

package quicparams

import (
	"github.com/sagernet/quic-go"
	congestion_meta2 "github.com/sagernet/sing-quic/congestion_meta2"
	hyCC "github.com/sagernet/sing-quic/hysteria/congestion"
	"github.com/sagernet/sing/common/logger"
)

// Install sets the congestion control on a freshly dialed QUIC connection. The
// default leaves quic-go's own controller in place. logger may be nil; then
// brutal_debug logs nothing.
func (c Congestion) Install(conn *quic.Conn, logger logger.Logger) {
	switch c.kind {
	case BBR:
		profile, err := congestion_meta2.ParseProfile(c.bbrProfile)
		if err != nil {
			// Parse already accepted only known profiles.
			profile = congestion_meta2.ProfileStandard
		}
		conn.SetCongestionControl(congestion_meta2.NewBbrSenderWithProfile(conn.InitialPacketSize(), profile))
	case Brutal:
		conn.SetCongestionControl(hyCC.NewBrutalSender(c.sendBPS, conn.InitialPacketSize(), c.debug && logger != nil, logger))
	}
}

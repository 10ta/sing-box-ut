//go:build with_xhttp && with_quic

package v2rayxhttp

import (
	"github.com/sagernet/quic-go"
	"github.com/sagernet/sing/common/logger"
)

// install sets the congestion control on a freshly dialed QUIC connection
// (common/quicparams). The default leaves quic-go's own controller in place.
func (c quicCongestion) install(conn *quic.Conn, logger logger.Logger) {
	c.Install(conn, logger)
}

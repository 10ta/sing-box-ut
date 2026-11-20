//go:build !with_xhttp || !with_quic

package v2rayxhttp

import (
	"time"

	"github.com/sagernet/sing-box/common/lxsniffguard"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// ut: without quic-go there is no HTTP/3 and nothing to install quic_params on;
// lx's stub refuses the config.
func newHTTP3TransportUT(dialer N.Dialer, serverAddr M.Socksaddr, tlsConfig tls.Config, keepAlivePeriod time.Duration, _ quicCongestion, warn func(string), sniffGuard *lxsniffguard.Node, logger logger.Logger) (func() xmuxConn, error) {
	return newHTTP3Transport(dialer, serverAddr, tlsConfig, keepAlivePeriod, warn, sniffGuard, logger)
}

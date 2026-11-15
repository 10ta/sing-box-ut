// Package lxdatagram is a stub of sing-box-lx's package of the same name.
//
// ut: the only call the XHTTP transport makes is GuardQUIC, which applies
// lx.sniff_guard to an HTTP/3 handshake. With no guard (see the lxsniffguard
// stub) sing-box-lx returns the conn untouched and a nil handshake; so does
// this stub.
package lxdatagram

import (
	"net"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/lxsniffguard"
	"github.com/sagernet/sing/common/logger"
)

// QUICHandshake is the guarded form of one QUIC handshake; always nil here.
type QUICHandshake struct{}

// GuardQUIC returns conn untouched, as sing-box-lx does for a nil attempt.
func GuardQUIC(_ *lxsniffguard.Attempt, _ any, _ *adapter.CarrierRecord, conn net.Conn, _ logger.Logger) (net.Conn, *QUICHandshake) {
	return conn, nil
}

// Done does nothing for a nil handshake.
func (h *QUICHandshake) Done() {}

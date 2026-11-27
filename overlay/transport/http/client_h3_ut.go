//go:build with_quic

package http

import (
	"context"
	"net"

	"github.com/sagernet/quic-go"
	"github.com/sagernet/quic-go/quicvarint"
	"github.com/sagernet/sing-quic"
	"github.com/sagernet/sing/common/bufio"
)

// ut: dialQUICUT dials the QUIC connection of an HTTP/3 client. With the
// zero HTTP3UTOptions it is upstream's qtls.DialEarly. A connection ID length
// needs a quic.Transport of our own (qtls has no knob for it); it lives as
// long as the connection.
func (c *http3ClientImpl) dialQUICUT(ctx context.Context, rawConn net.Conn) (*quic.Conn, error) {
	var (
		quicConn *quic.Conn
		err      error
	)
	if c.ut.ConnectionIDLength == 0 {
		quicConn, err = qtls.DialEarly(ctx, rawConn, c.tlsConfig, c.quicConfig)
	} else {
		quicConn, err = c.dialQUICTransportUT(ctx, rawConn)
	}
	if err != nil {
		return nil, err
	}
	c.ut.Congestion.Install(quicConn, c.ut.Logger)
	return quicConn, nil
}

func (c *http3ClientImpl) dialQUICTransportUT(ctx context.Context, rawConn net.Conn) (*quic.Conn, error) {
	stdConfig, err := c.tlsConfig.STDConfig()
	if err != nil {
		return nil, err
	}
	quicConfig := c.quicConfig.Clone()
	if timeout := c.tlsConfig.HandshakeTimeout(); timeout > 0 && quicConfig.HandshakeIdleTimeout == 0 {
		quicConfig.HandshakeIdleTimeout = timeout
	}
	transport := &quic.Transport{
		Conn:               bufio.NewUnbindPacketConn(rawConn),
		ConnectionIDLength: c.ut.ConnectionIDLength,
	}
	quicConn, err := transport.DialEarly(ctx, rawConn.RemoteAddr(), stdConfig, quicConfig)
	if err != nil {
		transport.Close()
		return nil, qtls.WrapError(err)
	}
	context.AfterFunc(quicConn.Context(), func() {
		transport.Close()
	})
	return quicConn, nil
}

// tunnelStreamUT wraps the request stream of an established tunnel. With
// DraftDatagrams and a server that announced only the draft-00 setting, quic-go
// does not route datagrams to the stream, so they are read off the QUIC
// connection here.
func (c *http3ClientImpl) tunnelStreamUT(base *http3RequestDatagramStream, conn *http3Conn) DatagramStream {
	settings := conn.Settings()
	if settings.EnableDatagrams || !c.ut.DraftDatagrams || settings.Other[SettingDatagramDraft00] != 1 || conn.quicConn == nil {
		return base
	}
	base.datagramsEnabled = true
	return &http3DraftDatagramStream{http3RequestDatagramStream: base, conn: conn.quicConn}
}

type http3DraftDatagramStream struct {
	*http3RequestDatagramStream
	conn *quic.Conn
}

func (s *http3DraftDatagramStream) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	streamID := uint64(s.stream.StreamID())
	for {
		datagram, err := s.conn.ReceiveDatagram(ctx)
		if err != nil {
			return nil, err
		}
		quarterStreamID, n, err := quicvarint.Parse(datagram)
		if err != nil || quarterStreamID*4 != streamID {
			continue
		}
		return datagram[n:], nil
	}
}

var _ DatagramStream = (*http3DraftDatagramStream)(nil)

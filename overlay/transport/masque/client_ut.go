package masque

import (
	"net/netip"
	"slices"
)

// ut: ClientUTOptions are the CONNECT-IP additions of sing-box-ut, set by
// masque-client in Cloudflare WARP mode. The zero value is upstream's
// behaviour.
type ClientUTOptions struct {
	// Protocol is the :protocol of the CONNECT request; "" = "connect-ip"
	// (RFC 9484). Cloudflare's WARP edge wants "cf-connect-ip".
	Protocol string
	// Address, when set, is the tunnel's local addresses: the session is ready
	// as soon as the stream is open, and no ADDRESS_REQUEST is sent (WARP gives
	// the addresses at registration and does not answer one). An
	// ADDRESS_ASSIGN the server still sends replaces them, as upstream.
	Address []netip.Prefix
}

// CloudflareProtocol is the :protocol Cloudflare's WARP edge expects.
const CloudflareProtocol = "cf-connect-ip"

func (o ClientUTOptions) protocol() string {
	if o.Protocol == "" {
		return upgradeToken
	}
	return o.Protocol
}

// requestAddressesUT starts a session: upstream asks for one IPv4 and one IPv6
// address; with static addresses the session is configured right away.
func (s *clientSession) requestAddressesUT() error {
	addresses := s.client.ut.Address
	if len(addresses) == 0 {
		return s.writeCapsule(newAddressCapsule(capsuleTypeAddressRequest, []AssignedAddress{
			{RequestID: 1, Prefix: netip.PrefixFrom(netip.IPv4Unspecified(), 32)},
			{RequestID: 2, Prefix: netip.PrefixFrom(netip.IPv6Unspecified(), 128)},
		}))
	}
	s.access.Lock()
	s.configuration.Address = slices.Clone(addresses)
	configuration := s.configuration
	s.access.Unlock()
	return s.updateConfiguration(configuration)
}

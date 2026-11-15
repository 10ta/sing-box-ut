package adapter

// ut: the part of sing-box-lx's adapter/carrier_lx.go (SPEC 060) the XHTTP
// transport calls. sing-box-lx fills the record from its detour dialer and
// chain groups and reads it for lx.sniff_guard, which this project does not
// have (see common/lxsniffguard); here the record is carried but never filled.

import "context"

type carrierRecordKey struct{}

// CarrierRecord is the outbound a dial went through. Never filled here.
type CarrierRecord struct {
	Type string
	Tag  string
}

// ContextWithCarrierRecord attaches a fresh record to ctx and returns it.
func ContextWithCarrierRecord(ctx context.Context) (context.Context, *CarrierRecord) {
	record := &CarrierRecord{}
	return context.WithValue(ctx, carrierRecordKey{}, record), record
}

// ContextWithExistingCarrierRecord attaches record to ctx; nil leaves ctx as is.
func ContextWithExistingCarrierRecord(ctx context.Context, record *CarrierRecord) context.Context {
	if record == nil {
		return ctx
	}
	return context.WithValue(ctx, carrierRecordKey{}, record)
}

// CarrierRecordFromContext returns the record in ctx, or nil.
func CarrierRecordFromContext(ctx context.Context) *CarrierRecord {
	record, _ := ctx.Value(carrierRecordKey{}).(*CarrierRecord)
	return record
}

// OutboundTagFromContext returns "": sing-box-lx's outbound registry sets the
// tag only for lx.sniff_guard, which is not here.
func OutboundTagFromContext(context.Context) string { return "" }

// Package lxsniffguard is a stub of sing-box-lx's package of the same name.
//
// ut: sing-box-lx's lx.sniff_guard (SPEC 060) needs lx's own top-level `lx`
// config section, which lives in upstream files this project does not replace.
// The XHTTP transport only reaches it through the calls below, and sing-box-lx
// treats a nil guard, node and attempt as "feature off": every call then
// behaves as upstream. These stubs always return nil, so XHTTP runs exactly as
// it does in sing-box-lx with lx.sniff_guard unset.
package lxsniffguard

import "context"

// Guard is the lx.sniff_guard block. Never built here.
type Guard struct{}

// Node is the guard's handle of one node; always nil here.
type Node struct{}

// Attempt is one guarded dial; always nil here.
type Attempt struct{}

// FromContext returns nil: no guard is configured.
func FromContext(context.Context) *Guard { return nil }

// Node returns nil: no node is guarded.
func (g *Guard) Node(string) *Node { return nil }

// Dial runs do, as sing-box-lx does for a nil node.
func (n *Node) Dial(ctx context.Context, do func(ctx context.Context) error) error { return do(ctx) }

// AttemptFromContext returns nil: no dial is guarded.
func AttemptFromContext(context.Context) *Attempt { return nil }

// ContextWithAttempt returns ctx unchanged.
func ContextWithAttempt(ctx context.Context, _ *Attempt) context.Context { return ctx }

// Package source is the boundary between this module and wherever a schedule
// actually comes from.
//
// # Why the boundary exists before any origin does
//
// The phase that produced it was blocked on one unresolved question — where
// Aegean sailing times may legitimately be obtained from — and was explicitly
// forbidden from answering it by inventing rows. That is not a reason to leave
// a hole where the seam belongs. It is a reason to build the seam and ship the
// one implementation that is honest under the constraint: an origin that
// returns an empty schedule.
//
// The empty origin is not a placeholder in the apologetic sense. It is a real,
// total implementation of the contract, it is exercised by the same tests every
// other origin will be, and it lets the whole pipeline below it — validation,
// graph construction, routing — be built and proven before any row exists.
// Building the consumer first and the producer second is what keeps invented
// data out: nobody ever needs to populate something merely to get the code to
// run.
//
// # What an origin may do
//
// Nothing here opens a network connection, reads a credential or names a
// supplier, and no implementation added later should change that about this
// package — an origin that talks to the outside world implements the interface
// from its own package, where its dependencies and its terms of use are visible
// at its own import list rather than hidden behind this one.
package source

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pigfox/aegean-solver/internal/ferry"
)

var (
	// ErrNilFeed is an origin that reported success and returned nothing. It is
	// a defect in the origin, not in its input, which is why it is separate
	// from a validation failure.
	ErrNilFeed = errors.New("source: origin returned a nil feed with no error")
	// ErrInvalidFeed is an origin that returned a feed the schema refuses.
	ErrInvalidFeed = errors.New("source: origin returned an invalid feed")
)

// Description says what an origin is, for a log line or a status page.
type Description struct {
	// Name identifies the origin. It is a description of a MECHANISM — "empty
	// origin", "local archive" — and never the name of a company or a service.
	Name string
	// Populated reports whether this origin is capable of returning rows at
	// all. It exists so a caller can tell "there is no connection between these
	// two ports" apart from "there is no schedule loaded", which are the same
	// answer from the solver and completely different answers to a user.
	Populated bool
}

// Source is the contract every schedule origin satisfies.
//
// Feed returns the whole schedule. It takes a context because a real origin
// will do slow work; the empty origin honors cancellation anyway, so that a
// caller which handles cancellation correctly against the empty origin is
// handling it correctly against every other one.
type Source interface {
	Describe() Description
	Feed(ctx context.Context) (*ferry.Feed, error)
}

// Load reads from an origin and enforces the boundary's postcondition: what
// comes back is non-nil and passes ferry.Feed.Validate.
//
// IT IS A PACKAGE FUNCTION AND NOT A METHOD, deliberately. If validating were
// left to each implementation, every origin would have to remember, and the one
// that forgot would be the one whose rows were worst. Here it cannot be
// skipped: an origin returns what it read, and the boundary decides whether
// that is admissible.
func Load(ctx context.Context, s Source) (*ferry.Feed, error) {
	feed, err := s.Feed(ctx)
	if err != nil {
		return nil, err
	}
	if feed == nil {
		return nil, fmt.Errorf("%w: %s", ErrNilFeed, s.Describe().Name)
	}
	if err := feed.Validate(); err != nil {
		return nil, fmt.Errorf("%w from %s: %w", ErrInvalidFeed, s.Describe().Name, err)
	}
	return feed, nil
}

// Empty is the null origin: it returns a valid schedule containing nothing.
//
// This is the module's ONLY implementation of Source, and shipping exactly one
// is the point rather than a shortfall. See the package comment.
type Empty struct {
	// Location is the zone the empty schedule declares. Nil means UTC. It is
	// settable because a caller testing a downstream consumer usually wants the
	// zone that consumer will really run in, and an empty feed is the cheapest
	// way to give it one.
	Location *time.Location
}

// Describe names the mechanism.
func (Empty) Describe() Description {
	return Description{Name: "empty origin", Populated: false}
}

// Feed returns an empty schedule, or the context's error if the caller has
// already given up.
func (e Empty) Feed(ctx context.Context) (*ferry.Feed, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &ferry.Feed{Location: e.Location}, nil
}

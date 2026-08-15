package source

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pigfox/aegean-solver/ferry"
)

// stub is a Source that returns exactly what a test tells it to. It exists so
// the boundary's postcondition can be checked against origins that misbehave in
// each of the three available ways, none of which the one shipped
// implementation can produce.
type stub struct {
	feed *ferry.Feed
	err  error
}

func (stub) Describe() Description { return Description{Name: "stub origin", Populated: true} }

func (s stub) Feed(context.Context) (*ferry.Feed, error) { return s.feed, s.err }

func TestEmptyOriginDescribesAMechanism(t *testing.T) {
	t.Parallel()

	d := Empty{}.Describe()
	if d.Name == "" {
		t.Fatal("the origin did not name itself")
	}
	// Populated is the field that lets a caller tell "these ports are not
	// connected" apart from "no schedule is loaded". They are the same answer
	// from the solver and completely different answers to a person.
	if d.Populated {
		t.Fatal("the empty origin reported itself populated")
	}
}

func TestEmptyOriginReturnsAValidEmptyFeed(t *testing.T) {
	t.Parallel()

	feed, err := Load(context.Background(), Empty{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(feed.Ports) != 0 || len(feed.Trips) != 0 {
		t.Fatalf("the empty origin returned %d ports and %d trips", len(feed.Ports), len(feed.Trips))
	}
	if got := feed.Loc(); got != time.UTC {
		t.Fatalf("Loc() = %v, want UTC", got)
	}
}

func TestEmptyOriginCarriesADeclaredZone(t *testing.T) {
	t.Parallel()

	zone := time.FixedZone("plus-three", 3*60*60)
	feed, err := Load(context.Background(), Empty{Location: zone})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := feed.Loc(); got != zone {
		t.Fatalf("Loc() = %v, want the declared zone", got)
	}
}

// TestEmptyOriginHonorsCancellation is not ceremony. A caller that handles
// cancellation correctly against the cheapest origin is handling it correctly
// against every other one, and the only way to keep that true is for the
// cheapest origin to actually check.
func TestEmptyOriginHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := (Empty{}).Feed(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if _, err := Load(ctx, Empty{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load got %v, want context.Canceled", err)
	}
}

func TestLoadEnforcesThePostcondition(t *testing.T) {
	t.Parallel()

	failed := errors.New("the origin gave up")

	cases := []struct {
		name string
		src  Source
		want error
	}{
		{
			name: "an origin that reported a failure",
			src:  stub{err: failed},
			want: failed,
		},
		{
			name: "an origin that succeeded and returned nothing",
			src:  stub{},
			want: ErrNilFeed,
		},
		{
			name: "an origin that returned rows the schema refuses",
			src:  stub{feed: &ferry.Feed{Ports: []ferry.Port{{ID: "Alpha"}}}},
			want: ErrInvalidFeed,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			feed, err := Load(context.Background(), c.src)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want an error wrapping %v", err, c.want)
			}
			if feed != nil {
				t.Fatal("a refused load still returned a feed")
			}
		})
	}
}

// TestValidationIsAtTheBoundaryAndNotInTheOrigin pins WHY Load is a package
// function. An origin that returns rows the schema refuses is caught even
// though it never validates anything itself, which is what stops the rule from
// being something each future origin has to remember — and the one that forgot
// would be the one whose rows were worst.
func TestValidationIsAtTheBoundaryAndNotInTheOrigin(t *testing.T) {
	t.Parallel()

	bad := &ferry.Feed{Trips: []ferry.Trip{{ID: "trip-one"}}}
	direct, err := stub{feed: bad}.Feed(context.Background())
	if err != nil || direct == nil {
		t.Fatalf("premise failed: the stub itself must succeed, got %v, %v", direct, err)
	}
	if _, err := Load(context.Background(), stub{feed: bad}); !errors.Is(err, ErrInvalidFeed) {
		t.Fatalf("Load got %v, want ErrInvalidFeed", err)
	}
}

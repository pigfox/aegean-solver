package vocab

import (
	"errors"
	"testing"
)

func TestIDValid(t *testing.T) {
	t.Parallel()

	long := ID("a")
	for len(long) <= MaxIDLength {
		long += "a"
	}

	cases := []struct {
		name string
		id   ID
		want bool
	}{
		{name: "empty is refused", id: "", want: false},
		{name: "past the length ceiling is refused", id: long, want: false},
		{name: "exactly at the ceiling is accepted", id: long[:MaxIDLength], want: true},
		{name: "one lowercase letter is enough", id: "a", want: true},
		{name: "letters digits and the separator", id: "aegean-port-12", want: true},
		{name: "a trailing separator is legal", id: "port-", want: true},
		{name: "a leading digit is refused", id: "1port", want: false},
		{name: "a leading separator is refused", id: "-port", want: false},
		{name: "a leading capital is refused", id: "Port", want: false},
		{name: "a later capital is refused", id: "poRt", want: false},
		{name: "an underscore is refused", id: "port_one", want: false},
		{name: "a space is refused", id: "port one", want: false},
		{name: "a non-ASCII leading rune is refused", id: "πort", want: false},
		{name: "a non-ASCII later rune is refused", id: "poπt", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.id.Valid(); got != c.want {
				t.Fatalf("ID(%q).Valid() = %v, want %v", string(c.id), got, c.want)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "already normal", in: "piraeus", want: "piraeus"},
		{name: "case is folded", in: "PIRAEUS", want: "piraeus"},
		{name: "punctuation becomes one separator", in: "Piraeus (Port)", want: "piraeus port"},
		{name: "runs collapse", in: "Ano   ---   Meria", want: "ano meria"},
		{name: "the ends are trimmed", in: "   Adamas   ", want: "adamas"},
		{name: "digits survive", in: "Berth 12", want: "berth 12"},
		{name: "punctuation alone normalizes to nothing", in: "!!! ---", want: ""},
		{name: "empty stays empty", in: "", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeName(c.in); got != c.want {
				t.Fatalf("NormalizeName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestNormalizeNameDoesNotFoldDiacritics pins the KNOWN LIMIT the function
// documents, rather than leaving it as a sentence nobody checks.
//
// It is asserted as an INEQUALITY on purpose. If a later change adds folding,
// this test fails and the person making that change has to come here and decide
// whether the alias sets that were written to work around the limit are now
// carrying redundant entries. Silently gaining the behavior would leave every
// one of those sets looking deliberate when it had become noise.
func TestNormalizeNameDoesNotFoldDiacritics(t *testing.T) {
	t.Parallel()

	accented := NormalizeName("Πειραιάς")
	plain := NormalizeName("Πειραιας")
	if accented == plain {
		t.Fatalf("diacritics were folded: both spellings normalized to %q, "+
			"which contradicts the documented limit on NormalizeName", accented)
	}
}

func TestAliasIndexResolvesEveryRegisteredSpelling(t *testing.T) {
	t.Parallel()

	x := NewAliasIndex()
	if err := x.Add("adamas", "Adamas", "ADAMAS", "Milos (Adamas)"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := x.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2 distinct keys: two of the three spellings normalize alike", got)
	}
	for _, spelling := range []string{"adamas", "Adamas", "ADAMAS", "  adamas  ", "Milos (Adamas)"} {
		id, ok := x.Lookup(spelling)
		if !ok || id != "adamas" {
			t.Fatalf("Lookup(%q) = %q, %v; want %q, true", spelling, string(id), ok, "adamas")
		}
	}
	if _, ok := x.Lookup("somewhere else"); ok {
		t.Fatal("Lookup of an unregistered name reported a hit")
	}
}

func TestAliasIndexAddIsIdempotentForTheSameID(t *testing.T) {
	t.Parallel()

	x := NewAliasIndex()
	if err := x.Add("naxos", "Naxos"); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	// A vocabulary is usually assembled in several passes over the same rows,
	// so re-registering a name for the id that already owns it is normal and
	// must not be an error.
	if err := x.Add("naxos", "Naxos", "NAXOS"); err != nil {
		t.Fatalf("second Add of the same alias for the same id: %v", err)
	}
	if got := x.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
}

func TestAliasIndexAddRefusals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup func(*AliasIndex) error
		call  func(*AliasIndex) error
		want  error
	}{
		{
			name: "an identifier outside the grammar",
			call: func(x *AliasIndex) error { return x.Add("Naxos", "Naxos") },
			want: ErrInvalidID,
		},
		{
			name: "an alias that normalizes to nothing",
			call: func(x *AliasIndex) error { return x.Add("naxos", "-- ??") },
			want: ErrEmptyAlias,
		},
		{
			name:  "one alias claimed by two identifiers",
			setup: func(x *AliasIndex) error { return x.Add("naxos", "Chora") },
			call:  func(x *AliasIndex) error { return x.Add("ios", "Chora") },
			want:  ErrAliasCollision,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			x := NewAliasIndex()
			if c.setup != nil {
				if err := c.setup(x); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			err := c.call(x)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
}

func TestVesselClassEnvelopes(t *testing.T) {
	t.Parallel()

	for _, class := range Classes() {
		if !class.Valid() {
			t.Fatalf("declared class %q reports itself invalid", string(class))
		}
		lo, hi, ok := class.SpeedEnvelopeKnots()
		if !ok {
			t.Fatalf("declared class %q has no envelope", string(class))
		}
		if lo <= 0 {
			t.Fatalf("class %q lower bound is %v, which is not a speed", string(class), lo)
		}
		if hi <= lo {
			t.Fatalf("class %q envelope is %v..%v, which is not a band", string(class), lo, hi)
		}
	}
}

func TestUnknownVesselClassHasNoEnvelope(t *testing.T) {
	t.Parallel()

	unknown := VesselClass("submarine")
	if unknown.Valid() {
		t.Fatal("an undeclared class reported itself valid")
	}
	lo, hi, ok := unknown.SpeedEnvelopeKnots()
	if ok || lo != 0 || hi != 0 {
		t.Fatalf("SpeedEnvelopeKnots on an undeclared class = %v, %v, %v; want 0, 0, false", lo, hi, ok)
	}
}

// TestClassOrderAndEnvelopesAgree is the guard the duplication in constants.go
// is made safe by. The stable order and the envelope table are two declarations
// of one set, and this holds them together in BOTH directions — a class in the
// order with no envelope, or an envelope no order lists, fails here.
func TestClassOrderAndEnvelopesAgree(t *testing.T) {
	t.Parallel()

	listed := Classes()
	if len(listed) == 0 {
		t.Fatal("Classes() is empty, so this guard would pass vacuously")
	}
	if len(listed) != len(speedEnvelopeKnots) {
		t.Fatalf("Classes() has %d entries and the envelope table has %d", len(listed), len(speedEnvelopeKnots))
	}
	seen := make(map[VesselClass]struct{}, len(listed))
	for _, c := range listed {
		if _, dup := seen[c]; dup {
			t.Fatalf("class %q appears twice in the stable order", string(c))
		}
		seen[c] = struct{}{}
		if _, ok := speedEnvelopeKnots[c]; !ok {
			t.Fatalf("class %q is listed in the stable order but has no envelope", string(c))
		}
	}
	for c := range speedEnvelopeKnots {
		if _, ok := seen[c]; !ok {
			t.Fatalf("class %q has an envelope but is missing from the stable order", string(c))
		}
	}
}

func TestClassesReturnsACopy(t *testing.T) {
	t.Parallel()

	first := Classes()
	if len(first) < 2 {
		t.Fatal("need at least two classes for this to prove anything")
	}
	first[0], first[1] = first[1], first[0]

	second := Classes()
	if second[0] == first[0] {
		t.Fatal("reordering the returned slice reordered the package's own list")
	}
}

func TestExceptionTypeValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   ExceptionType
		want bool
	}{
		{name: "added", in: ServiceAdded, want: true},
		{name: "removed", in: ServiceRemoved, want: true},
		{name: "the zero value is not a default", in: 0, want: false},
		{name: "an unknown code", in: 3, want: false},
		{name: "a negative code", in: -1, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.in.Valid(); got != c.want {
				t.Fatalf("ExceptionType(%d).Valid() = %v, want %v", int(c.in), got, c.want)
			}
		})
	}
}

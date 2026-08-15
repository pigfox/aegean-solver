// Package vocab holds the identity and classification vocabulary that ports,
// operators and vessels are described with.
//
// It is a VOCABULARY and not a dataset. There is not one port, one operator and
// not one sailing in this package, and there is not meant to be. What lives
// here is the grammar of an identifier, the machinery for resolving the many
// names a place is known by onto one canonical ID, and the closed set of vessel
// classes a feed may use. Every one of those is a rule about how a row is
// SHAPED; none of them is a row.
//
// # Why identity gets its own package at all
//
// Because a port has more names than a schema row has fields. The harbor is
// called one thing, the island it serves is called another, the operator prints
// a third on a ticket, and a traveler types a fourth. Phase one could ignore
// all of that: it sited depots by coordinates, and a coordinate has no
// spelling. A routing query cannot ignore it — the query arrives as a name.
//
// So identity is a canonical ID plus an alias set, and the divergence between a
// port name and an island name is a first-class case rather than an exception
// to be special-cased later. See ferry.Port, which carries both as separate
// required fields precisely so that neither has to stand in for the other.
package vocab

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// The errors a caller can act on, as values rather than formatted strings, for
// the same reason the root package uses sentinels: a caller that has to match
// on wording is coupled to the wording.
var (
	// ErrInvalidID is an identifier outside the grammar ID.Valid describes.
	ErrInvalidID = errors.New("vocab: invalid canonical id")
	// ErrEmptyAlias is an alias that normalizes to nothing — punctuation and
	// spaces alone, or the empty string.
	ErrEmptyAlias = errors.New("vocab: alias normalizes to the empty string")
	// ErrAliasCollision is one alias claimed by two different canonical IDs.
	ErrAliasCollision = errors.New("vocab: alias claimed by two canonical ids")
)

// ID is a canonical identifier for a port, an operator, a vessel, a route, a
// trip or a service.
//
// It is a distinct type rather than a string so that the compiler catches the
// one mistake that costs the most here: passing a display name where an
// identifier belongs. A display name is what a user types and what a timetable
// prints, and it is unstable in every direction — spelling, accent, language,
// and whether it names the harbor or the island. An ID is chosen once and never
// interpreted by this module.
type ID string

// Valid reports whether the identifier is inside the grammar.
//
// The rules are: at least one character and at most MaxIDLength; the first
// character is a lowercase ASCII letter; every later character is a lowercase
// ASCII letter, an ASCII digit, or IDSeparator. Leading digits are excluded
// because an identifier that parses as a number tends to be treated as one
// somewhere downstream.
func (id ID) Valid() bool {
	if len(id) == 0 || len(id) > MaxIDLength {
		return false
	}
	for i, r := range string(id) {
		if i == 0 {
			if r < 'a' || r > 'z' {
				return false
			}
			continue
		}
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == IDSeparator:
		default:
			return false
		}
	}
	return true
}

// NormalizeName folds a display name into the key an alias lookup uses.
//
// It lowercases, replaces every run of non-alphanumeric characters with one
// space, and trims the ends. That is enough to make "Piraeus", "PIRAEUS" and
// "Piraeus (Port)" the same key.
//
// KNOWN LIMIT, WRITTEN DOWN BECAUSE IT BOUNDS WHAT AN ALIAS SET HAS TO CONTAIN:
// this does NOT fold diacritics. The standard library carries no Unicode
// normalization form, so decomposing an accented character and dropping its
// combining mark is not available without a dependency, and this module takes
// none. The consequence is concrete and a caller must plan for it — an accented
// spelling and its unaccented spelling are DIFFERENT KEYS, so both belong in
// the alias set explicitly. That is more typing and it is honest; the
// alternative is a half-working folding table that silently misses the letters
// nobody thought of.
func NormalizeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pendingSeparator := false
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			pendingSeparator = true
			continue
		}
		if pendingSeparator && b.Len() > 0 {
			b.WriteRune(nameSeparator)
		}
		pendingSeparator = false
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// AliasIndex resolves any known name for a thing onto its canonical ID.
//
// It refuses a collision rather than resolving it. Two ports that share an
// alias is not a preference to be settled by insertion order — it is a defect
// in whoever built the vocabulary, and the only useful thing an index can do is
// say so at the point the second one is added, while the caller still knows
// which two they were.
type AliasIndex struct {
	byName map[string]ID
}

// NewAliasIndex returns an empty index.
func NewAliasIndex() *AliasIndex {
	return &AliasIndex{byName: make(map[string]ID)}
}

// Add registers every name as an alias of id.
//
// Re-adding a name for the SAME id is a no-op rather than an error, because a
// vocabulary is usually assembled from several passes over the same rows and a
// duplicate is not a mistake. Registering it for a DIFFERENT id is
// ErrAliasCollision.
func (x *AliasIndex) Add(id ID, names ...string) error {
	if !id.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidID, string(id))
	}
	for _, name := range names {
		key := NormalizeName(name)
		if key == "" {
			return fmt.Errorf("%w: %q", ErrEmptyAlias, name)
		}
		if prev, ok := x.byName[key]; ok && prev != id {
			return fmt.Errorf("%w: %q is claimed by %q and %q", ErrAliasCollision, key, string(prev), string(id))
		}
		x.byName[key] = id
	}
	return nil
}

// Lookup resolves a display name. The name is normalized first, so the caller
// does not have to.
func (x *AliasIndex) Lookup(name string) (ID, bool) {
	id, ok := x.byName[NormalizeName(name)]
	return id, ok
}

// Len is the number of distinct alias keys registered, NOT the number of
// canonical IDs — one ID usually carries several.
func (x *AliasIndex) Len() int { return len(x.byName) }

// VesselClass is the closed set of hull types a feed may declare.
//
// It is closed rather than free text because it carries a SPEED CHARACTERISTIC
// that the feed validator relies on, and a class nobody has assigned a speed
// band to would silently disable that check for every vessel that used it.
type VesselClass string

// The classes. A feed that needs one not listed here adds it in the same patch
// that adds its speed band, which is the point of keeping the set closed.
const (
	ClassConventionalFerry  VesselClass = "conventional-ferry"
	ClassRoPax              VesselClass = "ro-pax"
	ClassHighSpeedCatamaran VesselClass = "high-speed-catamaran"
	ClassHydrofoil          VesselClass = "hydrofoil"
	ClassLandingCraft       VesselClass = "landing-craft"
)

// SpeedEnvelopeKnots returns the plausible service-speed band for the class and
// whether the class is known at all. See the comment on speedEnvelopeKnots for
// what these numbers are and — more importantly — what they are not.
func (c VesselClass) SpeedEnvelopeKnots() (lo, hi float64, ok bool) {
	e, ok := speedEnvelopeKnots[c]
	return e.lo, e.hi, ok
}

// Valid reports whether the class is one of the declared set.
func (c VesselClass) Valid() bool {
	_, _, ok := c.SpeedEnvelopeKnots()
	return ok
}

// Classes returns the declared classes in a stable order. The slice is a copy,
// so a caller cannot reorder the package's own list.
func Classes() []VesselClass {
	out := make([]VesselClass, len(classOrder))
	copy(out, classOrder)
	return out
}

// ExceptionType is a calendar override: a service running on a date its regular
// pattern excludes, or not running on a date its pattern includes.
//
// The two values match the GTFS encoding on the wire (1 added, 2 removed) so a
// feed reader has nothing to translate. Zero is deliberately not a valid value,
// so a struct field left unset is refused rather than defaulting to "added",
// which is the direction that invents sailings.
type ExceptionType int

const (
	// ServiceAdded means the service runs on this date even though its regular
	// pattern does not include it.
	ServiceAdded ExceptionType = 1
	// ServiceRemoved means the service does not run on this date even though
	// its regular pattern includes it.
	ServiceRemoved ExceptionType = 2
)

// Valid reports whether the exception type is one of the two declared values.
func (e ExceptionType) Valid() bool {
	return e == ServiceAdded || e == ServiceRemoved
}

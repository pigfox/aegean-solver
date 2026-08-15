// Package ferry is the GTFS-shaped schedule schema this module routes over.
//
// # It ships zero rows, on purpose
//
// There is not one port, one operator, one vessel and not one sailing in this
// package. Every type here is a SHAPE and a set of rules about what a valid row
// looks like; populating them is somebody else's job, done from a real source
// through the source boundary. The phase that produced this package was
// explicitly forbidden from authoring, transcribing or synthesizing schedule
// data, and the schema is what that prohibition leaves behind: a place for real
// data to go, and a validator that will refuse it if it arrives malformed.
//
// # Where it follows GTFS, and the two places it does not
//
// The vocabulary is GTFS: ports are stops, operators are agencies, routes and
// trips and stop times and calendars keep their names and their meanings, and
// the calendar exception codes match the wire values. Two things are
// deliberately different, and both are recorded at the type that changes.
//
// Trip.StopTimes is a slice on the trip rather than a separate table keyed by
// trip and sequence number. GTFS has to keep them apart because it is a set of
// CSV files; nothing here is. Folding them in makes an out-of-order sequence
// and a duplicated sequence number unrepresentable rather than merely invalid.
//
// A service may carry MORE THAN ONE Calendar row, where GTFS allows exactly
// one. See Calendar for why: a season that runs in July and again in December
// needs two windows, and one row per service forces that to be expressed as a
// pile of date exceptions instead.
package ferry

import (
	"errors"
	"fmt"

	"github.com/pigfox/aegean-solver/internal/config"
	"github.com/pigfox/aegean-solver/internal/vocab"
)

// The errors a caller can act on. Every Validate wraps one of these with %w and
// adds the offending value, so a caller matches on the sentinel and a human
// reads the detail.
var (
	// ErrID is an identifier outside the canonical grammar.
	ErrID = errors.New("ferry: invalid identifier")
	// ErrName is a required display name that is blank or is punctuation only.
	ErrName = errors.New("ferry: blank name")
	// ErrCoordinate is a latitude or longitude off the globe.
	ErrCoordinate = errors.New("ferry: coordinate out of range")
	// ErrTransferTime is a negative or implausibly long connection time.
	ErrTransferTime = errors.New("ferry: transfer time out of range")
	// ErrVesselClass is a class outside the declared vocabulary.
	ErrVesselClass = errors.New("ferry: unknown vessel class")
	// ErrVesselSpeed is a service speed outside its class's envelope.
	ErrVesselSpeed = errors.New("ferry: service speed outside the class envelope")
	// ErrStopTimeRange is a stop time that is negative, out of order within its
	// own stop, or past the ceiling.
	ErrStopTimeRange = errors.New("ferry: stop time out of range")
	// ErrStopSequence is a trip whose stops do not advance in time.
	ErrStopSequence = errors.New("ferry: stop times do not advance")
	// ErrTooFewStops is a trip with fewer than MinStopTimesPerTrip stops.
	ErrTooFewStops = errors.New("ferry: trip has too few stops")
	// ErrWeekdays is a calendar naming no day, or carrying a bit that is not a
	// day.
	ErrWeekdays = errors.New("ferry: invalid weekday set")
	// ErrDate is a date that is not a real calendar date.
	ErrDate = errors.New("ferry: invalid date")
	// ErrDateWindow is a validity window that ends before it starts.
	ErrDateWindow = errors.New("ferry: validity window ends before it starts")
	// ErrExceptionType is a calendar override that is neither added nor
	// removed.
	ErrExceptionType = errors.New("ferry: invalid calendar exception type")
	// ErrDuplicateID is the same identifier declared twice in one feed.
	ErrDuplicateID = errors.New("ferry: duplicate identifier")
	// ErrDanglingRef is a reference to an identifier the feed does not declare.
	ErrDanglingRef = errors.New("ferry: reference to an undeclared identifier")
	// ErrConflictingException is one service given two overrides on one date.
	ErrConflictingException = errors.New("ferry: conflicting calendar exceptions on one date")
	// ErrImpliedSpeed is a leg the assigned vessel could not physically sail in
	// the time the schedule allows.
	ErrImpliedSpeed = errors.New("ferry: implied speed exceeds the vessel envelope")
	// ErrRouteName is a route with neither a short nor a long name.
	ErrRouteName = errors.New("ferry: route has neither a short nor a long name")
)

// blank reports whether a display name carries nothing a reader or an alias
// index could use. It is defined in terms of the alias normalizer rather than
// as a space trim, so that a name which cannot produce a lookup key is refused
// at the same point a name which is empty is.
func blank(s string) bool { return vocab.NormalizeName(s) == "" }

// Port is a place a vessel calls at.
//
// # Two names, and neither substitutes for the other
//
// PortName and IslandName are BOTH REQUIRED and are separate fields, because in
// this basin they routinely differ and the divergence is the normal case rather
// than an exception. The harbor on Milos is Adamas; the harbor on Santorini is
// Athinios; Piraeus is a port with no island at all. A traveler asks for the
// island, a timetable prints the harbor, and a schema that carries one field
// forces whoever populates it to choose which of the two to lose. Callers that
// want either name to resolve a query put both into Aliases and build a
// vocab.AliasIndex over them.
//
// The two being EQUAL is legal and expected — a mainland port sets IslandName
// to the region it serves. The rule is not that they differ; it is that the
// schema never makes one stand in for the other.
type Port struct {
	// ID is the canonical identifier. Nothing here interprets it.
	ID vocab.ID
	// PortName is the harbor as a timetable prints it.
	PortName string
	// IslandName is the island, or for a mainland port the region, that the
	// harbor serves.
	IslandName string
	// Aliases are every other name this place is known by, in any language or
	// spelling. Diacritics are NOT folded by the normalizer, so an accented and
	// an unaccented spelling are two aliases — see vocab.NormalizeName.
	Aliases []string
	// Lat and Lon are degrees, WGS84.
	Lat, Lon float64
	// MinTransferSeconds is how long changing vessels takes here. Zero means
	// config.DefaultMinTransferSeconds; use MinTransfer rather than reading
	// this field directly.
	MinTransferSeconds int
}

// Validate checks one port in isolation.
func (p Port) Validate() error {
	if !p.ID.Valid() {
		return fmt.Errorf("%w: port %q", ErrID, string(p.ID))
	}
	if blank(p.PortName) {
		return fmt.Errorf("%w: port %q has no port name", ErrName, string(p.ID))
	}
	if blank(p.IslandName) {
		return fmt.Errorf("%w: port %q has no island name", ErrName, string(p.ID))
	}
	if p.Lat < MinLatitude || p.Lat > MaxLatitude {
		return fmt.Errorf("%w: port %q latitude %v", ErrCoordinate, string(p.ID), p.Lat)
	}
	if p.Lon < MinLongitude || p.Lon > MaxLongitude {
		return fmt.Errorf("%w: port %q longitude %v", ErrCoordinate, string(p.ID), p.Lon)
	}
	if p.MinTransferSeconds < 0 || p.MinTransferSeconds > MaxMinTransferSeconds {
		return fmt.Errorf("%w: port %q transfer %ds", ErrTransferTime, string(p.ID), p.MinTransferSeconds)
	}
	return nil
}

// MinTransfer is the connection time to use at this port, with the default
// applied. Every consumer goes through it so that "zero means default" is
// decided in one place instead of at each call site.
func (p Port) MinTransfer() int {
	if p.MinTransferSeconds == 0 {
		return config.DefaultMinTransferSeconds
	}
	return p.MinTransferSeconds
}

// Operator is the company that runs a route — GTFS calls it an agency.
type Operator struct {
	ID      vocab.ID
	Name    string
	Aliases []string
}

// Validate checks one operator in isolation.
func (o Operator) Validate() error {
	if !o.ID.Valid() {
		return fmt.Errorf("%w: operator %q", ErrID, string(o.ID))
	}
	if blank(o.Name) {
		return fmt.Errorf("%w: operator %q has no name", ErrName, string(o.ID))
	}
	return nil
}

// Vessel is the ship assigned to a trip.
//
// GTFS HAS NO SUCH TABLE, and this one is added rather than borrowed. The
// reason is that Class carries a speed characteristic, and a speed
// characteristic is the only thing in this schema that can catch a schedule
// which is internally consistent and physically impossible — an arrival time
// that would have the vessel crossing at eighty knots. Every other rule here
// checks a row against the grammar; this one checks it against the sea.
type Vessel struct {
	ID         vocab.ID
	Name       string
	OperatorID vocab.ID
	// Class is REQUIRED and is what carries the speed envelope.
	Class vocab.VesselClass
	// ServiceSpeedKnots is this hull's own cruising speed, which must sit
	// inside its class's envelope.
	ServiceSpeedKnots float64
}

// Validate checks one vessel in isolation.
func (v Vessel) Validate() error {
	if !v.ID.Valid() {
		return fmt.Errorf("%w: vessel %q", ErrID, string(v.ID))
	}
	if blank(v.Name) {
		return fmt.Errorf("%w: vessel %q has no name", ErrName, string(v.ID))
	}
	if !v.OperatorID.Valid() {
		return fmt.Errorf("%w: vessel %q operator %q", ErrID, string(v.ID), string(v.OperatorID))
	}
	lo, hi, ok := v.Class.SpeedEnvelopeKnots()
	if !ok {
		return fmt.Errorf("%w: vessel %q class %q", ErrVesselClass, string(v.ID), string(v.Class))
	}
	if v.ServiceSpeedKnots < lo || v.ServiceSpeedKnots > hi {
		return fmt.Errorf("%w: vessel %q at %v kn, class %q allows %v..%v", ErrVesselSpeed,
			string(v.ID), v.ServiceSpeedKnots, string(v.Class), lo, hi)
	}
	return nil
}

// Route is a named service pattern an operator sells.
type Route struct {
	ID         vocab.ID
	OperatorID vocab.ID
	ShortName  string
	LongName   string
}

// Validate checks one route in isolation. At least one of the two names must be
// present, which is the GTFS rule and a good one: a route with neither cannot
// be shown to anybody.
func (r Route) Validate() error {
	if !r.ID.Valid() {
		return fmt.Errorf("%w: route %q", ErrID, string(r.ID))
	}
	if !r.OperatorID.Valid() {
		return fmt.Errorf("%w: route %q operator %q", ErrID, string(r.ID), string(r.OperatorID))
	}
	if blank(r.ShortName) && blank(r.LongName) {
		return fmt.Errorf("%w: route %q", ErrRouteName, string(r.ID))
	}
	return nil
}

// StopTime is one call at one port on one trip.
//
// Both times are SECONDS AFTER THE START OF THE SERVICE DAY, not times of day,
// and they may exceed 86400 — that is how an overnight sailing is written. See
// MaxStopTimeSeconds.
type StopTime struct {
	PortID           vocab.ID
	ArrivalSeconds   int
	DepartureSeconds int
}

// Validate checks one stop time in isolation. Arrival may equal departure — a
// vessel that does not linger is normal, and the first stop of a trip almost
// always has them equal.
func (s StopTime) Validate() error {
	if !s.PortID.Valid() {
		return fmt.Errorf("%w: stop time port %q", ErrID, string(s.PortID))
	}
	if s.ArrivalSeconds < 0 || s.ArrivalSeconds > MaxStopTimeSeconds {
		return fmt.Errorf("%w: arrival %ds at %q", ErrStopTimeRange, s.ArrivalSeconds, string(s.PortID))
	}
	if s.DepartureSeconds < s.ArrivalSeconds || s.DepartureSeconds > MaxStopTimeSeconds {
		return fmt.Errorf("%w: departure %ds at %q", ErrStopTimeRange, s.DepartureSeconds, string(s.PortID))
	}
	return nil
}

// Trip is one run of one route by one vessel on the days one service names.
//
// StopTimes is ordered and the order IS the sequence — there is no sequence
// number, because a slice already has one and two representations of the same
// ordering can disagree.
//
// A trip MAY call at the same port more than once. A rotation that returns to
// where it started is ordinary here, and forbidding it would reject real
// schedules to make one proof in the verifier easier. That proof does not
// need it: see the bound in the tdsp package, which counts arrival events
// rather than ports.
type Trip struct {
	ID        vocab.ID
	RouteID   vocab.ID
	VesselID  vocab.ID
	ServiceID vocab.ID
	Headsign  string
	StopTimes []StopTime
}

// Validate checks one trip in isolation — its own identifiers, its stop times,
// and that those stop times advance. It cannot check that the identifiers it
// references exist; that is Feed.Validate's job.
func (t Trip) Validate() error {
	if !t.ID.Valid() {
		return fmt.Errorf("%w: trip %q", ErrID, string(t.ID))
	}
	for _, ref := range []vocab.ID{t.RouteID, t.VesselID, t.ServiceID} {
		if !ref.Valid() {
			return fmt.Errorf("%w: trip %q references %q", ErrID, string(t.ID), string(ref))
		}
	}
	if len(t.StopTimes) < MinStopTimesPerTrip {
		return fmt.Errorf("%w: trip %q has %d", ErrTooFewStops, string(t.ID), len(t.StopTimes))
	}
	for i, st := range t.StopTimes {
		if err := st.Validate(); err != nil {
			return fmt.Errorf("trip %q stop %d: %w", string(t.ID), i, err)
		}
		if i == 0 {
			continue
		}
		// STRICTLY greater, not greater-or-equal. A leg that takes no time at
		// all is not a sailing, and admitting one would let the solver report a
		// journey that teleports — which is the class of wrong answer that
		// looks best on the page.
		if st.ArrivalSeconds <= t.StopTimes[i-1].DepartureSeconds {
			return fmt.Errorf("%w: trip %q stop %d arrives at %ds, stop %d departed at %ds",
				ErrStopSequence, string(t.ID), i, st.ArrivalSeconds, i-1, t.StopTimes[i-1].DepartureSeconds)
		}
	}
	return nil
}

// Calendar is one validity window for one service.
//
// # More than one row per service is legal here, and GTFS says otherwise
//
// GTFS keys calendar.txt on service_id, so a service gets exactly one window
// and one weekday pattern. That makes the ordinary Aegean case awkward: a route
// that runs Tuesdays through July and again through December has two windows
// and one pattern, and with a single row the second window can only be written
// as a heap of individual date exceptions — dozens of rows that say nothing
// about the pattern they came from, and that nobody can read back as a season.
//
// So a service here may carry several Calendar rows and its active days are
// their UNION. The directive that produced this package asked for a calendar in
// which "runs Tuesdays in July and not in October" is representable without
// branching, and this is what makes it a matter of declaring one row per
// window. Exceptions remain available in CalendarDate for the genuinely
// irregular day.
type Calendar struct {
	ServiceID vocab.ID
	// Days is the weekday pattern inside the window.
	Days Weekdays
	// Start and End are INCLUSIVE.
	Start Date
	End   Date
}

// Validate checks one calendar window in isolation.
func (c Calendar) Validate() error {
	if !c.ServiceID.Valid() {
		return fmt.Errorf("%w: calendar service %q", ErrID, string(c.ServiceID))
	}
	if !c.Days.Valid() {
		return fmt.Errorf("%w: calendar service %q days %08b", ErrWeekdays, string(c.ServiceID), uint8(c.Days))
	}
	if !c.Start.Valid() {
		return fmt.Errorf("%w: calendar service %q start %s", ErrDate, string(c.ServiceID), c.Start)
	}
	if !c.End.Valid() {
		return fmt.Errorf("%w: calendar service %q end %s", ErrDate, string(c.ServiceID), c.End)
	}
	if c.Start.Compare(c.End) > 0 {
		return fmt.Errorf("%w: calendar service %q %s..%s", ErrDateWindow, string(c.ServiceID), c.Start, c.End)
	}
	return nil
}

// Covers reports whether this window includes the date and names its weekday.
func (c Calendar) Covers(d Date) bool {
	if c.Start.Compare(d) > 0 || c.End.Compare(d) < 0 {
		return false
	}
	return c.Days.Has(d.Weekday())
}

// CalendarDate overrides a service's regular pattern on one date.
type CalendarDate struct {
	ServiceID vocab.ID
	Date      Date
	Exception vocab.ExceptionType
}

// Validate checks one calendar override in isolation.
func (cd CalendarDate) Validate() error {
	if !cd.ServiceID.Valid() {
		return fmt.Errorf("%w: calendar date service %q", ErrID, string(cd.ServiceID))
	}
	if !cd.Date.Valid() {
		return fmt.Errorf("%w: calendar date service %q date %s", ErrDate, string(cd.ServiceID), cd.Date)
	}
	if !cd.Exception.Valid() {
		return fmt.Errorf("%w: calendar date service %q exception %d", ErrExceptionType, string(cd.ServiceID), int(cd.Exception))
	}
	return nil
}

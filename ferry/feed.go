package ferry

import (
	"fmt"
	"time"

	aegean "github.com/pigfox/aegean-solver"
	"github.com/pigfox/aegean-solver/internal/config"
	"github.com/pigfox/aegean-solver/vocab"
)

// Feed is a whole schedule: every table, in one value.
//
// THE ZERO FEED IS VALID, and that is a requirement rather than an accident.
// The source boundary ships a null implementation whose whole job is to hand
// back an empty feed that passes Validate, so that every consumer downstream
// can be exercised end to end before a single real row exists. A validator that
// rejected the empty case would make that impossible and would push everyone
// into populating something just to get the pipeline to run — which is exactly
// how invented data gets into a repository that swore it would not have any.
type Feed struct {
	// Location is the zone the service days and stop times are expressed in.
	// Nil means UTC; read it through Loc rather than directly.
	Location *time.Location

	Ports         []Port
	Operators     []Operator
	Vessels       []Vessel
	Routes        []Route
	Trips         []Trip
	Calendars     []Calendar
	CalendarDates []CalendarDate
}

// Loc is the feed's zone with the nil default applied.
func (f *Feed) Loc() *time.Location {
	if f.Location == nil {
		return time.UTC
	}
	return f.Location
}

// PortByID looks a port up.
func (f *Feed) PortByID(id vocab.ID) (Port, bool) {
	for _, p := range f.Ports {
		if p.ID == id {
			return p, true
		}
	}
	return Port{}, false
}

// ServiceActiveOn reports whether the named service runs on the given date.
//
// AN OVERRIDE BEATS THE PATTERN, which is the GTFS rule and the only one that
// makes a season editable: the pattern says what usually happens and the
// exception says what happened instead. Feed.Validate guarantees at most one
// override per service and date, so the first match found is the only match and
// returning from inside the loop is deterministic rather than
// order-of-iteration luck.
//
// With no override, the service runs if ANY of its calendar windows covers the
// date and names its weekday — the union described on Calendar.
func (f *Feed) ServiceActiveOn(serviceID vocab.ID, d Date) bool {
	for _, cd := range f.CalendarDates {
		if cd.ServiceID == serviceID && cd.Date == d {
			return cd.Exception == vocab.ServiceAdded
		}
	}
	for _, c := range f.Calendars {
		if c.ServiceID == serviceID && c.Covers(d) {
			return true
		}
	}
	return false
}

// Validate checks the whole feed: every row against its own rules, every
// cross-table reference, and every leg against the physics of the vessel
// assigned to sail it.
//
// The order matters and is not alphabetical. Each stage builds the index the
// next one needs, so a dangling reference is reported against a table that has
// already been proven internally sound — which keeps the error message about
// the reference instead of about a row that was malformed anyway.
func (f *Feed) Validate() error {
	ports, err := f.validatePorts()
	if err != nil {
		return err
	}
	operators, err := f.validateOperators()
	if err != nil {
		return err
	}
	vessels, err := f.validateVessels(operators)
	if err != nil {
		return err
	}
	routes, err := f.validateRoutes(operators)
	if err != nil {
		return err
	}
	services, err := f.validateCalendars()
	if err != nil {
		return err
	}
	if err := f.validateTrips(ports, vessels, routes, services); err != nil {
		return err
	}
	return f.validateSpeeds(ports, vessels)
}

// declare records an identifier and refuses a second declaration of it.
func declare(seen map[vocab.ID]struct{}, id vocab.ID, kind string) error {
	if _, dup := seen[id]; dup {
		return fmt.Errorf("%w: %s %q", ErrDuplicateID, kind, string(id))
	}
	seen[id] = struct{}{}
	return nil
}

// require refuses a reference to an identifier nothing declared.
func require(seen map[vocab.ID]struct{}, id vocab.ID, kind, from string) error {
	if _, ok := seen[id]; !ok {
		return fmt.Errorf("%w: %s %q referenced by %s", ErrDanglingRef, kind, string(id), from)
	}
	return nil
}

func (f *Feed) validatePorts() (map[vocab.ID]Port, error) {
	ports := make(map[vocab.ID]Port, len(f.Ports))
	seen := make(map[vocab.ID]struct{}, len(f.Ports))
	for _, p := range f.Ports {
		if err := p.Validate(); err != nil {
			return nil, err
		}
		if err := declare(seen, p.ID, "port"); err != nil {
			return nil, err
		}
		ports[p.ID] = p
	}
	return ports, nil
}

func (f *Feed) validateOperators() (map[vocab.ID]struct{}, error) {
	seen := make(map[vocab.ID]struct{}, len(f.Operators))
	for _, o := range f.Operators {
		if err := o.Validate(); err != nil {
			return nil, err
		}
		if err := declare(seen, o.ID, "operator"); err != nil {
			return nil, err
		}
	}
	return seen, nil
}

func (f *Feed) validateVessels(operators map[vocab.ID]struct{}) (map[vocab.ID]Vessel, error) {
	vessels := make(map[vocab.ID]Vessel, len(f.Vessels))
	seen := make(map[vocab.ID]struct{}, len(f.Vessels))
	for _, v := range f.Vessels {
		if err := v.Validate(); err != nil {
			return nil, err
		}
		if err := declare(seen, v.ID, "vessel"); err != nil {
			return nil, err
		}
		if err := require(operators, v.OperatorID, "operator", "vessel "+string(v.ID)); err != nil {
			return nil, err
		}
		vessels[v.ID] = v
	}
	return vessels, nil
}

func (f *Feed) validateRoutes(operators map[vocab.ID]struct{}) (map[vocab.ID]struct{}, error) {
	seen := make(map[vocab.ID]struct{}, len(f.Routes))
	for _, r := range f.Routes {
		if err := r.Validate(); err != nil {
			return nil, err
		}
		if err := declare(seen, r.ID, "route"); err != nil {
			return nil, err
		}
		if err := require(operators, r.OperatorID, "operator", "route "+string(r.ID)); err != nil {
			return nil, err
		}
	}
	return seen, nil
}

// serviceDate keys the one-override-per-service-per-date rule.
type serviceDate struct {
	service vocab.ID
	date    Date
}

// validateCalendars checks both calendar tables and returns the set of services
// either of them declares.
//
// A service is DECLARED BY EITHER TABLE. One that exists only as a set of
// overrides — a relief sailing on three named dates and no pattern at all — is
// a real thing, and requiring a window for it would force whoever populates the
// feed to invent a pattern that does not exist.
func (f *Feed) validateCalendars() (map[vocab.ID]struct{}, error) {
	services := make(map[vocab.ID]struct{}, len(f.Calendars))
	for _, c := range f.Calendars {
		if err := c.Validate(); err != nil {
			return nil, err
		}
		// NO duplicate check on ServiceID: several windows per service is the
		// deliberate divergence from GTFS documented on Calendar.
		services[c.ServiceID] = struct{}{}
	}
	overrides := make(map[serviceDate]struct{}, len(f.CalendarDates))
	for _, cd := range f.CalendarDates {
		if err := cd.Validate(); err != nil {
			return nil, err
		}
		key := serviceDate{service: cd.ServiceID, date: cd.Date}
		if _, dup := overrides[key]; dup {
			return nil, fmt.Errorf("%w: service %q on %s", ErrConflictingException, string(cd.ServiceID), cd.Date)
		}
		overrides[key] = struct{}{}
		services[cd.ServiceID] = struct{}{}
	}
	return services, nil
}

func (f *Feed) validateTrips(ports map[vocab.ID]Port, vessels map[vocab.ID]Vessel, routes, services map[vocab.ID]struct{}) error {
	seen := make(map[vocab.ID]struct{}, len(f.Trips))
	for _, t := range f.Trips {
		if err := t.Validate(); err != nil {
			return err
		}
		if err := declare(seen, t.ID, "trip"); err != nil {
			return err
		}
		from := "trip " + string(t.ID)
		if err := require(routes, t.RouteID, "route", from); err != nil {
			return err
		}
		if _, ok := vessels[t.VesselID]; !ok {
			return fmt.Errorf("%w: vessel %q referenced by %s", ErrDanglingRef, string(t.VesselID), from)
		}
		if err := require(services, t.ServiceID, "service", from); err != nil {
			return err
		}
		for i, st := range t.StopTimes {
			if _, ok := ports[st.PortID]; !ok {
				return fmt.Errorf("%w: port %q referenced by %s stop %d", ErrDanglingRef, string(st.PortID), from, i)
			}
		}
	}
	return nil
}

// validateSpeeds is the one check in this package that is about the sea rather
// than about the grammar.
//
// It runs LAST and only after validateTrips has proven every reference, so the
// lookups below cannot miss and the elapsed time cannot be zero — Trip.Validate
// already refused a leg whose arrival does not strictly follow the previous
// departure. Neither fact is re-checked here, because a branch that cannot be
// taken is a branch nothing can test.
func (f *Feed) validateSpeeds(ports map[vocab.ID]Port, vessels map[vocab.ID]Vessel) error {
	for _, t := range f.Trips {
		vessel := vessels[t.VesselID]
		ceiling := vessel.ServiceSpeedKnots * config.SpeedToleranceFactor
		for i := 1; i < len(t.StopTimes); i++ {
			prev, cur := t.StopTimes[i-1], t.StopTimes[i]
			elapsed := cur.ArrivalSeconds - prev.DepartureSeconds
			implied := impliedSpeedKnots(ports[prev.PortID], ports[cur.PortID], elapsed)
			if implied > ceiling {
				return fmt.Errorf("%w: trip %q leg %d implies %.1f kn, vessel %q allows %.1f kn",
					ErrImpliedSpeed, string(t.ID), i, implied, string(vessel.ID), ceiling)
			}
		}
	}
	return nil
}

// impliedSpeedKnots is the average speed a vessel would need to cover the
// straight line between two ports in the time the schedule allows.
//
// The distance is a GREAT CIRCLE, so it is always at or below the distance
// actually sailed. That asymmetry is deliberate and is what makes the check
// safe to apply: it can only ever understate the required speed, so it cannot
// invent a violation, and a schedule it does flag is one the sea itself would
// refuse.
func impliedSpeedKnots(from, to Port, elapsedSeconds int) float64 {
	km := aegean.DistanceKm(
		aegean.Site{Lat: from.Lat, Lon: from.Lon},
		aegean.Site{Lat: to.Lat, Lon: to.Lon},
	)
	hours := float64(elapsedSeconds) / float64(config.SecondsPerHour)
	return km / hours / config.KilometersPerNauticalMile
}

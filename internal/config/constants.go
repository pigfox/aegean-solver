// Package config holds the tunables that more than one package in this module
// reads.
//
// It exists so that a figure two packages must agree on has exactly one place
// to be argued about. A constant used by a single package does NOT belong here
// — it belongs in that package's own constants.go, where its blast radius is
// visible from the file it sits in. The split is deliberate: a config package
// that accumulates every literal in a module stops being a place to look and
// becomes a place to grep.
//
// Nothing in here is schedule data. These are units, ceilings and defaults.
package config

// The unit ladder. Written out rather than inlined so that a duration in this
// module reads as a quantity with a unit attached, and so that a reviewer can
// see 20*SecondsPerMinute and check it against a timetable without doing
// arithmetic in their head.
//
// These are SECONDS, not time.Duration, because the schedule side of this
// module speaks GTFS, and GTFS stop times are integer seconds after the start
// of a service day — a value that routinely exceeds 24 hours and therefore
// cannot be a wall-clock time of day. Converting to an absolute instant
// happens once, at the boundary where a service day is pinned to a calendar
// date in a named location.
const (
	SecondsPerMinute = 60
	SecondsPerHour   = 60 * SecondsPerMinute
	SecondsPerDay    = 24 * SecondsPerHour
)

const (
	// DefaultMinTransferSeconds is the connection time assumed at a port that
	// declares none of its own.
	//
	// TWENTY MINUTES IS A DEFAULT, NOT A MEASUREMENT. No port in this module
	// ships a real figure and this number is not derived from one. It is here
	// so that a feed which omits the field produces itineraries a planner would
	// not be embarrassed by, rather than producing zero — and zero is the
	// dangerous default, because it silently permits a connection that steps
	// off one vessel and onto another in the same instant, which then reads as
	// a real routing result. A caller that knows the true figure sets
	// Port.MinTransferSeconds and this is never consulted.
	DefaultMinTransferSeconds = 20 * SecondsPerMinute

	// DefaultSearchHorizonDays is how far past the requested departure an
	// earliest-arrival query looks before reporting that no connection exists.
	//
	// A WEEK, because the thing this module was written for is a route served
	// twice a week. A horizon of one day answers "no connection exists" for
	// every island pair not on a daily boat, which is the wrong answer stated
	// confidently. Seven days is the shortest horizon on which a weekly pattern
	// is guaranteed to repeat at least once.
	DefaultSearchHorizonDays = 7

	// MaxSearchHorizonDays refuses a horizon large enough to turn one query
	// into a denial of service. The expanded graph grows linearly in the
	// horizon; the exhaustive verifier that checks it grows very much worse
	// than linearly in the number of trip instances the horizon admits.
	MaxSearchHorizonDays = 60
)

const (
	// KilometersPerNauticalMile converts knots to km/h. It is the exact
	// definition of the nautical mile, not an approximation.
	KilometersPerNauticalMile = 1.852

	// SpeedToleranceFactor is how far above a vessel's declared service speed
	// an implied point-to-point speed may sit before a feed is rejected.
	//
	// IT IS DELIBERATELY LOOSE, and the looseness is the point. The implied
	// speed of a leg is computed from a GREAT-CIRCLE distance, and no vessel
	// sails a great circle — it sails around islands, out of a channel and into
	// a harbor approach. So the implied figure always UNDERSTATES the distance
	// actually covered, which means a plausible schedule can never be rejected
	// here for being too slow, and can only be rejected for being too fast. A
	// leg that clears 25% above the vessel's own declared service speed on the
	// straight-line distance alone has something wrong with it: a transposed
	// digit, a stop sequence out of order, or a time recorded against the wrong
	// day. This is a smoke alarm, not a physics model.
	SpeedToleranceFactor = 1.25
)

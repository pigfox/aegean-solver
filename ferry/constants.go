package ferry

import "github.com/pigfox/aegean-solver/internal/config"

// The bounds a calendar date must sit inside.
//
// They are sanity rails, not history. A schedule dated before the Unix epoch or
// four figures into the future is a parse defect — a two-digit year read as an
// absolute one, a millisecond timestamp read as seconds — and catching it at
// the boundary is cheaper than watching the solver silently find no connection
// because every service window is a thousand years away.
const (
	MinYear = 1970
	MaxYear = 2999
)

// ServiceDayAnchorHour is the hour a service day is measured from, indirectly.
//
// GTFS defines a stop time as seconds after "noon minus twelve hours" on the
// service date rather than after midnight, and the wording is not pedantry. On
// the two days a year a clock shifts, local midnight and noon-minus-twelve are
// different instants, and a schedule anchored to midnight moves every sailing
// on those days by an hour. Noon is chosen because no jurisdiction shifts its
// clock at noon, so the anchor is always an unambiguous local instant.
const ServiceDayAnchorHour = 12

// MinStopTimesPerTrip is two, because a trip with one stop is not a journey and
// a trip with none is a row with nothing in it. Refusing them here means the
// graph builder can index i+1 without a guard.
const MinStopTimesPerTrip = 2

// MaxStopTimeSeconds is the ceiling on a stop time measured from the start of
// its service day.
//
// GTFS stop times routinely exceed 24 hours — that is how an overnight sailing
// that departs at 23:30 and berths at 06:15 is written, as 06:15 on the NEXT
// clock day but still against the service day it departed on. So a ceiling is
// needed and 86400 is the wrong one. Three days is chosen as a generous bound
// on any single sailing in a basin this size: past that, the value is far more
// likely to be a date accidentally added to a time than a very long crossing.
const MaxStopTimeSeconds = 3 * config.SecondsPerDay

// MaxMinTransferSeconds refuses a connection time longer than a day. A port
// where changing vessels takes more than 24 hours is not a transfer, it is an
// overnight stay, and modeling it as a transfer would make the solver report
// journeys nobody would call a connection.
const MaxMinTransferSeconds = config.SecondsPerDay

// The coordinate bounds. WGS84 degrees.
const (
	MinLatitude  = -90.0
	MaxLatitude  = 90.0
	MinLongitude = -180.0
	MaxLongitude = 180.0
)

// allWeekdaysMask is the seven low bits, one per time.Weekday. A Weekdays value
// carrying anything above them was built by casting rather than by With, and is
// refused — a stray high bit is a day that can never match, which reads on the
// page as a service that simply never runs.
const allWeekdaysMask Weekdays = 1<<7 - 1

package vocab

// The identifier grammar. An ID reaches a URL path, a JSON object key and a
// generated file name before it is ever printed for a human, so the character
// set is narrow on purpose: lowercase ASCII letters, digits and one separator.
// Anything wider means an ID that round-trips through one of those surfaces
// and comes back different.
const (
	// MaxIDLength is a length ceiling rather than a measured limit. It exists
	// so that a caller who accidentally passes a whole description where an ID
	// belongs is refused at the boundary instead of storing it.
	MaxIDLength = 64

	// IDSeparator is the only punctuation an ID may carry. A hyphen and not an
	// underscore, because an ID is expected to appear in a URL path.
	IDSeparator = '-'
)

// nameSeparator is what NormalizeName collapses every run of non-alphanumeric
// input to. It is a plain space, which cannot appear in an ID, so a normalized
// display name can never be mistaken for a canonical ID.
const nameSeparator = ' '

// speedEnvelope is the plausible service-speed band for a class of vessel, in
// knots.
type speedEnvelope struct{ lo, hi float64 }

// speedEnvelopeKnots is the speed characteristic the schema requires every
// vessel class to carry.
//
// THESE ARE ENVELOPES, NOT SCHEDULES, and the distinction is the whole reason
// they are allowed to exist in a module that ships zero schedule data. A band
// like "a conventional ferry does somewhere between 8 and 24 knots" is a fact
// about a category of hull, not a fact about any operator, route or sailing.
// Nothing here is ever used to COMPUTE a time: the only consumer is the feed
// validator, which uses the band to reject a leg whose implied speed is
// physically wrong for the vessel that is supposed to be sailing it. If a
// future change starts deriving a departure or an arrival from one of these
// numbers, that change has crossed into inventing a timetable and the band
// should be removed rather than reused.
//
// The bands are deliberately wide. A narrow band would start rejecting real
// schedules, and a validator that cries wolf gets switched off.
var speedEnvelopeKnots = map[VesselClass]speedEnvelope{
	ClassConventionalFerry:  {lo: 8, hi: 24},
	ClassRoPax:              {lo: 10, hi: 26},
	ClassHighSpeedCatamaran: {lo: 20, hi: 45},
	ClassHydrofoil:          {lo: 20, hi: 40},
	ClassLandingCraft:       {lo: 6, hi: 14},
}

// classOrder is the canonical order Classes reports.
//
// It is a SECOND declaration of the same set, which would normally be the drift
// this estate writes guards against — a map cannot be ranged over in a stable
// order, and callers that print a class list need one. The duplication is made
// safe by TestClassOrderAndEnvelopesAgree, which holds the two together in both
// directions: a class in one and not the other fails the build.
var classOrder = []VesselClass{
	ClassConventionalFerry,
	ClassRoPax,
	ClassHighSpeedCatamaran,
	ClassHydrofoil,
	ClassLandingCraft,
}

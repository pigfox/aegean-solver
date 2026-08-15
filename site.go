package aegean

import (
	"errors"
	"math"
)

// Site is one place on the map: a source of demand, a candidate for a facility,
// or — as in every instance this package was written for — both at once.
type Site struct {
	// ID is a stable key the caller assigns. This package never interprets it.
	ID string
	// Name is for display and never for identity.
	Name string
	// Lat and Lon are degrees, WGS84.
	Lat, Lon float64
	// Weight is the demand at this site — population, in the Aegean instance.
	// A zero weight is legal: the site still competes as a facility candidate
	// while contributing nothing to the objective.
	Weight float64
}

// The errors a caller can act on. They are values rather than formatted strings
// because the WASM shim turns each into a different message and matching on a
// sentinel is what keeps that mapping off the wording.
var (
	// ErrNoSites is an empty instance.
	ErrNoSites = errors.New("aegean: no sites")
	// ErrFacilityCount is a p or k outside 1..len(sites).
	ErrFacilityCount = errors.New("aegean: facility count must be between 1 and the number of sites")
	// ErrRadius is a non-positive coverage radius.
	ErrRadius = errors.New("aegean: coverage radius must be positive")
	// ErrTooManyCombinations refuses a brute-force search that would not finish.
	ErrTooManyCombinations = errors.New("aegean: too many combinations for an exhaustive search")
)

// earthRadiusKm is the mean radius. The great-circle distances here are used to
// COMPARE placements, so the absolute figure matters less than its consistency —
// but it is the standard mean radius rather than a rounded one, because a
// reported kilometre figure on a page should be a real kilometre.
const earthRadiusKm = 6371.0088

// DistanceKm is the great-circle distance between two sites in kilometres.
//
// HAVERSINE RATHER THAN A PLANAR APPROXIMATION, and at Aegean scale that is a
// deliberate choice rather than a reflex. The basin spans roughly 35°N to 41°N,
// where a degree of longitude is about 20% shorter than a degree of latitude;
// treating the plate carrée plane as metric would stretch every east-west leg by
// that much and quietly bias every placement northward. The cost of doing it
// properly is a handful of trigonometric calls per pair.
func DistanceKm(a, b Site) float64 {
	const deg = math.Pi / 180

	lat1 := a.Lat * deg
	lat2 := b.Lat * deg
	dLat := (b.Lat - a.Lat) * deg
	dLon := (b.Lon - a.Lon) * deg

	sinLat := math.Sin(dLat / 2)
	sinLon := math.Sin(dLon / 2)

	h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(clampUnit(h)))
}

// clampUnit holds a haversine term inside [0,1] before it reaches Asin.
//
// IT IS A NAMED FUNCTION RATHER THAN AN INLINE `if`, and that is about being
// able to test it. Accumulated rounding can push the term a hair above 1 for a
// near-antipodal pair, and Asin of 1.0000000002 is NaN rather than π/2 — a
// single NaN distance poisons every comparison downstream, because every
// comparison against NaN is false and the nearest-facility search would pick
// whatever it started with. No real pair of Aegean sites reaches that, and none
// of the test instances do either, so as an inline branch it was unreachable
// code that could only be verified by reading it. Pulled out, the contract is
// checkable directly and the safety is still there.
func clampUnit(x float64) float64 {
	if x > 1 {
		return 1
	}
	if x < 0 {
		return 0
	}
	return x
}

// Matrix is a precomputed all-pairs distance table in kilometres.
//
// It exists because both solvers read the same distances thousands of times.
// Teitz-Bart evaluates every (chosen, unchosen) swap on every pass, and each
// evaluation reassigns every demand point; recomputing a haversine inside that
// loop dominates the runtime for no reason. The table is symmetric and is built
// once per solve.
type Matrix struct {
	n int
	d []float64
}

// NewMatrix builds the all-pairs table.
func NewMatrix(sites []Site) *Matrix {
	n := len(sites)
	m := &Matrix{n: n, d: make([]float64, n*n)}
	for i := range n {
		for j := i + 1; j < n; j++ {
			km := DistanceKm(sites[i], sites[j])
			m.d[i*n+j] = km
			m.d[j*n+i] = km
		}
	}
	return m
}

// At returns the distance between two site indices.
func (m *Matrix) At(i, j int) float64 { return m.d[i*m.n+j] }

// N is the number of sites the table covers.
func (m *Matrix) N() int { return m.n }

// validate is the shared precondition check. Both entry points run it so a bad
// instance is refused identically rather than in two slightly different ways.
func validate(sites []Site, count int) error {
	if len(sites) == 0 {
		return ErrNoSites
	}
	if count < 1 || count > len(sites) {
		return ErrFacilityCount
	}
	return nil
}

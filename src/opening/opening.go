// Package opening names Octad's blind-deploy starting positions.
//
// Every game begins with each side arranging its four pieces (one king, one
// knight, two pawns) on its home rank. There are 4!/2! = 12 distinct
// arrangements per side ("formations"), and 12x12 = 144 ordered
// White-vs-Black "matchups". This package is the single source of truth for the
// human names of both — a pure function of a game's starting OFEN — so the
// archived PGN, the copied PGN, and the archive UI never disagree and no name
// table is duplicated on the client.
//
// A formation is keyed by its owner's own left-to-right home rank as a 4-char
// string of piece letters (k/n/p). The classic default start `nkpp` is "The
// Standard". The 12 names are military formations; they fall into six left/right
// mirror pairs that share a motif. The 144 matchup names are nature/science
// proper names, each grounded in the specific clash (see matchups).
package opening

import "strings"

// formationKeys is the canonical index order of the 12 formations: each is one
// arrangement of {king, knight, two pawns} on the four home-rank files, written
// from the owner's own left-to-right perspective. The index is used to key the
// matchup matrix.
var formationKeys = [12]string{
	"nkpp", // 0  The Standard — knight on the rim, king inside it, pawns joined right (the default start)
	"ppkn", // 1  The Ensign   — its mirror: pawns joined left, king inside, knight on the rim
	"knpp", // 2  The Redoubt  — king cornered behind the knight, pawns joined right
	"ppnk", // 3  The Bastion  — its mirror: king cornered behind the knight, pawns joined left
	"kppn", // 4  The Rampart  — king and knight anchor opposite corners, pawns wall the center
	"nppk", // 5  The Bulwark  — its mirror
	"pknp", // 6  The Keep     — king and knight held in the two center files, pawns on the wings
	"pnkp", // 7  The Citadel  — its mirror
	"npkp", // 8  The Skirmish — split pawns, king pushed off the wing: loose order
	"pkpn", // 9  The Sortie   — its mirror
	"kpnp", // 10 The Echelon  — fully alternating pieces: a staggered line
	"pnpk", // 11 The Cordon   — its mirror
}

// formationNames maps each formation key to its display name (military theme).
var formationNames = map[string]string{
	"nkpp": "The Standard",
	"ppkn": "The Ensign",
	"knpp": "The Redoubt",
	"ppnk": "The Bastion",
	"kppn": "The Rampart",
	"nppk": "The Bulwark",
	"pknp": "The Keep",
	"pnkp": "The Citadel",
	"npkp": "The Skirmish",
	"pkpn": "The Sortie",
	"kpnp": "The Echelon",
	"pnpk": "The Cordon",
}

// formationIndex maps a formation key to its index in formationKeys, and reports
// whether the key is one of the 12 legal formations.
var formationIndex = func() map[string]int {
	m := make(map[string]int, len(formationKeys))
	for i, k := range formationKeys {
		m[k] = i
	}
	return m
}()

// matchups[white][black] is the proper name of the opening reached when White
// deploys formation `white` and Black deploys formation `black` (indexes into
// formationKeys). White moves first, so the pairing is ordered and all 144 are
// distinct.
//
// The names are nature/science phenomena grounded in the specific clash:
//   - the diagonal (white == black) is the exactly 180-degree-symmetric
//     position, so each is a symmetric phenomenon (Standing Wave, Twin Peaks,
//     The Eye, Moire, ...);
//   - each row draws on the imagery of White's formation acting with the move —
//     the orthodox Standard/Ensign as tide and wave, the cornered
//     Redoubt/Bastion as bedrock and stone, the wall-building Rampart/Bulwark
//     as water and barrier, the central Keep/Citadel as star and energy, the
//     dispersed Skirmish/Sortie as fire and storm, the staggered Echelon/Cordon
//     as wave optics — meeting the character of Black's reply.
var matchups = [12][12]string{
	// White = The Standard (nkpp)
	{"Standing Wave", "Mirror Lake", "Storm Surge", "Lee Shore", "Levee Break", "Breakwater", "Solar Flare", "Tidal Siege", "Wildfire", "Backdraft", "Undertow", "Riptide"},
	// White = The Ensign (ppkn)
	{"Ebb Tide", "Antiphase", "Windward", "Sea Wall", "Floodgate", "Spillway", "Corona", "Neap Tide", "Brushfire", "Firestorm", "Crosswind", "Whitecaps"},
	// White = The Redoubt (knpp)
	{"Granite Tide", "Bedrock", "Basalt Columns", "Continental Drift", "Fault Line", "Tectonic Shift", "Caldera", "Total Eclipse", "Rockslide", "Landslide", "Strata", "Sediment"},
	// White = The Bastion (ppnk)
	{"Chalk Cliffs", "Terra Firma", "Rift Valley", "The Palisades", "Escarpment", "Cornerstone", "Magma Chamber", "Obsidian", "Scree", "Avalanche", "Moraine", "Talus"},
	// White = The Rampart (kppn)
	{"High Water", "Watermark", "Causeway", "Aqueduct", "The Watershed", "Continental Divide", "Reservoir", "Floodplain", "Delta", "Estuary", "Meltwater", "Confluence"},
	// White = The Bulwark (nppk)
	{"Slack Water", "Tidewater", "Berm", "Jetty", "The Divide", "The Isthmus", "Cistern", "Headwaters", "Braided River", "Backwater", "Cataract", "Millrace"},
	// White = The Keep (pknp)
	{"Sunspot", "Aurora", "Magnetar", "Supernova", "Plasma", "Ion Storm", "Twin Peaks", "Binary Star", "Solar Wind", "Heliostorm", "Pulsar", "Quasar"},
	// White = The Citadel (pnkp)
	{"Starfall", "Nebula", "Red Giant", "White Dwarf", "Solar Cycle", "Sunstorm", "Gravity Well", "The Eye", "Meteor Shower", "Photosphere", "Radio Burst", "Gamma Ray"},
	// White = The Skirmish (npkp)
	{"Embers", "Cinders", "Bushfire", "Firebreak", "Smolder", "Ashfall", "Ignition", "Flashpoint", "Brownian Motion", "Crossfire", "Wind Shear", "Dust Devil"},
	// White = The Sortie (pkpn)
	{"Squall", "Gale", "Flashover", "Conflagration", "Downdraft", "Sandstorm", "Combustion", "Shockfront", "Whirlwind", "Crosscurrent", "Turbulence", "Vortex"},
	// White = The Echelon (kpnp)
	{"Ripple", "Wavelength", "Diffraction", "Refraction", "Resonance", "Harmonic", "Amplitude", "Frequency", "Diffusion", "Dispersion", "Moiré", "Beat Pattern"},
	// White = The Cordon (pnpk)
	{"Wake", "Swell", "Reflection", "Echo", "Overtone", "Node", "Spectrum", "Prism", "Shimmer", "Mirage", "Interference", "Concentric"},
}

// Names resolves the White and Black formation names and the matchup name from a
// game's starting OFEN (e.g. "ppkn/4/4/NKPP w NCFncf - 0 1"). ok is false when
// the OFEN's home ranks are not a legal deploy start (e.g. a mid-game position);
// callers then omit the names. A real starting position always resolves.
func Names(startingOFEN string) (white, black, matchup string, ok bool) {
	board, _, found := strings.Cut(startingOFEN, " ")
	if !found {
		return "", "", "", false
	}
	ranks := strings.Split(board, "/")
	if len(ranks) != 4 {
		return "", "", "", false
	}

	// rank 1 (files a..d) is White's home rank; White's own perspective already
	// runs left-to-right, so its key is the rank lowercased as-is.
	whiteKey := strings.ToLower(ranks[3])
	// rank 4 is Black's home rank; Black's own perspective is the board order
	// reversed (its board is flipped in the client), so its key is the reverse.
	blackKey := reverse(strings.ToLower(ranks[0]))

	wi, wok := formationIndex[whiteKey]
	bi, bok := formationIndex[blackKey]
	if !wok || !bok {
		return "", "", "", false
	}
	return formationNames[whiteKey], formationNames[blackKey], matchups[wi][bi], true
}

// reverse returns s with its bytes reversed. Formation keys are 4 ASCII bytes,
// so a byte reversal is a rune reversal.
func reverse(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

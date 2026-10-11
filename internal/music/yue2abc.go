package music

// This file turns a song score that YuE2 planned into an instrumental score, for a YuE2
// server that cannot do it itself (YuE2-Turbo). It is a Go port of three parts of the
// MIT-licensed instrumental helper in the YuE2 repository
// (skills/yue2-music/instrumental/scripts, read at commit 1647252 on 2026-10-11):
//
//   - abc_tools.py parse: reads the two-voice ABC dialect (voices Vocal and Ins) of YuE2.
//   - instrumentalize.py convert_score: moves the Vocal notes to the Ins voice. Where an
//     Ins note overlaps a Vocal note, the Vocal note wins and the Ins note is shortened.
//   - compile_score.py compile_events: writes the notes back as ABC in that dialect.
//
// It also ports fit_score and instrumental_style from cav's own wrapper
// (tools/yue2-server/yue2_server.py). Both servers then get the same score for the same
// plan: the tests compare this port with the Python output, byte for byte.
//
// All times are exact fractions of a quarter note, as in the Python code.

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// frac is an exact fraction n/d with d > 0, always in lowest terms. The scores use only
// small denominators (powers of two and meter denominators), so int64 is enough.
type frac struct{ n, d int64 }

func fi(n int64) frac { return frac{n, 1} }

func mkfrac(n, d int64) frac {
	if d < 0 {
		n, d = -n, -d
	}
	g := gcd(abs64(n), d)
	if g == 0 {
		return frac{0, 1}
	}
	return frac{n / g, d / g}
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func lcm(a, b int64) int64 { return a / gcd(a, b) * b }

func abs64(a int64) int64 {
	if a < 0 {
		return -a
	}
	return a
}

func (a frac) add(b frac) frac  { return mkfrac(a.n*b.d+b.n*a.d, a.d*b.d) }
func (a frac) sub(b frac) frac  { return mkfrac(a.n*b.d-b.n*a.d, a.d*b.d) }
func (a frac) mul(b frac) frac  { return mkfrac(a.n*b.n, a.d*b.d) }
func (a frac) div(b frac) frac  { return mkfrac(a.n*b.d, a.d*b.n) }
func (a frac) cmp(b frac) int   { return cmpInt(a.n*b.d, b.n*a.d) }
func (a frac) less(b frac) bool { return a.cmp(b) < 0 }
func (a frac) float() float64   { return float64(a.n) / float64(a.d) }

// String writes the fraction like Python's Fraction: "3/2", or "4" for a whole number.
func (a frac) String() string {
	if a.d == 1 {
		return strconv.FormatInt(a.n, 10)
	}
	return fmt.Sprintf("%d/%d", a.n, a.d)
}

func minFrac(a, b frac) frac {
	if b.less(a) {
		return b
	}
	return a
}

func maxFrac(a, b frac) frac {
	if a.less(b) {
		return b
	}
	return a
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

var (
	abcDurations = []int64{48, 32, 24, 16, 12, 8, 6, 4, 3, 2, 1} // largest first
	abcQualities = []string{"", "m", "dim", "aug", "7", "maj7", "m7", "dim7", "m7b5",
		"sus4", "sus2", "6", "m6", "7sus4", "m(maj7)"}
	abcSections = []string{"intro", "verse", "pre-chorus", "chorus", "bridge", "interlude", "outro", "instrumental"}
	abcNatural  = map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}
	abcLetters  = "CDEFGAB"
	abcKeys     = map[string]int{}

	abcChord = func() *regexp.Regexp {
		var q []string
		for _, s := range abcQualities {
			q = append(q, regexp.QuoteMeta(s))
		}
		p := `[A-G](?:bb|##|b|#)?`
		return regexp.MustCompile(`^` + p + `(?:` + strings.Join(q, "|") + `)(?:/` + p + `)?$`)
	}()
	abcTokenSrc = `"(?P<chord>[^"\n]*)"|\[K:(?P<key>[^\]\n]+)\]|` +
		`(?P<acc>\^\^|__|\^|_|=)?(?P<note>[A-Ga-gz])(?P<oct>[,']*)(?P<duration>[0-9]*)(?P<tie>-?)`
	abcToken    = regexp.MustCompile(abcTokenSrc)
	abcTokenAt  = regexp.MustCompile(`^(?:` + abcTokenSrc + `)`)
	abcFullRest = regexp.MustCompile(`^Z([2-4])?$`)
)

func init() {
	for i, k := range []string{"Cb", "Gb", "Db", "Ab", "Eb", "Bb", "F", "C", "G", "D", "A", "E", "B", "F#", "C#"} {
		abcKeys[k] = i - 7
	}
	for i, k := range []string{"Abm", "Ebm", "Bbm", "Fm", "Cm", "Gm", "Dm", "Am", "Em", "Bm", "F#m", "C#m", "G#m", "D#m", "A#m"} {
		abcKeys[k] = i - 7
	}
}

func abcFail(format string, args ...any) error { return fmt.Errorf("ABC: "+format, args...) }

// keyAccidentals gives the alteration (-1, 0 or 1) of each letter in a key signature.
func keyAccidentals(key string) (map[byte]int, error) {
	count, ok := abcKeys[key]
	if !ok {
		return nil, abcFail("unsupported key %q", key)
	}
	out := map[byte]int{}
	for i := 0; i < len(abcLetters); i++ {
		out[abcLetters[i]] = 0
	}
	order, alt := "FCGDAEB", 1
	if count < 0 {
		order, alt, count = "BEADGCF", -1, -count
	}
	for i := 0; i < count; i++ {
		out[order[i]] = alt
	}
	return out, nil
}

var meterRe = regexp.MustCompile(`^([1-9][0-9]*)/([1-9][0-9]*)$`)

func meterValue(text string) ([2]int64, error) {
	m := meterRe.FindStringSubmatch(text)
	if m == nil {
		return [2]int64{}, abcFail("unsupported meter %q", text)
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	d, _ := strconv.ParseInt(m[2], 10, 64)
	if d > 1024 || d&(d-1) != 0 {
		return [2]int64{}, abcFail("unsupported meter denominator %d", d)
	}
	return [2]int64{n, d}, nil
}

type abcNote struct {
	t     frac
	pitch int
	dur   frac
}

func (a abcNote) cmp(b abcNote) int {
	if c := a.t.cmp(b.t); c != 0 {
		return c
	}
	if a.pitch != b.pitch {
		return cmpInt(int64(a.pitch), int64(b.pitch))
	}
	return a.dur.cmp(b.dur)
}

type abcBar struct {
	start, length frac
	meter         [2]int64
}

type abcEvent struct {
	t     frac
	value string // a chord symbol or a key
}

type abcVoice struct {
	meter   [2]int64
	key     string
	time    frac
	notes   []abcNote
	bars    []abcBar
	chords  []abcEvent
	keys    []abcEvent
	pending *[2]int // pitch and written pitch of a note tied to the next one
}

type abcScore struct {
	unit       frac
	bpm        int
	vocal, ins *abcVoice
	musicLines map[int]string // line index to voice name
}

func (s *abcScore) nominalSeconds() float64 {
	return s.vocal.time.mul(fi(60)).div(fi(int64(s.bpm))).float()
}

// splitLines splits like Python's str.splitlines for the "\n" and "\r\n" endings.
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func parseBar(body string, v *abcVoice, unit frac, where string) error {
	length := mkfrac(4*v.meter[0], v.meter[1])
	start, offset := v.time, fi(0)
	local := map[byte]int{} // accidentals hold by letter, across octaves, to the bar end
	sig, err := keyAccidentals(v.key)
	if err != nil {
		return err
	}
	if body == "Z" {
		if v.pending != nil {
			return abcFail("%s: a tie enters a full-measure rest", where)
		}
		offset = length
	} else {
		for cursor := 0; cursor < len(body); {
			if body[cursor] == ' ' || body[cursor] == '\t' {
				cursor++
				continue
			}
			m := abcTokenAt.FindStringSubmatchIndex(body[cursor:])
			if m == nil {
				return abcFail("%s: unsupported token at %q", where, clip(body[cursor:], 24))
			}
			group := func(name string) (string, bool) {
				i := abcTokenAt.SubexpIndex(name)
				if m[2*i] < 0 {
					return "", false
				}
				return body[cursor+m[2*i] : cursor+m[2*i+1]], true
			}
			next := cursor + m[1]
			if !offset.less(length) {
				return abcFail("%s: an event after the measure end", where)
			}
			if chord, ok := group("chord"); ok {
				if !abcChord.MatchString(chord) {
					return abcFail("%s: unsupported chord %q", where, chord)
				}
				v.chords = append(v.chords, abcEvent{start.add(offset), chord})
				cursor = next
				continue
			}
			if key, ok := group("key"); ok {
				if sig, err = keyAccidentals(key); err != nil {
					return err
				}
				v.key = key
				v.keys = append(v.keys, abcEvent{start.add(offset), key})
				local = map[byte]int{}
				cursor = next
				continue
			}
			note, _ := group("note")
			acc, _ := group("acc")
			oct, _ := group("oct")
			tie, _ := group("tie")
			ds, _ := group("duration")
			units := int64(1)
			if ds != "" {
				units, _ = strconv.ParseInt(ds, 10, 64)
			}
			if !slices.Contains(abcDurations, units) {
				return abcFail("%s: unsupported duration %d", where, units)
			}
			dur := fi(units).mul(unit).mul(fi(4))
			if length.less(offset.add(dur)) {
				return abcFail("%s: a note or rest exceeds the meter", where)
			}
			if strings.Contains(oct, ",") && strings.Contains(oct, "'") {
				return abcFail("%s: mixed octave marks", where)
			}
			if note == "z" {
				if acc != "" || oct != "" || tie != "" {
					return abcFail("%s: a rest cannot have accidentals, octave marks or ties", where)
				}
				if v.pending != nil {
					return abcFail("%s: a tie enters a rest", where)
				}
			} else {
				letter := strings.ToUpper(note)[0]
				written := 60 + abcNatural[letter]
				if note[0] >= 'a' {
					written += 12
				}
				written += 12 * (strings.Count(oct, "'") - strings.Count(oct, ","))
				alteration, ok := local[letter]
				if !ok {
					alteration = sig[letter]
				}
				if acc != "" {
					alteration = map[string]int{"=": 0, "_": -1, "__": -2, "^": 1, "^^": 2}[acc]
					local[letter] = alteration
				}
				pitch := written + alteration
				if v.pending != nil {
					// An unmarked continuation keeps its tied accidental across a barline.
					if acc == "" && written == v.pending[1] {
						pitch = v.pending[0]
					}
					if pitch != v.pending[0] {
						return abcFail("%s: a tie changes the pitch", where)
					}
					last := &v.notes[len(v.notes)-1]
					last.dur = last.dur.add(dur)
				} else {
					if pitch < 0 || pitch > 127 {
						return abcFail("%s: pitch %d is outside the MIDI range", where, pitch)
					}
					v.notes = append(v.notes, abcNote{start.add(offset), pitch, dur})
				}
				v.pending = nil
				if tie != "" {
					v.pending = &[2]int{pitch, written}
				}
			}
			offset = offset.add(dur)
			cursor = next
		}
	}
	if offset != length {
		return abcFail("%s: the notes fill %v quarter notes, the meter %v", where, offset, length)
	}
	v.bars = append(v.bars, abcBar{start, length, v.meter})
	v.time = v.time.add(length)
	return nil
}

var (
	unitRe  = regexp.MustCompile(`^L:1/([1-9][0-9]*)$`)
	tempoRe = regexp.MustCompile(`^Q:1/4=([1-9][0-9]*)$`)
)

const (
	abcVocalDef = `V: Vocal clef=treble name="Vocal Melody" snm="Vocal"`
	abcInsDef   = `V: Ins clef=treble name="Ins Melody" snm="Inst."`
)

// parseABC reads a score in YuE2's native two-voice dialect and refuses anything else.
func parseABC(text string) (*abcScore, error) {
	lines := splitLines(text)
	if len(lines) < 12 {
		return nil, abcFail("an incomplete two-voice score")
	}
	if lines[0] != "X:1" || lines[1] != "T:" {
		return nil, abcFail("expected the X:1 and blank T: header")
	}
	if !strings.HasPrefix(lines[2], "M:") {
		return nil, abcFail("missing the M: header")
	}
	meter, err := meterValue(lines[2][2:])
	if err != nil {
		return nil, err
	}
	um := unitRe.FindStringSubmatch(lines[3])
	if um == nil {
		return nil, abcFail("expected L:1/<power of two>")
	}
	den, _ := strconv.ParseInt(um[1], 10, 64)
	if den > 1024 || den&(den-1) != 0 {
		return nil, abcFail("unsupported L: denominator")
	}
	tm := tempoRe.FindStringSubmatch(lines[4])
	if tm == nil {
		return nil, abcFail("expected Q:1/4=<BPM>")
	}
	bpm, _ := strconv.Atoi(tm[1])
	if lines[5] != abcVocalDef || lines[6] != abcInsDef {
		return nil, abcFail("expected the native Vocal and Ins voice definitions")
	}
	if !strings.HasPrefix(lines[7], "K:") {
		return nil, abcFail("missing the K: header")
	}
	key := lines[7][2:]
	if _, err := keyAccidentals(key); err != nil {
		return nil, err
	}
	s := &abcScore{unit: mkfrac(1, den), bpm: bpm, musicLines: map[int]string{}}
	s.vocal = &abcVoice{meter: meter, key: key, time: fi(0), keys: []abcEvent{{fi(0), key}}}
	s.ins = &abcVoice{meter: meter, key: key, time: fi(0), keys: []abcEvent{{fi(0), key}}}
	cursor, group := 8, 0
	for cursor < len(lines) {
		for cursor < len(lines) && strings.HasPrefix(lines[cursor], "% ") {
			cursor++
		}
		if cursor == len(lines) {
			return nil, abcFail("a section comment without music")
		}
		group++
		var counts []int
		for _, name := range []string{"Vocal", "Ins"} {
			where := fmt.Sprintf("group %d, %s", group, name)
			if cursor >= len(lines) || lines[cursor] != "V: "+name {
				return nil, abcFail("%s: expected V: %s", where, name)
			}
			cursor++
			v := s.vocal
			if name == "Ins" {
				v = s.ins
			}
			fields := map[string]bool{}
			for cursor < len(lines) && (strings.HasPrefix(lines[cursor], "M:") || strings.HasPrefix(lines[cursor], "K:")) {
				field, value := lines[cursor][:1], lines[cursor][2:]
				if fields[field] {
					return nil, abcFail("%s: a duplicate %s: field", where, field)
				}
				fields[field] = true
				if field == "M" {
					if v.meter, err = meterValue(value); err != nil {
						return nil, err
					}
				} else {
					if _, err := keyAccidentals(value); err != nil {
						return nil, err
					}
					v.key = value
					v.keys = append(v.keys, abcEvent{v.time, value})
				}
				cursor++
			}
			if cursor >= len(lines) {
				return nil, abcFail("%s: missing the music line", where)
			}
			line := lines[cursor]
			if !strings.HasSuffix(line, "|") {
				return nil, abcFail("%s: the music line must end with a plain barline", where)
			}
			s.musicLines[cursor] = name
			cursor++
			var bars []string
			for _, bar := range strings.Split(line[:len(line)-1], "|") {
				bar = strings.TrimSpace(bar)
				if bar == "" {
					return nil, abcFail("%s: an empty measure, or an unsupported double or repeat barline", where)
				}
				if m := abcFullRest.FindStringSubmatch(bar); m != nil {
					n := 1
					if m[1] != "" {
						n, _ = strconv.Atoi(m[1])
					}
					for range n {
						bars = append(bars, "Z")
					}
				} else {
					bars = append(bars, bar)
				}
			}
			if len(bars) < 1 || len(bars) > 4 {
				return nil, abcFail("%s: expected 1 to 4 measures", where)
			}
			counts = append(counts, len(bars))
			for _, bar := range bars {
				if err := parseBar(bar, v, s.unit, fmt.Sprintf("%s, bar %d", where, len(v.bars)+1)); err != nil {
					return nil, err
				}
			}
		}
		if counts[0] != counts[1] {
			return nil, abcFail("group %d: the voices have different measure counts", group)
		}
	}
	if s.vocal.pending != nil || s.ins.pending != nil {
		return nil, abcFail("an unresolved tie at the end of the score")
	}
	if len(s.ins.chords) > 0 {
		return nil, abcFail("chord symbols belong in Vocal, not Ins")
	}
	if !slices.Equal(s.vocal.bars, s.ins.bars) {
		return nil, abcFail("the voices have different meter or time grids")
	}
	if !slices.Equal(s.vocal.keys, s.ins.keys) {
		return nil, abcFail("the voices have different key changes")
	}
	return s, nil
}

// sectionStarts maps the section comments ("% verse") to the start time of their first bar.
func sectionStarts(text string, s *abcScore) (map[frac]string, error) {
	out := map[frac]string{}
	cursor, pending := 0, ""
	for i, line := range splitLines(text) {
		if strings.HasPrefix(line, "% ") {
			pending = line[2:]
			if !slices.Contains(abcSections, pending) {
				return nil, abcFail("unknown section label %q", pending)
			}
		}
		if s.musicLines[i] != "Vocal" {
			continue
		}
		if pending != "" {
			out[s.vocal.bars[cursor].start] = pending
			pending = ""
		}
		for _, bar := range strings.Split(line[:len(line)-1], "|") {
			if m := abcFullRest.FindStringSubmatch(strings.TrimSpace(bar)); m != nil && m[1] != "" {
				n, _ := strconv.Atoi(m[1])
				cursor += n
			} else {
				cursor++
			}
		}
	}
	return out, nil
}

// abcEvents is the input of compileEvents. A bar with an empty section keeps the section
// of the bar before it.
type abcEvents struct {
	bpm    int
	key    string
	bars   []abcEventBar
	notes  []abcNote
	chords []abcEvent
}

type abcEventBar struct{ meter, key, section string }

// abcLengths splits a whole number of units into the supported note lengths.
func abcLengths(v frac) ([]int64, error) {
	if v.d != 1 || v.n <= 0 {
		return nil, abcFail("a length of %v units cannot be written", v)
	}
	left := v.n
	var out []int64
	for _, part := range abcDurations {
		for left >= part {
			out = append(out, part)
			left -= part
		}
	}
	return out, nil
}

// spelling writes a MIDI pitch as an ABC note in a key, updating the accidentals already
// active in the bar.
func spelling(pitch int, key string, active map[byte]int) string {
	sig, _ := keyAccidentals(key)
	sum := 0
	for _, a := range sig {
		sum += a
	}
	preferSharp := sum >= 0
	type cand struct {
		score              [3]int
		letter             byte
		alteration, octave int
	}
	var best *cand
	b2i := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	for i := 0; i < len(abcLetters); i++ {
		letter := abcLetters[i]
		for _, alt := range []int{-1, 0, 1} {
			base := pitch - alt - 60 - abcNatural[letter]
			if base%12 != 0 {
				continue
			}
			wrong := alt > 0
			if preferSharp {
				wrong = alt < 0
			}
			c := cand{[3]int{b2i(alt != sig[letter]), max(alt, -alt), b2i(wrong)}, letter, alt, base / 12}
			if best == nil || candLess(c.score, c.letter, c.alteration, c.octave, best.score, best.letter, best.alteration, best.octave) {
				cc := c
				best = &cc
			}
		}
	}
	acc := ""
	cur, ok := active[best.letter]
	if !ok {
		cur = sig[best.letter]
	}
	if best.alteration != cur {
		acc = map[int]string{-1: "_", 0: "=", 1: "^"}[best.alteration]
		active[best.letter] = best.alteration
	}
	name := string(best.letter)
	switch {
	case best.octave < 0:
		name += strings.Repeat(",", -best.octave)
	case best.octave > 0:
		name = strings.ToLower(name) + strings.Repeat("'", best.octave-1)
	}
	return acc + name
}

// candLess orders spelling candidates the way Python orders their tuples.
func candLess(as [3]int, al byte, aa, ao int, bs [3]int, bl byte, ba, bo int) bool {
	for i := range as {
		if as[i] != bs[i] {
			return as[i] < bs[i]
		}
	}
	if al != bl {
		return al < bl
	}
	if aa != ba {
		return aa < ba
	}
	return ao < bo
}

func compressBars(bars []string) string {
	var b strings.Builder
	for i := 0; i < len(bars); {
		if bars[i] != "Z" {
			b.WriteString(bars[i] + "|")
			i++
			continue
		}
		end := i + 1
		for end < len(bars) && bars[end] == "Z" {
			end++
		}
		b.WriteString("Z")
		if end-i > 1 {
			b.WriteString(strconv.Itoa(end - i))
		}
		b.WriteString("|")
		i = end
	}
	return b.String()
}

func restsFor(v frac) ([]string, error) {
	ls, err := abcLengths(v)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range ls {
		out = append(out, "z"+lenSuffix(n))
	}
	return out, nil
}

func lenSuffix(n int64) string {
	if n == 1 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

// dedupChords drops a chord that repeats the one before it.
func dedupChords(in []abcEvent) []abcEvent {
	var out []abcEvent
	for _, e := range in {
		if len(out) == 0 || out[len(out)-1].value != e.value {
			out = append(out, e)
		}
	}
	return out
}

// compileEvents writes notes, chords and bars as a score in YuE2's dialect: all notes in
// Ins, the chord symbols over rests in Vocal.
func compileEvents(ev abcEvents) (string, error) {
	if ev.bpm <= 0 {
		return "", abcFail("the tempo must be a positive whole number")
	}
	if len(ev.bars) == 0 {
		return "", abcFail("a score needs bars")
	}
	type cbar struct {
		start, end          frac
		meter, key, section string
	}
	var bars []cbar
	t, meter, key, section := fi(0), "4/4", ev.key, ""
	unitDen := int64(32)
	for i, b := range ev.bars {
		if b.meter != "" {
			meter = b.meter
		}
		if b.key != "" {
			key = b.key
		}
		m, err := meterValue(meter)
		if err != nil {
			return "", err
		}
		if _, err := keyAccidentals(key); err != nil {
			return "", err
		}
		unitDen = lcm(unitDen, 4*m[1])
		if b.section != "" {
			if !slices.Contains(abcSections, b.section) {
				return "", abcFail("bar %d: unknown section %q", i+1, b.section)
			}
			section = b.section
		}
		end := t.add(mkfrac(4*m[0], m[1]))
		bars = append(bars, cbar{t, end, meter, key, section})
		t = end
	}
	notes := slices.Clone(ev.notes)
	for i, n := range notes {
		if n.pitch < 0 || n.pitch > 127 {
			return "", abcFail("note %d: the pitch is outside the MIDI range", i+1)
		}
		if n.t.less(fi(0)) || !fi(0).less(n.dur) || t.less(n.t.add(n.dur)) {
			return "", abcFail("note %d: outside the score", i+1)
		}
		unitDen = lcm(unitDen, lcm(n.t.div(fi(4)).d, n.dur.div(fi(4)).d))
	}
	slices.SortFunc(notes, abcNote.cmp)
	if len(notes) == 0 {
		return "", abcFail("the instrumental score has no notes")
	}
	for i := 1; i < len(notes); i++ {
		if notes[i].t.less(notes[i-1].t.add(notes[i-1].dur)) {
			return "", abcFail("overlapping notes at quarter %v", notes[i].t)
		}
	}
	chords := slices.Clone(ev.chords)
	for i, c := range chords {
		if c.t.less(fi(0)) || !c.t.less(t) || !abcChord.MatchString(c.value) {
			return "", abcFail("chord %d: unsupported symbol or onset", i+1)
		}
		unitDen = lcm(unitDen, c.t.div(fi(4)).d)
	}
	sort.SliceStable(chords, func(i, j int) bool {
		if c := chords[i].t.cmp(chords[j].t); c != 0 {
			return c < 0
		}
		return chords[i].value < chords[j].value
	})
	for i := 1; i < len(chords); i++ {
		if chords[i].t == chords[i-1].t {
			return "", abcFail("two chords at the same time")
		}
	}
	if unitDen > 1024 || unitDen&(unitDen-1) != 0 {
		return "", abcFail("the rhythm is outside the power-of-two grid")
	}
	unitQ := mkfrac(4, unitDen)
	var ins, vocal []string
	for _, bar := range bars {
		var pieces []string
		cursor, active, has := bar.start, map[byte]int{}, false
		for _, n := range notes {
			if !(n.t.less(bar.end) && bar.start.less(n.t.add(n.dur))) {
				continue
			}
			has = true
			a, b := maxFrac(bar.start, n.t), minFrac(bar.end, n.t.add(n.dur))
			if cursor.less(a) {
				r, err := restsFor(a.sub(cursor).div(unitQ))
				if err != nil {
					return "", err
				}
				pieces = append(pieces, r...)
			}
			parts, err := abcLengths(b.sub(a).div(unitQ))
			if err != nil {
				return "", err
			}
			for i, count := range parts {
				tie := ""
				if i < len(parts)-1 || b.less(n.t.add(n.dur)) {
					tie = "-"
				}
				pieces = append(pieces, spelling(n.pitch, bar.key, active)+lenSuffix(count)+tie)
			}
			cursor = b
		}
		if cursor.less(bar.end) {
			r, err := restsFor(bar.end.sub(cursor).div(unitQ))
			if err != nil {
				return "", err
			}
			pieces = append(pieces, r...)
		}
		if has {
			ins = append(ins, strings.Join(pieces, ""))
		} else {
			ins = append(ins, "Z")
		}
		current := ""
		for _, c := range chords {
			if !bar.start.less(c.t) {
				current = c.value
			}
		}
		changes := []abcEvent{{bar.start, current}}
		for _, c := range chords {
			if bar.start.less(c.t) && c.t.less(bar.end) {
				changes = append(changes, c)
			}
		}
		var chunks []string
		named := false
		for i, ch := range changes {
			stop := bar.end
			if i+1 < len(changes) {
				stop = changes[i+1].t
			}
			if ch.value != "" {
				chunks = append(chunks, `"`+ch.value+`"`)
				named = true
			}
			r, err := restsFor(stop.sub(ch.t).div(unitQ))
			if err != nil {
				return "", err
			}
			chunks = append(chunks, r...)
		}
		if named {
			vocal = append(vocal, strings.Join(chunks, ""))
		} else {
			vocal = append(vocal, "Z")
		}
	}
	lines := []string{"X:1", "T:", "M:" + bars[0].meter, fmt.Sprintf("L:1/%d", unitDen), fmt.Sprintf("Q:1/4=%d", ev.bpm),
		abcVocalDef, abcInsDef, "K:" + bars[0].key}
	for i := 0; i < len(bars); {
		bar := bars[i]
		prev := bar
		if i > 0 {
			prev = bars[i-1]
		}
		end := i + 1
		for end < len(bars) && end-i < 4 && bars[end].meter == bar.meter && bars[end].key == bar.key && bars[end].section == bar.section {
			end++
		}
		if bar.section != "" && (i == 0 || bar.section != prev.section) {
			lines = append(lines, "% "+bar.section)
		}
		for _, voice := range []struct {
			name     string
			rendered []string
		}{{"Vocal", vocal}, {"Ins", ins}} {
			lines = append(lines, "V: "+voice.name)
			if bar.meter != prev.meter {
				lines = append(lines, "M:"+bar.meter)
			}
			if bar.key != prev.key {
				lines = append(lines, "K:"+bar.key)
			}
			lines = append(lines, compressBars(voice.rendered[i:end]))
		}
		i = end
	}
	text := strings.Join(lines, "\n") + "\n"
	parsed, err := parseABC(text)
	if err != nil {
		return "", fmt.Errorf("the compiled score does not read back: %w", err)
	}
	if !slices.Equal(parsed.ins.notes, notes) || len(parsed.vocal.notes) > 0 {
		return "", abcFail("the compiled notes do not read back the same")
	}
	if !slices.Equal(dedupChords(parsed.vocal.chords), dedupChords(chords)) {
		return "", abcFail("the compiled chords do not read back the same")
	}
	return text, nil
}

// barKey is the key in force at a time.
func barKey(keys []abcEvent, at frac) string {
	k := ""
	for _, e := range keys {
		if !at.less(e.t) {
			k = e.value
		}
	}
	return k
}

// subtractIntervals removes the occupied spans from [start, start+dur).
func subtractIntervals(start, dur frac, occupied [][2]frac) [][2]frac {
	pieces := [][2]frac{{start, start.add(dur)}}
	for _, o := range occupied {
		var rest [][2]frac
		for _, p := range pieces {
			if !o[0].less(p[1]) || !p[0].less(o[1]) { // no overlap
				rest = append(rest, p)
				continue
			}
			if p[0].less(o[0]) {
				rest = append(rest, [2]frac{p[0], o[0]})
			}
			if o[1].less(p[1]) {
				rest = append(rest, [2]frac{o[1], p[1]})
			}
		}
		pieces = rest
	}
	return pieces
}

// convertScore moves the Vocal notes of a planned score to Ins and keeps the chords. It
// returns the new score and the number of Vocal notes it moved.
func convertScore(text string) (string, int, error) {
	if strings.TrimSpace(text) == "" {
		return "", 0, abcFail("an empty score")
	}
	src, err := parseABC(text)
	if err != nil {
		return "", 0, err
	}
	vocal, ins := src.vocal, src.ins
	if len(vocal.notes) == 0 && len(ins.notes) == 0 {
		return "", 0, abcFail("the score has no notes")
	}
	var occupied [][2]frac
	for _, n := range vocal.notes {
		occupied = append(occupied, [2]frac{n.t, n.t.add(n.dur)})
	}
	merged := slices.Clone(vocal.notes)
	for _, n := range ins.notes {
		for _, p := range subtractIntervals(n.t, n.dur, occupied) {
			merged = append(merged, abcNote{p[0], n.pitch, p[1].sub(p[0])})
		}
	}
	slices.SortFunc(merged, abcNote.cmp)
	sections, err := sectionStarts(text, src)
	if err != nil {
		return "", 0, err
	}
	starts := map[frac]bool{}
	for _, b := range vocal.bars {
		starts[b.start] = true
	}
	for _, k := range vocal.keys {
		if !starts[k.t] {
			return "", 0, abcFail("a key change inside a bar")
		}
	}
	var bars []abcEventBar
	for _, b := range vocal.bars {
		bars = append(bars, abcEventBar{meter: fmt.Sprintf("%d/%d", b.meter[0], b.meter[1]), key: barKey(vocal.keys, b.start), section: sections[b.start]})
	}
	var chords []abcEvent
	for _, c := range vocal.chords {
		if len(chords) > 0 && chords[len(chords)-1].t == c.t {
			if chords[len(chords)-1].value != c.value {
				return "", 0, abcFail("two different chords at the same time")
			}
			continue
		}
		chords = append(chords, c)
	}
	out, err := compileEvents(abcEvents{bpm: src.bpm, key: vocal.keys[0].value, bars: bars, notes: merged, chords: chords})
	if err != nil {
		return "", 0, err
	}
	target, err := parseABC(out)
	if err != nil {
		return "", 0, err
	}
	if !slices.Equal(target.ins.notes, merged) || len(target.vocal.notes) > 0 {
		return "", 0, abcFail("moving the voice changed the notes")
	}
	if !slices.Equal(target.ins.bars, ins.bars) || target.bpm != src.bpm {
		return "", 0, abcFail("moving the voice changed the meter, timing or tempo")
	}
	if !slices.Equal(dedupChords(target.vocal.chords), dedupChords(vocal.chords)) {
		return "", 0, abcFail("moving the voice changed the harmony")
	}
	return out, len(vocal.notes), nil
}

var (
	adjacentRests = regexp.MustCompile(`Z[234]?\|Z[234]?(?:\||$)`)
	onlyRests     = regexp.MustCompile(`^(?:z\d*)+$`)
)

// validateInstrumental checks an instrumental score the way the helper's validate_score
// does before it generates: no Vocal notes, Ins notes, and the native notation.
func validateInstrumental(text string) (*abcScore, error) {
	s, err := parseABC(text)
	if err != nil {
		return nil, err
	}
	if len(s.vocal.notes) > 0 {
		return nil, abcFail("Vocal still has notes")
	}
	if len(s.ins.notes) == 0 {
		return nil, abcFail("Ins has no notes")
	}
	for _, b := range s.ins.bars {
		if s.unit.d%(4*b.meter[1]) != 0 {
			return nil, abcFail("L is too coarse for the four-subbeat grid")
		}
	}
	lines := splitLines(text)
	previous := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "% ") {
			label := line[2:]
			if !slices.Contains(abcSections, label) || label == previous {
				return nil, abcFail("unknown or repeated section label %q", label)
			}
			previous = label
		}
	}
	for i := range s.musicLines {
		line := lines[i]
		if adjacentRests.MatchString(line) {
			return nil, abcFail("adjacent full-bar rests must be one Z2, Z3 or Z4")
		}
		for _, bar := range strings.Split(line[:len(line)-1], "|") {
			if onlyRests.MatchString(bar) {
				return nil, abcFail("a full-bar rest must be Z")
			}
			for _, m := range abcToken.FindAllStringSubmatch(bar, -1) {
				if m[abcToken.SubexpIndex("duration")] == "1" {
					return nil, abcFail("an explicit duration 1")
				}
			}
		}
	}
	return s, nil
}

// lyricTags lists the section labels of a score as YuE2 lyrics: "[Verse]\n\n[Chorus]\n".
func lyricTags(text string) string {
	var tags []string
	for _, line := range splitLines(text) {
		if strings.HasPrefix(line, "% ") {
			tags = append(tags, "["+pyTitle(line[2:])+"]")
		}
	}
	if len(tags) == 0 {
		return ""
	}
	return strings.Join(tags, "\n\n") + "\n"
}

// pyTitle capitalises each word like Python's str.title: "pre-chorus" becomes "Pre-Chorus".
func pyTitle(s string) string {
	b := []byte(strings.ToLower(s))
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' && (i == 0 || !isLetter(b[i-1])) {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

var (
	instrumentalPrefix = regexp.MustCompile(`(?i)^instrumental\b`)
	noVocals           = []string{"no vocals", "no singing", "no choir", "no spoken words"}
)

// instrumentalStyle is the style the helper writes for an instrumental: it starts with
// "Instrumental" and says there are no vocals.
func instrumentalStyle(style string) string {
	style = strings.TrimRight(strings.TrimSpace(style), ".,")
	if !instrumentalPrefix.MatchString(style) {
		style = "Instrumental, " + style
	}
	for _, c := range noVocals {
		if !strings.Contains(strings.ToLower(style), c) {
			style += ", " + c
		}
	}
	return style + "."
}

// fitScore cuts an instrumental score to about `seconds`, like fit_score in cav's wrapper.
// It keeps the score from the start up to a cut point (a section start, or every fourth bar
// inside a section) and, when the score ends with an outro, the whole outro after it. It
// picks the cut whose length is closest to the request. It returns the new score and its
// nominal length; a score that is already the closest comes back unchanged.
func fitScore(text string, seconds float64) (string, float64, error) {
	s, err := parseABC(text)
	if err != nil {
		return "", 0, err
	}
	vocal, ins := s.vocal, s.ins
	secPerQuarter := mkfrac(60, int64(s.bpm))
	total := vocal.time
	starts, err := sectionStarts(text, s)
	if err != nil {
		return "", 0, err
	}
	set := map[frac]bool{fi(0): true}
	for t := range starts {
		set[t] = true
	}
	var sections []frac
	for t := range set {
		sections = append(sections, t)
	}
	slices.SortFunc(sections, frac.cmp)
	ends := append(slices.Clone(sections[1:]), total)
	type span [2]frac
	var phrases []span
	var outro *span
	for i, a := range sections {
		b := ends[i]
		if starts[a] == "outro" && b == total {
			outro = &span{a, b}
			continue
		}
		var inside []frac
		for _, bar := range vocal.bars {
			if !bar.start.less(a) && bar.start.less(b) {
				inside = append(inside, bar.start)
			}
		}
		var cuts []frac
		for j := 0; j < len(inside); j += 4 {
			cuts = append(cuts, inside[j])
		}
		cuts = append(cuts, b)
		for j := 0; j+1 < len(cuts); j++ {
			phrases = append(phrases, span{cuts[j], cuts[j+1]})
		}
	}
	target := seconds / secPerQuarter.float()
	var options [][]span
	for k := 1; k <= len(phrases); k++ {
		options = append(options, slices.Clone(phrases[:k]))
		if outro != nil {
			options = append(options, append(slices.Clone(phrases[:k]), *outro))
		}
	}
	if outro != nil {
		options = append(options, []span{*outro})
	}
	// A cut that keeps only silent bars has nothing to play: keep a cut with a note in it.
	// The whole score is always one of the options, and it has notes.
	options = slices.DeleteFunc(options, func(o []span) bool {
		for _, seg := range o {
			for _, n := range ins.notes {
				if n.t.less(seg[1]) && seg[0].less(n.t.add(n.dur)) {
					return false
				}
			}
		}
		return true
	})
	length := func(o []span) frac {
		sum := fi(0)
		for _, p := range o {
			sum = sum.add(p[1].sub(p[0]))
		}
		return sum
	}
	diff := func(o []span) float64 {
		d := length(o).float() - target
		return max(d, -d)
	}
	best := options[0]
	for _, o := range options[1:] {
		if diff(o) < diff(best) || (diff(o) == diff(best) && len(o) < len(best)) {
			best = o
		}
	}
	all := slices.Clone(phrases)
	if outro != nil {
		all = append(all, *outro)
	}
	if slices.Equal(best, all) {
		return text, total.mul(secPerQuarter).float(), nil
	}
	// Rebuild the score from the kept segments, shifted to follow one another.
	var bars []abcEventBar
	var notes []abcNote
	var sources []int // the index in ins.notes of each kept note
	var chords []abcEvent
	at := fi(0)
	for _, seg := range best {
		a, b := seg[0], seg[1]
		shift := at.sub(a)
		for _, bar := range vocal.bars {
			if !bar.start.less(a) && bar.start.less(b) {
				bars = append(bars, abcEventBar{meter: fmt.Sprintf("%d/%d", bar.meter[0], bar.meter[1]),
					key: barKey(vocal.keys, bar.start), section: starts[bar.start]})
			}
		}
		// A note that sounds across a cut keeps its part inside the segment. When the next
		// segment follows on, its two parts join again into one held note.
		for i, n := range ins.notes {
			lo, hi := maxFrac(n.t, a), minFrac(n.t.add(n.dur), b)
			if !lo.less(hi) {
				continue
			}
			if k := len(notes) - 1; k >= 0 && sources[k] == i && notes[k].t.add(notes[k].dur) == lo.add(shift) {
				notes[k].dur = notes[k].dur.add(hi.sub(lo))
				continue
			}
			notes = append(notes, abcNote{lo.add(shift), n.pitch, hi.sub(lo)})
			sources = append(sources, i)
		}
		active := ""
		for _, c := range vocal.chords {
			if !a.less(c.t) {
				active = c.value
			}
		}
		if active != "" {
			chords = append(chords, abcEvent{at, active})
		}
		for _, c := range vocal.chords {
			if a.less(c.t) && c.t.less(b) {
				chords = append(chords, abcEvent{c.t.add(shift), c.value})
			}
		}
		at = at.add(b.sub(a))
	}
	var dedup []abcEvent
	for _, c := range chords {
		switch {
		case len(dedup) > 0 && dedup[len(dedup)-1].t == c.t:
			dedup[len(dedup)-1] = c
		case len(dedup) == 0 || dedup[len(dedup)-1].value != c.value:
			dedup = append(dedup, c)
		}
	}
	fitted, err := compileEvents(abcEvents{bpm: s.bpm, key: bars[0].key, bars: bars, notes: notes, chords: dedup})
	if err != nil {
		return "", 0, err
	}
	return fitted, at.mul(secPerQuarter).float(), nil
}

// yue2Instrumental is what cav sends YuE2-Turbo for an instrumental take.
type yue2Instrumental struct {
	abc, style, lyrics, cot string
	planned, nominal, bpm   float64
	vocalNotesMoved         int
}

// makeInstrumental turns a planned song score into an instrumental one, cut to about
// `seconds` when that is more than zero.
func makeInstrumental(planned, style string, seconds float64) (*yue2Instrumental, error) {
	converted, moved, err := convertScore(planned)
	if err != nil {
		return nil, err
	}
	s, err := validateInstrumental(converted)
	if err != nil {
		return nil, err
	}
	out := &yue2Instrumental{abc: converted, planned: s.nominalSeconds(), nominal: s.nominalSeconds(),
		bpm: float64(s.bpm), vocalNotesMoved: moved}
	if seconds > 0 {
		if out.abc, out.nominal, err = fitScore(converted, seconds); err != nil {
			return nil, err
		}
		if s, err = validateInstrumental(out.abc); err != nil {
			return nil, err
		}
	}
	if len(out.abc) > 64000 {
		return nil, errors.New("the instrumental score is longer than the 64000 characters YuE2-Turbo takes; ask for fewer seconds")
	}
	out.style = instrumentalStyle(style)
	out.lyrics = lyricTags(out.abc)
	if out.lyrics == "" {
		// YuE2-Turbo refuses blank lyrics; a score with no section labels is one section.
		out.lyrics = "[Instrumental]\n"
	}
	out.cot = "melody"
	if len(s.vocal.chords) > 0 {
		out.cot = "full"
	}
	return out, nil
}

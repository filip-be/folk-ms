package domain

import (
	"regexp"
	"strconv"
	"strings"
)

// ABC notation (abcnotation.com, standard 2.1), as far as the Tune
// Collection needs it: the header fields that propose a Tune's name, Key,
// Time Signature, Tune Type, Origin and Composer, a bar counter that cuts
// the Incipit, and splitting a multi-tune file at X:. The CMS never renders
// ABC; abcjs does that in the browser.

// Key is a Tune's key: a tonic (A–G with an optional # or b) and a mode.
type Key struct {
	Tonic string `json:"tonic"`
	Mode  string `json:"mode"`
}

// Modes a Key can have, in the order the admin lists them.
var Modes = []string{"major", "minor", "dorian", "mixolydian", "lydian", "phrygian", "locrian"}

var tonicRe = regexp.MustCompile(`^[A-G][#b]?$`)

// Valid reports whether k has a known tonic and mode.
func (k Key) Valid() bool {
	if !tonicRe.MatchString(k.Tonic) {
		return false
	}
	for _, m := range Modes {
		if k.Mode == m {
			return true
		}
	}
	return false
}

// String is the key as ABC writes it: "G", "Em", "Ador", "Dmix".
func (k Key) String() string {
	switch k.Mode {
	case "major":
		return k.Tonic
	case "minor":
		return k.Tonic + "m"
	}
	if len(k.Mode) < 3 {
		return k.Tonic
	}
	return k.Tonic + k.Mode[:3]
}

// modeOf reads a mode as ABC writes it after the tonic: "", "m", "maj",
// "minor", "Dor", "mixolydian", … Ionian and Aeolian are major and minor.
// Anything else (clef=, explicit accidentals, "exp") leaves major.
func modeOf(s string) string {
	s = strings.ToLower(s)
	if s == "m" {
		return "minor"
	}
	if len(s) < 3 {
		return "major"
	}
	switch s[:3] {
	case "min", "aeo":
		return "minor"
	case "dor":
		return "dorian"
	case "mix":
		return "mixolydian"
	case "lyd":
		return "lydian"
	case "phr":
		return "phrygian"
	case "loc":
		return "locrian"
	}
	return "major"
}

// ParseABCKey reads the value of a K: field ("G", "Em", "A dorian",
// "F#m clef=treble"). "none", Highland pipes ("HP") and anything without a
// tonic give false.
func ParseABCKey(v string) (Key, bool) {
	v = strings.TrimSpace(stripComment(v))
	if len(v) == 0 || v[0] < 'A' || v[0] > 'G' {
		return Key{}, false
	}
	k := Key{Tonic: v[:1]}
	rest := v[1:]
	if rest != "" && (rest[0] == '#' || rest[0] == 'b') {
		k.Tonic += rest[:1]
		rest = rest[1:]
	}
	rest = strings.TrimLeft(rest, " \t")
	word := rest
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		word = rest[:i]
	}
	if strings.Contains(word, "=") || strings.HasPrefix(word, "^") || strings.HasPrefix(word, "_") {
		word = ""
	}
	k.Mode = modeOf(word)
	return k, true
}

var meterRe = regexp.MustCompile(`^([1-9][0-9]?)/(1|2|4|8|16|32)$`)

// ValidMeter reports whether m is a Time Signature the CMS keeps: n/d with
// n from 1 to 32 and d a power of two up to 32.
func ValidMeter(m string) bool {
	g := meterRe.FindStringSubmatch(m)
	if g == nil {
		return false
	}
	n, _ := strconv.Atoi(g[1])
	return n <= 32
}

// ParseABCMeter reads the value of an M: field: "6/8", "C" (4/4), "C|"
// (2/2). "none" and compound forms ("2+3/8") give false.
func ParseABCMeter(v string) (string, bool) {
	v = strings.ReplaceAll(strings.TrimSpace(stripComment(v)), " ", "")
	switch v {
	case "C":
		return "4/4", true
	case "C|":
		return "2/2", true
	}
	return v, ValidMeter(v)
}

// stripComment cuts a % comment off a field value (\% is a literal %).
func stripComment(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '%' {
			return s[:i]
		}
	}
	return s
}

// ABCHeader holds the header fields of one tune, up to and including its
// first K: line.
type ABCHeader struct {
	Titles   []string `json:"titles"`
	Rhythm   string   `json:"rhythm,omitempty"`
	Origin   string   `json:"origin,omitempty"`
	Composer string   `json:"composer,omitempty"`
	Key      string   `json:"key,omitempty"`
	Meter    string   `json:"meter,omitempty"`
	Unit     string   `json:"unit,omitempty"`
}

var fieldRe = regexp.MustCompile(`^([A-Za-z]):(.*)$`)

// ParseABCHeader reads the header of one tune.
func ParseABCHeader(abc string) ABCHeader {
	var h ABCHeader
	for _, line := range strings.Split(strings.ReplaceAll(abc, "\r", ""), "\n") {
		m := fieldRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		v := strings.TrimSpace(stripComment(m[2]))
		switch m[1] {
		case "T":
			if v != "" {
				h.Titles = append(h.Titles, v)
			}
		case "R":
			h.Rhythm = firstNonEmpty(h.Rhythm, v)
		case "O":
			h.Origin = firstNonEmpty(h.Origin, v)
		case "C":
			h.Composer = firstNonEmpty(h.Composer, v)
		case "M":
			h.Meter = firstNonEmpty(h.Meter, v)
		case "L":
			h.Unit = firstNonEmpty(h.Unit, v)
		case "K":
			h.Key = v
			return h
		}
	}
	return h
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ABCProposal is what a Transcription proposes for its Tune: the Key from
// its first K: and the Time Signature from M: (empty when it has none the
// CMS can read).
type ABCProposal struct {
	Key   *Key   `json:"key,omitempty"`
	Meter string `json:"meter,omitempty"`
}

// ProposeFromABC reads the Key and Time Signature a Transcription proposes.
func ProposeFromABC(abc string) ABCProposal {
	h := ParseABCHeader(abc)
	var p ABCProposal
	if k, ok := ParseABCKey(h.Key); ok {
		p.Key = &k
	}
	if m, ok := ParseABCMeter(h.Meter); ok {
		p.Meter = m
	}
	return p
}

// SplitABCTunes splits a multi-tune ABC file at its X: lines; text before
// the first X: (a file header) is dropped. A text without X: is one tune.
func SplitABCTunes(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var out []string
	var cur []string
	started := false
	flush := func() {
		if t := strings.TrimSpace(strings.Join(cur, "\n")); t != "" {
			out = append(out, t)
		}
		cur = nil
	}
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimLeft(l, " \t"), "X:") {
			if started {
				flush()
			}
			started = true
			cur = nil
		}
		if started {
			cur = append(cur, l)
		}
	}
	if !started {
		cur = lines
	}
	flush()
	return out
}

// --- Incipit ------------------------------------------------------------------

// Incipit bar counts.
const (
	DefaultIncipitBars = 2
	MaxIncipitBars     = 8
)

// rat is a small non-negative rational (a duration in whole notes). The
// zero value (d == 0) reads as 0.
type rat struct{ n, d int }

// norm makes the zero value a proper 0/1, so arithmetic on it works.
func (a rat) norm() rat {
	if a.d == 0 {
		return rat{0, 1}
	}
	return a
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func mk(n, d int) rat {
	if d == 0 {
		return rat{0, 1}
	}
	g := gcd(n, d)
	if g == 0 {
		return rat{0, 1}
	}
	return rat{n / g, d / g}
}

func (a rat) add(b rat) rat {
	a, b = a.norm(), b.norm()
	return mk(a.n*b.d+b.n*a.d, a.d*b.d)
}
func (a rat) mul(b rat) rat { a, b = a.norm(), b.norm(); return mk(a.n*b.n, a.d*b.d) }
func (a rat) less(b rat) bool {
	a, b = a.norm(), b.norm()
	return a.n*b.d < b.n*a.d
}
func (a rat) zero() bool { return a.n == 0 }

// parseFraction reads "n/d" (an L: value or a meter); ok false otherwise.
func parseFraction(s string) (rat, bool) {
	a, b, found := strings.Cut(strings.TrimSpace(s), "/")
	if !found {
		return rat{}, false
	}
	n, err1 := strconv.Atoi(strings.TrimSpace(a))
	d, err2 := strconv.Atoi(strings.TrimSpace(b))
	if err1 != nil || err2 != nil || n <= 0 || d <= 0 || n > 64 || d > 64 {
		return rat{}, false
	}
	return mk(n, d), true
}

// abcState is what the bar counter needs to know at a point of the tune.
type abcState struct {
	meter    rat  // length of a full bar; zero when free (M:none)
	unit     rat  // L:
	unitSet  bool // L: given explicitly
	compound bool // meter numerator divisible by 3 (above 3): tuplet defaults
}

func (s *abcState) setMeter(v string) {
	m, ok := ParseABCMeter(v)
	if !ok {
		s.meter = rat{}
		s.compound = false
	} else {
		s.meter, _ = parseFraction(m)
		n, _ := strconv.Atoi(strings.Split(m, "/")[0])
		s.compound = n%3 == 0 && n > 3
	}
	if !s.unitSet {
		// The default unit length follows the meter (standard 2.1, 3.1.7).
		if !s.meter.zero() && s.meter.less(mk(3, 4)) {
			s.unit = mk(1, 16)
		} else {
			s.unit = mk(1, 8)
		}
	}
}

func (s *abcState) setUnit(v string) {
	if u, ok := parseFraction(stripComment(v)); ok {
		s.unit, s.unitSet = u, true
	}
}

// tuneParts splits one tune into its header lines (up to and including the
// first K:) and its body lines.
func tuneParts(abc string) (header, body []string) {
	lines := strings.Split(strings.ReplaceAll(abc, "\r", ""), "\n")
	for i, l := range lines {
		header = append(header, l)
		if m := fieldRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil && m[1] == "K" {
			return header, lines[i+1:]
		}
	}
	return header, nil
}

// Incipit cuts a Transcription after its pickup and first bars (1 to
// MaxIncipitBars; 0 means DefaultIncipitBars) and returns it as a small ABC
// tune: X:, M:, L: and K: of the header and the opening of the body, ended
// with a final bar line. A short first bar (shorter than the meter) is a
// pickup and does not count. ok is false when the text has no K: or no
// notes.
func Incipit(abc string, bars int) (string, bool) {
	if bars <= 0 {
		bars = DefaultIncipitBars
	}
	bars = min(bars, MaxIncipitBars)
	header, body := tuneParts(abc)
	if body == nil {
		return "", false
	}
	st := abcState{}
	st.setMeter("4/4")
	var meterLine, unitLine, keyLine string
	for _, l := range header {
		m := fieldRe.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		switch m[1] {
		case "M":
			meterLine = "M:" + strings.TrimSpace(stripComment(m[2]))
			st.setMeter(m[2])
		case "L":
			unitLine = "L:" + strings.TrimSpace(stripComment(m[2]))
			st.setUnit(m[2])
		case "K":
			keyLine = "K:" + strings.TrimSpace(stripComment(m[2]))
		}
	}
	c := &barCounter{st: st, want: bars}
	var out strings.Builder
lines:
	for _, line := range body {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "%") {
			continue
		}
		if m := fieldRe.FindStringSubmatch(trim); m != nil {
			switch m[1] {
			case "M":
				if c.meterChange(m[2]) {
					music := strings.TrimRight(out.String(), " \t\n\\")
					out.Reset()
					out.WriteString(music + "|]")
					break lines
				}
				out.WriteString("M:" + strings.TrimSpace(stripComment(m[2])) + "\n")
			case "L":
				c.st.setUnit(m[2])
				out.WriteString("L:" + strings.TrimSpace(stripComment(m[2])) + "\n")
			case "K":
				out.WriteString("K:" + strings.TrimSpace(stripComment(m[2])) + "\n")
			}
			continue // lyrics (w:), parts (P:) and the rest are left out
		}
		kept, done := c.line(line)
		out.WriteString(strings.TrimRight(kept, " \t\\"))
		if done {
			break
		}
		out.WriteString("\n")
	}
	// A tune that ends without a bar line, or with its pickup, still has
	// an opening.
	if c.counted == 0 && !c.done && !c.notes {
		return "", false
	}
	music := strings.TrimSpace(out.String())
	if !c.done {
		music = strings.TrimRight(music, " |:]") + "|]"
	}
	head := []string{"X:1"}
	for _, l := range []string{meterLine, unitLine, keyLine} {
		if l != "" {
			head = append(head, l)
		}
	}
	return strings.Join(head, "\n") + "\n" + music + "\n", true
}

// barCounter walks the music of a tune, bar by bar.
type barCounter struct {
	st      abcState
	want    int
	bar     rat // duration of the bar so far
	counted int // full bars counted
	first   bool
	done    bool
	notes   bool // a note, rest or chord was read
	// tuplet in progress: notes left and the factor.
	tupLeft int
	tupF    rat
}

var (
	noteRe   = regexp.MustCompile(`^[\^=_]*[A-Ga-gzxZX][,']*(\d*)(/*)(\d*)`)
	lengthRe = regexp.MustCompile(`^(\d*)(/*)(\d*)`)
	tupletRe = regexp.MustCompile(`^\(([2-9])(?::([1-9]?))?(?::([1-9]?))?`)
)

// length reads a note length suffix ("2", "/", "3/2", "//") into a factor.
func length(num, slashes, den string) rat {
	n := 1
	if num != "" {
		n, _ = strconv.Atoi(num)
	}
	d := 1
	switch {
	case den != "":
		d, _ = strconv.Atoi(den)
		for range max(0, len(slashes)-1) {
			d *= 2
		}
	case slashes != "":
		d = 1 << min(len(slashes), 6)
	}
	if n <= 0 || n > 64 || d <= 0 || d > 64 {
		return mk(1, 1)
	}
	return mk(n, d)
}

// note adds one note, rest or chord of length f (in units).
func (c *barCounter) note(f rat) {
	c.notes = true
	d := c.st.unit.mul(f)
	if c.tupLeft > 0 {
		d = d.mul(c.tupF)
		c.tupLeft--
	}
	c.bar = c.bar.add(d)
}

// barLine closes the current bar; it reports whether the incipit is complete.
func (c *barCounter) barLine() bool {
	if c.bar.zero() {
		return false
	}
	pickup := !c.first && !c.st.meter.zero() && c.bar.less(c.st.meter)
	c.first = true
	c.bar = rat{0, 1}
	if pickup {
		return false
	}
	c.counted++
	return c.counted >= c.want
}

// meterChange applies a new meter. A meter change mid-music closes a bar
// that is already full (a line written without its last bar line); it
// reports whether that completed the incipit.
func (c *barCounter) meterChange(v string) bool {
	full := !c.bar.zero() && !c.st.meter.zero() && !c.bar.less(c.st.meter)
	if full && c.barLine() {
		c.done = true
		return true
	}
	c.st.setMeter(v)
	return false
}

// line consumes one music line and returns the text to keep; done is true
// when the incipit ended in it (the kept text then ends with "|]").
func (c *barCounter) line(s string) (string, bool) {
	var out strings.Builder
	i := 0
	for i < len(s) {
		ch := s[i]
		switch {
		case ch == '%':
			return out.String(), false
		case ch == '"', ch == '!', ch == '+':
			j := strings.IndexByte(s[i+1:], ch)
			if j < 0 {
				out.WriteString(s[i:])
				return out.String(), false
			}
			out.WriteString(s[i : i+j+2])
			i += j + 2
		case ch == '{':
			j := strings.IndexByte(s[i:], '}')
			if j < 0 {
				return out.String(), false
			}
			out.WriteString(s[i : i+j+1])
			i += j + 1
		case ch == '[' && i+2 < len(s) && isLetter(s[i+1]) && s[i+2] == ':':
			// Inline field [M:6/8], [L:1/16], [K:D].
			j := strings.IndexByte(s[i:], ']')
			if j < 0 {
				return out.String(), false
			}
			field := s[i+1 : i+j]
			switch field[0] {
			case 'M':
				if c.meterChange(field[2:]) {
					out.WriteString("|]")
					return out.String(), true
				}
			case 'L':
				c.st.setUnit(field[2:])
			}
			out.WriteString(s[i : i+j+1])
			i += j + 1
		case ch == '[' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9':
			// A first or second ending: [1, [2.
			j := i + 1
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == ',' || s[j] == '-') {
				j++
			}
			out.WriteString(s[i:j])
			i = j
		case ch == '|' || ch == ':' || ch == '[' && i+1 < len(s) && s[i+1] == '|':
			j := i
			for j < len(s) && (s[j] == '|' || s[j] == ':' || s[j] == ']' || s[j] == '[') {
				// "[" only opens a bar line ("[|"); a chord after a bar line stops it.
				if s[j] == '[' && (j+1 >= len(s) || s[j+1] != '|') {
					break
				}
				if s[j] == ']' && j == i {
					break
				}
				j++
			}
			if j == i {
				j = i + 1
			}
			// Ending numbers right after the bar line: |1, :|2.
			k := j
			for k < len(s) && (s[k] >= '0' && s[k] <= '9' || s[k] == ',' || s[k] == '-' && k > j) {
				k++
			}
			if c.barLine() {
				c.done = true
				out.WriteString("|]")
				return out.String(), true
			}
			out.WriteString(s[i:k])
			i = k
		case ch == '[':
			// A chord: its length is its first note's, times the suffix.
			j := strings.IndexByte(s[i:], ']')
			if j < 0 {
				return out.String(), false
			}
			inner := s[i+1 : i+j]
			f := mk(1, 1)
			if m := noteRe.FindStringSubmatch(strings.TrimLeft(inner, "!.~")); m != nil {
				f = length(m[1], m[2], m[3])
			}
			k := i + j + 1
			m := lengthRe.FindStringSubmatch(s[k:])
			k += len(m[0])
			if m[0] != "" {
				f = f.mul(length(m[1], m[2], m[3]))
			}
			c.note(f)
			out.WriteString(s[i:k])
			i = k
		case ch == '(' && i+1 < len(s) && s[i+1] >= '2' && s[i+1] <= '9':
			m := tupletRe.FindStringSubmatch(s[i:])
			p, _ := strconv.Atoi(m[1])
			q := map[int]int{2: 3, 3: 2, 4: 3, 6: 2, 8: 3}[p]
			if q == 0 {
				q = 2
				if c.st.compound {
					q = 3
				}
			}
			if m[2] != "" {
				q, _ = strconv.Atoi(m[2])
			}
			r := p
			if m[3] != "" {
				r, _ = strconv.Atoi(m[3])
			}
			c.tupLeft, c.tupF = r, mk(q, p)
			out.WriteString(m[0])
			i += len(m[0])
		case ch == 'Z' || ch == 'X':
			// A multi-bar rest counts as full bars.
			m := noteRe.FindStringSubmatch(s[i:])
			n := 1
			if m[1] != "" {
				n, _ = strconv.Atoi(m[1])
			}
			for range max(1, n) {
				c.bar = c.st.meter
				if c.st.meter.zero() {
					c.bar = mk(1, 1)
				}
				c.first = true
				if c.barLine() {
					c.done = true
					out.WriteString(m[0] + "|]")
					return out.String(), true
				}
			}
			c.bar = rat{0, 1}
			out.WriteString(m[0])
			i += len(m[0])
		default:
			if m := noteRe.FindStringSubmatch(s[i:]); m != nil && m[0] != "" {
				c.note(length(m[1], m[2], m[3]))
				out.WriteString(m[0])
				i += len(m[0])
				continue
			}
			// Spaces, ties, slurs, broken rhythm (> and < keep a pair's
			// length), decoration letters, spacers and the rest.
			out.WriteByte(ch)
			i++
		}
	}
	return out.String(), false
}

func isLetter(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' }

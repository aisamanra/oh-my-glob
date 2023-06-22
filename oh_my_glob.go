package oh_my_glob

import (
	"log"
	"strings"
)

// "enum" for globs.
const (
	generalParts = iota
	// **/*.suffix
	recursiveWildcardSuffix = iota
	// **/filename
	recursiveWildcardFixedFile = iota
	// path/to/dir/*.suffix
	fixedPathWildcardSuffix = iota
)

// "enum" for parts, below.
const (
	// Something that is not a literal `*` or `**`, but makes no other
	// promises about its contents.
	literal    = iota
	singleStar = iota
	doubleStar = iota
)

type part struct {
	kind int8
	// only set if kind == literal
	no_stars bool
	// only set if kind == literal
	lit string
}

var starstar part = part{
	kind: doubleStar,
	lit:  "",
}

var star part = part{
	kind: singleStar,
	lit:  "",
}

type Glob struct {
	// this is for pretty-printing the glob
	original string
	kind     int8
	// we pre-break the glob at `/` boundaries for less processing
	// later
	parts []part
}

func (g *Glob) Original() string {
	return g.original
}

func (p *part) isWildcardSuffix() bool {
	if p.kind != literal {
		return false
	}

	if p.no_stars {
		return false
	}

	// p has a star, and if the tail of p.lit does not have a
	// star, then the star must have been at the beginning.
	return strings.IndexByte(p.lit[1:], '*') == -1
}

func allLiteralsNoWildcards(parts []part) bool {
	for _, p := range parts {
		if p.kind != literal {
			return false
		}
		if !p.no_stars {
			return false
		}
	}

	return true
}

func buildDirectoryPrefixPart(parts []part) part {
	// This is basically strings.Join(..., "/") + "/", but it's not quite
	// that simple, because we don't have an array of strings to pass to
	// Join.
	builder := strings.Builder{}
	capacity := 0
	for _, p := range parts {
		capacity += len(p.lit)
		capacity++ // for the slash
	}
	builder.Grow(capacity)

	for _, p := range parts {
		builder.WriteString(p.lit)
		builder.WriteString("/")
	}

	return part{
		kind:     literal,
		lit:      builder.String(),
		no_stars: true,
	}
}

// Do the parts represent **/*.suffix with a (possibly) non-empty prefix of
// literal, no-wildcard parts?
func recursiveWildcardSuffixPattern(parts []part) (bool, []part) {
	if parts[len(parts)-2].kind != doubleStar {
		return false, nil
	}

	if !parts[len(parts)-1].isWildcardSuffix() {
		return false, nil
	}

	prefixSlice := parts[:len(parts)-2]
	if !allLiteralsNoWildcards(prefixSlice) {
		return false, nil
	}

	return true, prefixSlice
}

// Do the parts represent **/filename with a (possibly) non-empty prefix of
// literal, no-wildcard parts?
func recursiveWildcardFixedFilePattern(parts []part) (bool, []part) {
	if parts[len(parts)-2].kind != doubleStar {
		return false, nil
	}

	p := parts[len(parts)-1]
	if p.kind != literal {
		return false, nil
	}
	if !p.no_stars {
		return false, nil
	}

	prefixSlice := parts[:len(parts)-2]
	if !allLiteralsNoWildcards(prefixSlice) {
		return false, nil
	}

	return true, prefixSlice
}

func Compile(glob string) Glob {
	if glob == "" {
		return Glob{
			original: "",
			parts:    nil,
		}
	}

	fragments := strings.Split(glob, "/")
	parts := make([]part, 0, len(fragments))
	for _, fragment := range fragments {
		if fragment == "**" {
			parts = append(parts, starstar)
		} else if fragment == "*" {
			parts = append(parts, star)
		} else {
			parts = append(parts, part{
				kind:     literal,
				lit:      fragment,
				no_stars: strings.IndexByte(fragment, '*') == -1,
			})
		}
	}

	if len(parts) >= 2 {
		if isPattern, prefixSlice := recursiveWildcardSuffixPattern(parts); isPattern {
			length := 1
			// Build the directory prefix before we overwrite things.
			if len(prefixSlice) != 0 {
				parts[1] = buildDirectoryPrefixPart(prefixSlice)
				length = 2
			}
			parts[0] = part{
				kind:     literal,
				lit:      parts[len(parts)-1].lit[1:],
				no_stars: true,
			}
			parts = parts[:length]
			return Glob{
				original: glob,
				kind:     recursiveWildcardSuffix,
				parts:    parts,
			}
		}

		if isPattern, prefixSlice := recursiveWildcardFixedFilePattern(parts); isPattern {
			length := 1
			// Build the directory prefix before we overwrite things.
			if len(prefixSlice) != 0 {
				parts[1] = buildDirectoryPrefixPart(prefixSlice)
				length = 2
			}
			parts[0] = parts[len(parts)-1]
			parts = parts[:length]
			return Glob{
				original: glob,
				kind:     recursiveWildcardFixedFile,
				parts:    parts,
			}
		}

		// Special-case path/to/dir/*suffix
		if parts[len(parts)-1].isWildcardSuffix() {
			prefixSlice := parts[:len(parts)-1]
			if allLiteralsNoWildcards(prefixSlice) {
				parts[0] = buildDirectoryPrefixPart(prefixSlice)
				parts[1] = part{
					kind:     literal,
					lit:      parts[len(parts)-1].lit[1:],
					no_stars: true,
				}
				parts = parts[:2]
				return Glob{
					original: glob,
					kind:     fixedPathWildcardSuffix,
					parts:    parts,
				}
			}
		}
	}

	return Glob{
		original: glob,
		kind:     generalParts,
		parts:    parts,
	}
}

// this code is borrowed directly from Russ Cox's research page,
// albeit with the single-character wildcard removed (since I don't
// believe we need it.) For more details:
//
//	https://research.swtch.com/glob
func match(pattern, name string) bool {
	px := 0
	nx := 0
	nextPx := 0
	nextNx := 0
	for px < len(pattern) || nx < len(name) {
		if px < len(pattern) {
			c := pattern[px]
			switch c {
			default:
				if nx < len(name) && name[nx] == c {
					px++
					nx++
					continue
				}

			case '*':
				// zero-or-more-character wildcard
				// Try to match at nx.
				// If that doesn't work out,
				// restart at nx+1 next.
				nextPx = px
				nextNx = nx + 1
				px++
				continue
			}
		}
		// Mismatch. Maybe restart.
		if 0 < nextNx && nextNx <= len(name) {
			px = nextPx
			nx = nextNx
			continue
		}
		return false
	}
	// Matched all of pattern to all of name. Success.
	return true
}

func (g *Glob) matchGeneral(path string) bool {
	// `px` is the index into the current path part
	px := 0
	// `nx` is the index into the string, which will change in
	// strides based on where we find `/` characters
	nx := 0
	// these are used for backtracking in the case of `**`,
	// c.f. the Russ Cox page linked above
	nextPx := 0
	nextNx := 0

	for px < len(g.parts) || nx < len(path) {
		// This is a little hairy: `incrNx` is going to be the
		// "next" `nx` value, and we'll only ever need it if
		// `nx < len(path)` above. `nx` should always point to
		// the beginning of a path segment _or_ to the end of
		// the path. We start searching from the current `nx`
		// and find the next '/' character
		incrNx := 0
		if nx < len(path) {
			tx := strings.IndexByte(path[nx:], '/')

			if tx < 0 {
				incrNx = len(path)
			} else {
				incrNx = nx + tx + 1
			}
		}

		if px < len(g.parts) {
			c := g.parts[px]
			switch c.kind {
			case literal:
				if nx < len(path) {
					// find the next substring
					var chunk string
					if incrNx == len(path) {
						chunk = path[nx:]
					} else {
						chunk = path[nx : incrNx-1]
					}

					if c.no_stars && c.lit == chunk {
						px++
						nx = incrNx
						continue
					} else if match(c.lit, chunk) {
						px++
						nx = incrNx
						continue
					}
				}
			case doubleStar:
				nextPx = px
				nextNx = incrNx
				px++
				continue
			case singleStar:
				if nx < len(path) {
					px++
					nx = incrNx
					continue
				}
			default:
				// this should never happen and
				// indicates a bug in library code
				log.Fatalf("Unexpected compiled glob value")
			}
		}

		if 0 < nextNx && nextNx <= len(path) {
			px = nextPx
			nx = nextNx
			continue
		}
		return false
	}
	return true
}

func (g *Glob) matchRecursiveWildcardSuffix(path string) bool {
	if !strings.HasSuffix(path, g.parts[0].lit) {
		return false
	}

	if len(g.parts) == 2 {
		if !strings.HasPrefix(path, g.parts[1].lit) {
			return false
		}
	}

	return true
}

func (g *Glob) matchRecursiveWildcardFixedFile(path string) bool {
	// If we don't have a prefix, we need to consider two cases:
	//
	// 1. path/to/dir/filename
	// 2. filename
	if !strings.HasSuffix(path, g.parts[0].lit) {
		return false
	}

	// We have a prefix, so the prefix needs to match.
	if len(g.parts) == 2 {
		if !strings.HasPrefix(path, g.parts[1].lit) {
			return false;
		}
	}

	if len(path) == len(g.parts[0].lit) {
		return true
	}

	return path[len(path)-len(g.parts[0].lit)-1] == '/'
}

func (g *Glob) matchFixedPathWildcardSuffix(path string) bool {
	// We test for the suffix first on the theory that matching the
	// suffix is a better fast-reject than matching the fixed path.
	if !strings.HasSuffix(path, g.parts[1].lit) {
		return false
	}

	if !strings.HasPrefix(path, g.parts[0].lit) {
		return false
	}

	// We have path/to/dir/<unknown>suffix.  Make sure that <unknown>
	// doesn't contain directory separators.
	return strings.IndexByte(path[len(g.parts[0].lit):], '/') == -1
}

func (g *Glob) Match(path string) bool {
	switch g.kind {
	case generalParts:
		return g.matchGeneral(path)
	case recursiveWildcardSuffix:
		return g.matchRecursiveWildcardSuffix(path)
	case recursiveWildcardFixedFile:
		return g.matchRecursiveWildcardFixedFile(path)
	case fixedPathWildcardSuffix:
		return g.matchFixedPathWildcardSuffix(path)
	default:
		log.Fatalf("Unexpected compiled glob kind")
		return false
	}
}

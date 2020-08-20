package wildcard

// Replacer represents wildcard replacer like strings.Replacer
type Replacer struct {
	oldnew []string
}

// NewReplacer creates a Replacer.
func NewReplacer(oldnew ...string) *Replacer {
	if len(oldnew)%2 != 0 {
		panic("wildcard.NewReplacer: odd argument count")
	}
	return &Replacer{
		oldnew: oldnew,
	}
}

// Replace replaces a string
func (r *Replacer) Replace(s string) string {
	for i := 0; i < len(r.oldnew)-1; i += 2 {
		if Match(r.oldnew[i], s) {
			return r.oldnew[i+1]
		}
	}
	return s
}

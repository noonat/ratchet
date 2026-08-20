package patch

import "strings"

// Repairs are the fixes a tool may apply to a reply before using it. A repair uses
// information the tool already holds, which is what separates one from the
// similarity matching this pipeline excludes: aider shipped an edit-distance
// matcher at a threshold of 0.8 and disabled it with a bare `return`, because an
// edit applied to code that merely resembles the target corrupts the file in a way
// review does not catch.
//
// There is one repair, and it runs when a patch is applied rather than when it is
// parsed, because it needs the line being replaced and only the applier has read
// the file.
//
// A second repair was measured and is deliberately absent. Filling in a missing
// sigil took correct body rows from 64% to 99%, but it was measured on a form whose
// body rows are all additions, where a bare row can only mean one thing. This
// parser reads a form with an old row and a new row, so a body of bare rows does
// not say which is which, and inferring it would be the guess the check exists to
// prevent. A corrective turn covers the same failure at a price that is known:
// measured, it recovers 221 of 494 diagnosed failures and turns 35 into wrong ones,
// so a refusal is worth about 45% of a correct edit. The repair's price is not
// known, because guessing which row is which lands silently.
type Repairs struct {
	// Reindent takes the indentation of the line being replaced when a single-line
	// replacement arrives without any.
	//
	// Unsound where whitespace carries meaning. In Python it cannot express moving
	// a line out of a block, and applying it there would silently refuse to make an
	// edit the model asked for correctly. IndentSensitive gates it.
	Reindent bool
}

// IndentSensitive reports whether a language's indentation is syntax, in which case
// the re-indent repair must not run.
//
// A list rather than a guess: getting this wrong in the permissive direction breaks
// Python silently, and in the restrictive direction only forgoes a repair.
func IndentSensitive(lang string) bool {
	switch strings.ToLower(strings.TrimPrefix(lang, ".")) {
	case "py", "python", "pyi", "yaml", "yml", "nim", "haml", "sass", "coffee",
		// Layout and offside rules: indentation decides which block a line is in.
		"hs", "lhs", "fs", "fsx", "fsi",
		// Nested list depth and code-block membership are indentation.
		"md", "markdown",
		// Indented template languages.
		"pug", "jade", "slim", "styl", "stylus", "cson":
		return true
	}
	return false
}

// Reindent gives a single-line replacement the indentation of the line it replaces.
//
// It refuses in two cases: when the language's indentation is syntax, and when the
// replacement already carries indentation of its own. The second matters because a
// model that indented deliberately is expressing an edit, and overwriting it would
// discard the one thing the model was asked for.
func Reindent(lang, original, replacement string) (string, bool) {
	if IndentSensitive(lang) {
		return replacement, false
	}
	if replacement == "" || replacement != strings.TrimLeft(replacement, " \t") {
		return replacement, false
	}
	indent := original[:len(original)-len(strings.TrimLeft(original, " \t"))]
	if indent == "" {
		return replacement, false
	}
	return indent + replacement, true
}

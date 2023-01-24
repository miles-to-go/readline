package completion

// Completer is a function generating completions.
// This is generally used so that a given completer function
// (history, registers, etc) can be cached and reused by the engine.
type Completer func() Values

// Candidate represents a completion candidate.
type Candidate struct {
	Value       string // Value is the value of the completion as actually inserted in the line
	Display     string // When display is not nil, this string is used to display the completion in the menu.
	Description string // A description to display next to the completion candidate.
	Style       string // An arbitrary string of color/text effects to use when displaying the completion.
	Tag         string // All completions with the same tag are grouped together and displayed under the tag heading.

	// A list of runes that are automatically trimmed when a space or a non-nil character is
	// inserted immediately after the completion. This is used for slash-autoremoval in path
	// completions, comma-separated completions, etc.
	noSpace suffixMatcher
}

// Values is used internally to hold all completion candidates and their associated data.
type Values struct {
	values   rawValues
	messages messages
	noSpace  suffixMatcher
	usage    string
	listLong map[string]bool
	noSort   map[string]bool
	listSep  map[string]string

	// Initially this will be set to the part of the current word
	// from the beginning of the word up to the position of the cursor;
	// it may be altered to give a common prefix for all matches.
	PREFIX string
}

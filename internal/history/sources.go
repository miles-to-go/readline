package history

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/reeflective/readline/inputrc"
	"github.com/reeflective/readline/internal/color"
	"github.com/reeflective/readline/internal/completion"
	"github.com/reeflective/readline/internal/core"
	"github.com/reeflective/readline/internal/ui"
)

// Sources manages and serves all history sources for the current shell.
type Sources struct {
	// History sources
	list       map[string]Source // Sources of history lines
	names      []string          // Names of histories stored in rl.histories
	maxEntries int               // Inputrc configured maximum number of entries.
	sourcePos  int               // The index of the currently used history
	buf        string            // The current line saved when we are on another history line
	hpos       int               // Index used for navigating the history lines with arrows/j/k
	infer      bool              // If the last command ran needs to infer the history line.
	accepted   bool              // The line has been accepted and must be returned.
	acceptHold bool              // Should we reuse the same accepted line on the next loop.
	acceptLine core.Line         // The line to return to the caller.
	acceptErr  error             // An error to return to the caller.

	// Line history
	skip    bool
	undoing bool
	last    inputrc.Bind
	lines   map[string]map[int]*lineHistory

	// Shell parameters
	line   *core.Line
	cursor *core.Cursor
	cpos   int
	hint   *ui.Hint
	opts   *inputrc.Config
}

// NewSources is a required constructor for the history sources manager type.
func NewSources(line *core.Line, cur *core.Cursor, hint *ui.Hint, opts *inputrc.Config) *Sources {
	sources := &Sources{
		// History sourcces
		list: make(map[string]Source),
		// Line history
		lines: make(map[string]map[int]*lineHistory),
		// Shell parameters
		line:   line,
		cursor: cur,
		cpos:   -1,
		hint:   hint,
		opts:   opts,
	}

	sources.names = append(sources.names, defaultSourceName)
	sources.list[defaultSourceName] = new(memory)

	// Inputrc settings.
	sources.maxEntries = opts.GetInt("history-size")
	sizeSet := opts.GetString("history-size") != ""

	if sources.maxEntries == 0 && !sizeSet {
		sources.maxEntries = -1
	} else if sources.maxEntries == 0 && sizeSet {
		sources.maxEntries = 500
	}

	return sources
}

// Init initializes the history sources positions and buffers
// at the start of each readline loop. If the last command asked
// to infer a command line from the history, it is performed now.
func (h *Sources) Init() {
	defer func() {
		h.sourcePos = 0
		h.accepted = false
		h.acceptLine = nil
		h.acceptErr = nil
		h.cpos = -1
	}()

	if h.acceptHold {
		h.hpos = 0
		h.line.Set(h.acceptLine...)
		h.cursor.Set(h.line.Len())

		return
	}

	if !h.infer {
		h.hpos = 0
		return
	}

	switch h.hpos {
	case -1:
		h.hpos = 0
	case 0:
		h.InferNext()
	default:
		h.Walk(-1)
	}

	h.infer = false
}

// Add adds a source of history lines bound to a given name (printed above
// this source when used). When only the default in-memory history is bound,
// it's replaced with the provided source. Following ones are added to the list.
func (h *Sources) Add(name string, hist Source) {
	if len(h.list) == 1 && h.names[0] == defaultSourceName {
		delete(h.list, defaultSourceName)
		h.names = make([]string, 0)
	}

	h.names = append(h.names, name)
	h.list[name] = hist
}

// New creates a new History populated from, and writing to a file.
func (h *Sources) AddFromFile(name, file string) {
	hist := new(fileHistory)
	hist.file = file
	hist.lines, _ = openHist(file)

	h.Add(name, hist)
}

// Delete deletes one or more history source by name.
// If no arguments are passed, all currently bound sources are removed.
func (h *Sources) Delete(sources ...string) {
	if len(sources) == 0 {
		h.list = make(map[string]Source)
		h.names = make([]string, 0)

		return
	}

	for _, name := range sources {
		delete(h.list, name)

		for i, hname := range h.names {
			if hname == name {
				h.names = append(h.names[:i], h.names[i+1:]...)
				break
			}
		}
	}

	h.sourcePos = 0
	if !h.infer {
		h.hpos = 0
	}
}

// Walk goes to the next or previous history line in the active source.
// If at the end of the history, the last history line is kept.
// If going back to the beginning of it, the saved line buffer is restored.
func (h *Sources) Walk(pos int) {
	history := h.Current()

	if history == nil || history.Len() == 0 {
		return
	}

	// When we are on the last/first item, don't do anything,
	// as it would change things like cursor positions.
	if (pos < 0 && h.hpos == 0) || (pos > 0 && h.hpos == history.Len()) {
		return
	}

	// Save the current line buffer if we are leaving it.
	if h.hpos == 0 && (h.hpos+pos) == 1 {
		h.buf = string(*h.line)
	}

	h.hpos += pos

	switch {
	case h.hpos > history.Len():
		h.hpos = history.Len()
	case h.hpos < 0:
		h.hpos = 0
	case h.hpos == 0:
		h.line.Set([]rune(h.buf)...)
		h.cursor.Set(h.line.Len())
	}

	if h.hpos == 0 {
		return
	}

	var line string
	var err error

	// When there is an available change history for
	// this line, use it instead of the fetched line.
	if hist := h.getLineHistory(); hist != nil && len(hist.items) > 0 {
		line = hist.items[len(hist.items)-1].line
	} else if line, err = history.GetLine(history.Len() - h.hpos); err != nil {
		h.hint.Set(color.FgRed + "history error: " + err.Error())
		return
	}

	// Update line buffer and cursor position.
	h.setLineCursorMatch(line)
}

// Fetch fetches the history event at the provided
// index position and makes it the current buffer.
func (h *Sources) Fetch(pos int) {
	history := h.Current()

	if history == nil || history.Len() == 0 {
		return
	}

	if pos < 0 || pos >= history.Len() {
		return
	}

	line, err := history.GetLine(pos)
	if err != nil {
		h.hint.Set(color.FgRed + "history error: " + err.Error())
		return
	}

	h.line.Set([]rune(line)...)
	h.cursor.Set(h.line.Len())
}

// GetLast returns the last saved history line in the active history source.
func (h *Sources) GetLast() string {
	history := h.Current()

	if history == nil || history.Len() == 0 {
		return ""
	}

	last, err := history.GetLine(history.Len() - 1)
	if err != nil {
		return ""
	}

	return last
}

// Cycle checks for the next history source (if any) and makes it the active one.
// If next is false, the source cycles to the previous source.
func (h *Sources) Cycle(next bool) {
	switch next {
	case true:
		h.sourcePos++

		if h.sourcePos == len(h.names) {
			h.sourcePos = 0
		}
	case false:
		h.sourcePos--

		if h.sourcePos < 0 {
			h.sourcePos = len(h.names) - 1
		}
	}
}

// OnLastSource returns true if the currently active
// history source is the last one in the list.
func (h *Sources) OnLastSource() bool {
	return h.sourcePos == len(h.names)-1
}

// Current returns the current/active history source.
func (h *Sources) Current() Source {
	if len(h.list) == 0 {
		return nil
	}

	return h.list[h.names[h.sourcePos]]
}

// Write writes the accepted input line to all available sources.
// If infer is true, the next history initialization will automatically
// insert the next history line found after the first match of the line
// that has just been written (thus, normally, accepted/executed).
func (h *Sources) Write(infer bool) {
	if infer {
		h.infer = true
		return
	}

	line := string(*h.line)

	if len(strings.TrimSpace(line)) == 0 {
		return
	}

	for _, history := range h.list {
		if history == nil {
			continue
		}

		// Don't write it if the history source has reached
		// the maximum number of lines allowed (inputrc)
		if h.maxEntries == 0 || h.maxEntries >= history.Len() {
			continue
		}

		var err error

		// Don't write the line if it's identical to the last one.
		last, err := history.GetLine(history.Len() - 1)
		if err == nil && last != "" && last == line {
			return
		}

		// Save the line and notify through hints if an error raised.
		h.hpos, err = history.Write(line)
		if err != nil {
			h.hint.Set(color.FgRed + err.Error())
		}
	}
}

// Accept signals the line has been accepted by the user and must be
// returned to the readline caller. If hold is true, the line is preserved
// and redisplayed on the next loop. If infer, the line is not written to
// the history, but preserved as a line to match against on the next loop.
// If infer is false, the line is automatically written to active sources.
func (h *Sources) Accept(hold, infer bool, err error) {
	h.accepted = true
	h.acceptHold = hold
	h.acceptLine = *h.line
	h.acceptErr = err

	// Simply write the line to the history sources.
	h.Write(infer)
}

// LineAccepted returns true if the user has accepted the line, signaling
// that the shell must return from its loop. The error can be nil, but may
// indicate a CtrlC/CtrlD style error.
// If the input line contains any comments (as defined by the configured
// comment sign), they will be removed before returning the line. Those
// are nonetheless preserved when the line is saved to history sources.
func (h *Sources) LineAccepted() (bool, string, error) {
	if !h.accepted {
		return false, "", nil
	}

	line := string(h.acceptLine)

	// Remove all comments before returning the line to the caller.
	comment := strings.Trim(h.opts.GetString("comment-begin"), "\"")
	commentPattern := fmt.Sprintf(`(^|\s)%s.*`, comment)
	if commentsMatch, err := regexp.Compile(commentPattern); err == nil {
		line = commentsMatch.ReplaceAllString(line, "")
	}

	// Revert all state changes to all lines.
	if h.opts.GetBool("revert-all-at-newline") {
		for source := range h.lines {
			h.lines[source] = make(map[int]*lineHistory)
		}
	}

	return true, line, h.acceptErr
}

// InsertMatch replaces the buffer with the first history line matching the provided buffer,
// between its beginning and the cursor position, either as a substring, or as a prefix.
func (h *Sources) InsertMatch(line *core.Line, cur *core.Cursor, usePos, fwd, regexp bool) {
	if len(h.list) == 0 {
		return
	}

	if h.Current() == nil {
		return
	}

	match, pos, found := h.match(line, cur, usePos, fwd, regexp)
	if !found {
		return
	}

	h.hpos = h.Current().Len() - pos
	h.line.Set([]rune(match)...)
	h.cursor.Set(h.line.Len())
}

// InferNext finds a line matching the current line in the history,
// finds the next line after it and, if any, inserts it.
func (h *Sources) InferNext() {
	if len(h.list) == 0 {
		return
	}

	history := h.Current()
	if history == nil {
		return
	}

	_, pos, found := h.match(h.line, nil, false, false, false)
	if !found {
		return
	}

	// If we have no match we return, or check for the next line.
	if history.Len() <= (history.Len()-pos)+1 {
		return
	}

	// Insert the next line
	line, err := history.GetLine(pos + 1)
	if err != nil {
		return
	}

	h.line.Set([]rune(line)...)
	h.cursor.Set(h.line.Len())
}

// Suggest returns the first line matching the current line buffer,
// so that caller can use for things like history autosuggestion.
// If no line matches the current line, it will return the latter.
func (h *Sources) Suggest(line *core.Line) core.Line {
	if len(h.list) == 0 || len(*line) == 0 {
		return *line
	}

	if h.Current() == nil {
		return *line
	}

	suggested, _, found := h.match(line, nil, false, false, false)
	if !found {
		return *line
	}

	return core.Line([]rune(suggested))
}

// Complete returns completions with the current history source values.
// If forward is true, the completions are proposed from the most ancient
// line in the history source to the most recent. If filter is true,
// only lines that match the current input line as a prefix are given.
func (h *Sources) Complete(forward, filter bool) completion.Values {
	if len(h.list) == 0 {
		return completion.Values{}
	}

	history := h.Current()
	if history == nil {
		return completion.Values{}
	}

	h.hint.Set(color.Bold + color.FgCyanBright + h.names[h.sourcePos] + color.Reset)

	compLines := make([]completion.Candidate, 0)

	// Set up iteration clauses
	var histPos int
	var done func(i int) bool
	var move func(inc int) int

	if forward {
		histPos = -1
		done = func(i int) bool { return i < history.Len()-1 }
		move = func(pos int) int { return pos + 1 }
	} else {
		histPos = history.Len()
		done = func(i int) bool { return i > 0 }
		move = func(pos int) int { return pos - 1 }
	}

	// And generate the completions.
nextLine:
	for done(histPos) {
		histPos = move(histPos)

		line, err := history.GetLine(histPos)
		if err != nil {
			continue
		}

		if strings.TrimSpace(line) == "" {
			continue
		}

		if filter && !strings.HasPrefix(line, string(*h.line)) {
			continue
		}

		display := strings.ReplaceAll(line, "\n", ` `)

		for _, comp := range compLines {
			if comp.Display == line {
				continue nextLine
			}
		}

		// Proper pad for indexes
		indexStr := strconv.Itoa(histPos)
		pad := strings.Repeat(" ", len(strconv.Itoa(history.Len()))-len(indexStr))
		display = fmt.Sprintf("%s%s %s%s", color.Dim, indexStr+pad, color.DimReset, display)

		value := completion.Candidate{
			Display: display,
			Value:   line,
		}

		compLines = append(compLines, value)
	}

	comps := completion.AddRaw(compLines)
	comps.NoSort["*"] = true
	comps.ListLong["*"] = true
	comps.PREFIX = string(*h.line)

	return comps
}

// Name returns the name of the currently active history source.
func (h *Sources) Name() string {
	return h.names[h.sourcePos]
}

func (h *Sources) match(match *core.Line, cur *core.Cursor, usePos, fwd, regex bool) (line string, pos int, found bool) {
	if len(h.list) == 0 {
		return
	}

	history := h.Current()
	if history == nil {
		return
	}

	// Set up iteration clauses
	var histPos int
	var done func(i int) bool
	var move func(inc int) int

	if fwd {
		histPos = -1
		done = func(i int) bool { return i < history.Len() }
		move = func(pos int) int { return pos + 1 }
	} else {
		histPos = history.Len()
		done = func(i int) bool { return i > 0 }
		move = func(pos int) int { return pos - 1 }
	}

	if usePos && h.hpos > 0 {
		histPos = history.Len() - h.hpos
	}

	for done(histPos) {
		// Set index and fetch the line
		histPos = move(histPos)

		histline, err := history.GetLine(histPos)
		if err != nil {
			return
		}

		// Matching: either as substring (regex) or since beginning.
		switch {
		case regex:
			line := string(*match)
			if cur.Pos() < match.Len() {
				line = line[:cur.Pos()]
			}

			regexLine, err := regexp.Compile(regexp.QuoteMeta(line))
			if err != nil {
				continue
			}

			// Go to next line if not matching as a substring.
			if !regexLine.MatchString(histline) {
				continue
			}

		default:
			// If too short or if not fully matching
			if len(histline) < match.Len() || !strings.HasPrefix(histline, string(*match)) {
				continue
			}
		}

		// Else we have our history match.
		return histline, histPos, true
	}

	// We should have returned a match from the loop.
	return "", 0, false
}

func (h *Sources) setLineCursorMatch(next string) {
	// Save the current cursor position when not saved before.
	if h.cpos == -1 && h.line.Len() > 0 && h.cursor.Pos() < h.line.Len()-1 {
		h.cpos = h.cursor.Pos()
	}

	h.line.Set([]rune(next)...)

	// Set cursor depending on inputrc options and line length.
	if h.opts.GetBool("history-preserve-point") && h.line.Len() > h.cpos && h.cpos != -1 {
		h.cursor.Set(h.cpos)
	} else {
		h.cursor.Set(h.line.Len())
	}
}

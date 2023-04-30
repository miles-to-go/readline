package readline

import (
	"strings"

	"github.com/reeflective/readline/inputrc"
	"github.com/reeflective/readline/internal/core"
	"github.com/reeflective/readline/internal/history"
	"github.com/reeflective/readline/internal/strutil"
)

//
// API ----------------------------------------------------------------
//

// History is an interface to allow you to write your own history logging tools.
// By default readline will just use an in-memory history satisfying this interface,
// which only logs the history to memory ([]string to be precise).
// Users who want an easy to use, file-based history should use NewHistoryFromFile().
type History = history.Source

// NewHistoryFromFile creates a new command history source writing to
// and reading from a file. The caller should bind the history source
// returned from this call to the readline instance, with AddHistory().
var NewHistoryFromFile = history.NewSourceFromFile

// NewInMemoryHistory creates a new in-memory command history source.
// The caller should bind the history source returned from this call
// to the readline instance, with AddHistory().
var NewInMemoryHistory = history.NewInMemoryHistory

// AddHistoryFromFile adds a command history source from a file path.
// The name is used when using/searching the history source.
func (rl *Shell) AddHistoryFromFile(name, filepath string) {
	rl.histories.AddFromFile(name, filepath)
}

// Add adds a source of history lines bound to a given name (printed above
// this source when used). When only the default in-memory history is bound,
// it's replaced with the provided source. Following ones are added to the list.
func (rl *Shell) AddHistory(name string, source history.Source) {
	rl.histories.Add(name, source)
}

// Delete deletes one or more history source by name.
// If no arguments are passed, all currently bound sources are removed.
func (rl *Shell) DeleteHistory(sources ...string) {
	rl.histories.Delete(sources...)
}

// historyCommands returns all history commands.
// Under each comment are gathered all commands related to the comment's
// subject. When there are two subgroups separated by an empty line, the
// second one comprises commands that are not legacy readline commands.
func (rl *Shell) historyCommands() commands {
	widgets := map[string]func(){
		"accept-line":                            rl.acceptLine,
		"next-history":                           rl.downHistory,
		"previous-history":                       rl.upHistory,
		"beginning-of-history":                   rl.beginningOfHistory,
		"end-of-history":                         rl.endOfHistory,
		"operate-and-get-next":                   rl.acceptLineAndDownHistory,
		"fetch-history":                          rl.fetchHistory,
		"forward-search-history":                 rl.forwardSearchHistory,
		"reverse-search-history":                 rl.reverseSearchHistory,
		"non-incremental-forward-search-history": rl.nonIncrementalForwardSearchHistory,
		"non-incremental-reverse-search-history": rl.nonIncrementalReverseSearchHistory,
		"history-search-forward":                 rl.historySearchForward,
		"history-search-backward":                rl.historySearchBackward,
		"history-substring-search-forward":       rl.historySubstringSearchForward,
		"history-substring-search-backward":      rl.historySubstringSearchBackward,
		"yank-last-arg":                          rl.yankLastArg,
		"yank-nth-arg":                           rl.yankNthArg,
		"magic-space":                            rl.magicSpace,

		"accept-and-hold":                    rl.acceptAndHold,
		"accept-and-infer-next-history":      rl.acceptAndInferNextHistory,
		"down-line-or-history":               rl.downLineOrHistory,
		"up-line-or-history":                 rl.upLineOrHistory,
		"up-line-or-search":                  rl.upLineOrSearch,
		"down-line-or-search":                rl.downLineOrSearch,
		"infer-next-history":                 rl.inferNextHistory,
		"beginning-of-buffer-or-history":     rl.beginningOfBufferOrHistory,
		"end-of-buffer-or-history":           rl.endOfBufferOrHistory,
		"beginning-of-line-hist":             rl.beginningOfLineHist,
		"end-of-line-hist":                   rl.endOfLineHist,
		"incremental-forward-search-history": rl.incrementalForwardSearchHistory,
		"incremental-reverse-search-history": rl.incrementalReverseSearchHistory,
		"autosuggest-accept":                 rl.autosuggestAccept,
		"autosuggest-execute":                rl.autosuggestExecute,
		"autosuggest-enable":                 rl.autosuggestEnable,
		"autosuggest-disable":                rl.autosuggestDisable,
		"autosuggest-toggle":                 rl.autosuggestToggle,
	}

	return widgets
}

//
// Standard ----------------------------------------------------------------
//

// Finish editing the buffer. Normally this causes the buffer to be executed as a shell command.
func (rl *Shell) acceptLine() {
	rl.acceptLineWith(false, false)
}

// Move to the next event in the history list.
func (rl *Shell) downHistory() {
	rl.histories.SkipSave()
	rl.histories.Walk(-1)
}

// Move to the previous event in the history list.
func (rl *Shell) upHistory() {
	rl.histories.SkipSave()
	rl.histories.Walk(1)
}

// Move to the first event in the history list.
func (rl *Shell) beginningOfHistory() {
	rl.histories.SkipSave()

	history := rl.histories.Current()
	if history == nil {
		return
	}

	rl.histories.Walk(history.Len())
}

// Move to the last event in the history list.
func (rl *Shell) endOfHistory() {
	history := rl.histories.Current()

	if history == nil {
		return
	}

	rl.histories.Walk(-history.Len() + 1)
}

// Execute the current line, and push the next history event on the buffer stack.
func (rl *Shell) acceptLineAndDownHistory() {
	rl.acceptLineWith(true, false)
}

// With a numeric argument, fetch that entry from the history
// list and make it the current line.  Without an argument,
// move back to the first entry in the history list.
func (rl *Shell) fetchHistory() {
	if rl.iterations.IsSet() {
		rl.histories.Fetch(rl.iterations.Get())
	} else {
		rl.histories.Fetch(0)
	}
}

// Search forward starting at the current line and moving `down' through
// the history as necessary.  This is an incremental search, opening and
// showing matching completions.
func (rl *Shell) forwardSearchHistory() {
	rl.histories.SkipSave()
	rl.historyCompletion(true, false, true)
}

// Search backward starting at the current line and moving `up' through
// the history as necessary.  This is an incremental search, opening and
// showing matching completions.
func (rl *Shell) reverseSearchHistory() {
	rl.histories.SkipSave()
	rl.historyCompletion(false, false, true)
}

// Search forward through the history starting at the current line
// using a non-incremental search for a string supplied by the user.
func (rl *Shell) nonIncrementalForwardSearchHistory() {
	rl.completer.NonIsearchStart(rl.histories.Name(), false, true, true)
}

// Search backward through the history starting at the current line
// using a non-incremental search for a string supplied by the user.
func (rl *Shell) nonIncrementalReverseSearchHistory() {
	rl.completer.NonIsearchStart(rl.histories.Name(), false, false, true)
}

// Search forward through the history for the string of characters
// between the start of the current line and the point.  The search
// string must match at the beginning of a history line.
// This shows the completions in autocomplete mode.
func (rl *Shell) historySearchForward() {
	rl.histories.Save()

	cpos := rl.cursor.Pos()
	rl.histories.InsertMatch(rl.line, rl.cursor, true, true, false)
	rl.cursor.Set(cpos)
}

// Search backward through the history for the string of characters
// between the start of the current line and the point.  The search
// string must match at the beginning of a history line.
// This shows the completions in autocomplete mode.
func (rl *Shell) historySearchBackward() {
	rl.histories.Save()

	cpos := rl.cursor.Pos()
	rl.histories.InsertMatch(rl.line, rl.cursor, true, false, false)
	rl.cursor.Set(cpos)
}

// Search forward through the history for the string of characters
// between the start of the current line and the current cursor position.
// The search string may match anywhere in a history line.  This is a non-
// incremental search.
func (rl *Shell) historySubstringSearchForward() {
	rl.histories.InsertMatch(rl.line, rl.cursor, true, true, true)
}

// Search backward through the history for the string of characters
// between the start of the current line and the current cursor position.
// The search string may match anywhere in a history line.  This is a non-
// incremental search.
func (rl *Shell) historySubstringSearchBackward() {
	rl.histories.InsertMatch(rl.line, rl.cursor, true, false, true)
}

// Insert the last argument to the previous command (the last
// word of the previous history entry).  With a numeric
// argument, behave exactly like yank-nth-arg.  Successive
// calls to yank-last-arg move back through the history list,
// inserting the last word (or the word specified by the
// argument to the first call) of each line in turn.
// Any numeric argument supplied to these successive calls
// determines the direction to move through the history.
// A negative argument switches the direction through the
// history (back or forward).  The history expansion
// facilities are used to extract the last argument, as if
// the "!$" history expansion had been specified.
func (rl *Shell) yankLastArg() {
	// Get the last history line.
	last := rl.histories.GetLast()
	if last == "" {
		return
	}

	// Split it into words, and get the last one.
	words, err := strutil.Split(last)
	if err != nil || len(words) == 0 {
		return
	}

	// Get the last word, and quote it if it contains spaces.
	lastArg := words[len(words)-1]
	if strings.ContainsAny(lastArg, " \t") {
		if strings.Contains(lastArg, "\"") {
			lastArg = "'" + lastArg + "'"
		} else {
			lastArg = "\"" + lastArg + "\""
		}
	}

	// And append it to the end of the line.
	rl.line.Insert(rl.cursor.Pos(), []rune(lastArg)...)
	rl.cursor.Move(len(lastArg))
}

// Insert the first argument to the previous command (usually
// the second word on the previous line) at point.  With an
// argument n, insert the nth word from the previous command
// (the words in the previous command begin with word 0).
// A negative argument inserts the nth word from the end of the
// previous command.  Once the argument n is computed, the argument
// is extracted as if the "!n" history expansion had been specified.
func (rl *Shell) yankNthArg() {
	// Get the last history line.
	last := rl.histories.GetLast()
	if last == "" {
		return
	}

	// Split it into words, and get the last one.
	words, err := strutil.Split(last)
	if err != nil || len(words) == 0 {
		return
	}

	var lastArg string

	// Abort if the required position is out of bounds.
	argNth := rl.iterations.Get()
	if len(words) < argNth {
		return
	} else {
		lastArg = words[argNth-1]
	}

	// Quote if required.
	if strings.ContainsAny(lastArg, " \t") {
		if strings.Contains(lastArg, "\"") {
			lastArg = "'" + lastArg + "'"
		} else {
			lastArg = "\"" + lastArg + "\""
		}
	}

	// And append it to the end of the line.
	rl.line.Insert(rl.line.Len(), []rune(lastArg)...)
	rl.cursor.Move(len(lastArg))
}

// Perform history expansion on the current line and insert a space.
// If the current blank word under cursor starts with an exclamation
// mark, the word up to the cursor is matched as a prefix against
// the history lines, and the first match is inserted in place of it.
func (rl *Shell) magicSpace() {
	cpos := rl.cursor.Pos()
	lineLen := rl.line.Len()

	// If no line, or the cursor is on a space, we can't perform.
	if lineLen == 0 || (cpos == lineLen && (*rl.line)[cpos-1] == inputrc.Space) {
		rl.selfInsert()
		return
	}

	// Select the word around cursor.
	rl.viSelectInBlankWord()
	word, bpos, _, _ := rl.selection.Pop()
	rl.cursor.Set(cpos)

	// Fail if empty or not prefixed expandable.
	if len(strings.TrimSpace(word)) == 0 {
		rl.selfInsert()
		return
	}

	if !strings.HasPrefix(word, "!") || word == "!" {
		rl.selfInsert()
		return
	}

	// Else, perform expansion on the remainder.
	pattern := core.Line((*rl.line)[bpos+1:])
	suggested := rl.histories.Suggest(&pattern)

	if string(suggested) == string(pattern) {
		rl.selfInsert()
		return
	}

	rl.histories.Save()
	rl.line.Cut(bpos, lineLen)
	rl.line.Insert(bpos, suggested...)
	rl.cursor.Set(bpos + suggested.Len())
}

//
// Added -------------------------------------------------------------------
//

// Accept the current input line (execute it) and
// keep it as the buffer on the next readline loop.
func (rl *Shell) acceptAndHold() {
	rl.acceptLineWith(false, true)
}

// Execute the contents of the buffer. Then search the history list for a line
// matching the current one and push the event following onto the buffer stack.
func (rl *Shell) acceptAndInferNextHistory() {
	rl.acceptLineWith(true, false)
}

// Move down a line in the buffer, or if already at the
// bottom line, move to the next event in the history list.
func (rl *Shell) downLineOrHistory() {
	rl.histories.SkipSave()

	times := rl.iterations.Get()
	linesDown := rl.line.Lines() - rl.cursor.Line()

	// If we can go down some lines out of
	// the available iterations, use them.
	if linesDown > 0 {
		rl.cursor.LineMove(times)
		times -= linesDown
	}

	if times > 0 {
		rl.histories.Walk(times * -1)
	}
}

// Move up a line in the buffer, or if already at the top
// line, move to the previous event in the history list.
func (rl *Shell) upLineOrHistory() {
	rl.histories.SkipSave()

	times := rl.iterations.Get()
	linesUp := rl.cursor.Line()

	// If we can go down some lines out of
	// the available iterations, use them.
	if linesUp > 0 {
		rl.cursor.LineMove(times * -1)
		times -= linesUp
	}

	if times > 0 {
		rl.histories.Walk(times)
	}
}

// If the cursor is on the first line of the buffer, start an incremental
// search backward on the history lines. Otherwise, move up a line in the buffer.
func (rl *Shell) upLineOrSearch() {
	rl.histories.SkipSave()
	switch {
	case rl.cursor.Line() > 0:
		rl.cursor.LineMove(-1)
	default:
		rl.historySearchBackward()
	}
}

// If the cursor is on the last line of the buffer, start an incremental
// search forward on the history lines. Otherwise, move up a line in the buffer.
func (rl *Shell) downLineOrSearch() {
	rl.histories.SkipSave()
	switch {
	case rl.cursor.Line() < rl.line.Lines():
		rl.cursor.LineMove(1)
	default:
		rl.historySearchForward()
	}
}

// Attempt to find a line in history matching the current line buffer as a prefix,
// and if one is found, fetch the next history event and make it the current buffer.
func (rl *Shell) inferNextHistory() {
	rl.histories.SkipSave()
	rl.histories.InferNext()
}

// If the cursor is not at the beginning of the buffer, go to it.
// Otherwise, go to the beginning of history.
func (rl *Shell) beginningOfBufferOrHistory() {
	rl.histories.SkipSave()

	if rl.cursor.Pos() > 0 {
		rl.cursor.Set(0)
		return
	}

	rl.beginningOfHistory()
}

// If the cursor is not at the end of the buffer, go to it.
// Otherwise, go to the end of history.
func (rl *Shell) endOfBufferOrHistory() {
	rl.histories.SkipSave()

	if rl.cursor.Pos() < rl.line.Len()-1 {
		rl.cursor.Set(rl.line.Len())
		return
	}

	rl.endOfHistory()
}

// Go to the beginning of the current line, if the cursor is not yet.
// If at the beginning of the line, attempt to move one line up.
// If at the beginning of the buffer, move up one history line.
func (rl *Shell) beginningOfLineHist() {
	rl.histories.SkipSave()

	switch {
	case rl.cursor.Pos() > 0:
		if rl.cursor.AtBeginningOfLine() {
			rl.cursor.Dec()
		}
		rl.beginningOfLine()
	default:
		rl.histories.Walk(1)
	}
}

// Go to the end of the current line, if the cursor is not yet.
// If at the end of the line, attempt to move one line down.
// If at the end of the buffer, move up one history line.
func (rl *Shell) endOfLineHist() {
	rl.histories.SkipSave()

	switch {
	case rl.cursor.Pos() < rl.line.Len()-1:
		if rl.cursor.AtEndOfLine() {
			rl.cursor.Inc()
		}

		rl.endOfLine()

	default:
		rl.histories.Walk(-1)
	}
}

// Start an forward history autocompletion mode, starting at the
// current line and moving `down' through the history as necessary.
func (rl *Shell) incrementalForwardSearchHistory() {
	rl.histories.SkipSave()
	rl.historyCompletion(true, true, false)
}

// Start an backward history autocompletion mode, starting at the
// current line and moving `down' through the history as necessary.
func (rl *Shell) incrementalReverseSearchHistory() {
	rl.histories.SkipSave()
	rl.historyCompletion(false, true, false)
}

// If a line is currently autoggested, make it the buffer.
func (rl *Shell) autosuggestAccept() {
	suggested := rl.histories.Suggest(rl.line)

	if suggested.Len() <= rl.line.Len() {
		return
	}

	rl.line.Set(suggested...)
	rl.cursor.Set(len(suggested))
}

// If a line is currently autoggested, make it the buffer and execute it.
func (rl *Shell) autosuggestExecute() {
	suggested := rl.histories.Suggest(rl.line)

	if suggested.Len() <= rl.line.Len() {
		return
	}

	rl.line.Set(suggested...)
	rl.cursor.Set(len(suggested))

	rl.acceptLine()
}

// Toggle line history autoggestions on/off.
func (rl *Shell) autosuggestToggle() {
	if rl.config.GetBool("history-autosuggest") {
		rl.autosuggestDisable()
	} else {
		rl.autosuggestEnable()
	}
}

// Enable history line autoggestions.
// When enabled and if a line is suggested, forward-word commands, will
// take the first word of the non-inserted part of this suggestion and
// will insert it in the real input line.
// The forward-char* commands, if at the end of the line, will accept it.
func (rl *Shell) autosuggestEnable() {
	rl.histories.SkipSave()
	rl.config.Vars["history-autosuggest"] = true
}

// Disable history line autoggestions.
func (rl *Shell) autosuggestDisable() {
	rl.histories.SkipSave()
	rl.config.Vars["history-autosuggest"] = false
}

//
// Utils -------------------------------------------------------------------
//

func (rl *Shell) acceptLineWith(infer, hold bool) {
	// If we are currently using the incremental-search buffer,
	// we should cancel this mode so as to run the rest of this
	// function on (with) the input line itself, not the minibuffer.
	rl.completer.Reset()

	// Non-incremental search modes are the only mode not cancelled
	// by the completion engine. If it's active, match the line result
	// and return without returning the line to the readline caller.
	searching, forward, substring := rl.completer.NonIncrementallySearching()
	if searching {
		defer rl.completer.NonIsearchStop()

		line, cursor, _ := rl.completer.GetBuffer()

		rl.histories.InsertMatch(line, cursor, true, forward, substring)

		return
	}

	// Use the correct buffer for the rest of the function.
	rl.line, rl.cursor, rl.selection = rl.completer.GetBuffer()

	// Without multiline support, we always return the line.
	if rl.AcceptMultiline == nil {
		rl.display.AcceptLine()
		rl.histories.Accept(hold, infer, nil)

		return
	}

	// Ask the caller if the line should be accepted
	// as is, save the command line and accept it.
	if rl.AcceptMultiline(*rl.line) {
		rl.display.AcceptLine()
		rl.histories.Accept(hold, infer, nil)

		return
	}

	// If not, we should start editing another line,
	// and insert a newline where our cursor value is.
	// This has the nice advantage of being able to work
	// in multiline mode even in the middle of the buffer.
	rl.line.Insert(rl.cursor.Pos(), '\n')
	rl.cursor.Inc()
}

func (rl *Shell) insertAutosuggestPartial(emacs bool) {
	cpos := rl.cursor.Pos()
	if cpos < rl.line.Len()-1 {
		return
	}

	if !rl.config.GetBool("history-autosuggest") {
		return
	}

	suggested := rl.histories.Suggest(rl.line)
	if suggested.Len() > rl.line.Len() {

		var forward int

		if emacs {
			forward = suggested.ForwardEnd(suggested.Tokenize, cpos)
		} else {
			forward = suggested.Forward(suggested.Tokenize, cpos)
		}

		if cpos+1+forward > suggested.Len() {
			forward = suggested.Len() - cpos - 1
		}

		rl.line.Insert(cpos+1, suggested[cpos+1:cpos+forward+1]...)
	}
}

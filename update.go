package readline

import (
	"errors"
	"fmt"
	"strings"
)

// RefreshMultiline - Reprints the prompt considering several parameters:
// @prompt      => If not nil (""), will use this prompt instead of the currently set prompt.
// @offset      => Used to set the number of lines to go upward, before reprinting. Set to 0 if not used.
// @clearLine   => If true, will clean the current input line on the next refresh.
// Please check the Instance.HideNextPrompt variable and its effects on this function !
func (rl *Instance) RefreshMultiline(prompt string, offset int, clearLine bool) (err error) {

	if !rl.Multiline {
		return errors.New("readline error: refresh cannot happen, prompt is not multiline")
	}

	// We adjust cursor movement, depending on which mode we're currently in.
	if !rl.modeTabCompletion {
		rl.tcUsedY = 1
	} else if rl.modeTabCompletion && rl.modeAutoFind { // Account for the hint line
		rl.tcUsedY = 0
	} else {
		rl.tcUsedY = 1
	}

	// Add user-provided offset
	rl.tcUsedY += offset

	// Clear the input line and everything below
	print(seqClearLine)
	moveCursorUp(rl.hintY + rl.tcUsedY)
	moveCursorBackwards(GetTermWidth())
	print("\r\n" + seqClearScreenBelow)

	// Update the prompt if a special has been passed.
	if prompt != "" {
		rl.prompt = prompt
	}

	// Only print the prompt if we have not been instructed to hide it.
	if !rl.HideNextPrompt {
		fmt.Println(rl.prompt)
		rl.renderHelpers()
	}

	// If input line was empty, check that we clear it from detritus
	// The three lines are borrowed from clearLine(), we don't need more.
	if clearLine {
		rl.clearLine()
	}

	return
}

// computePrompt - At any moment, returns prompt actualized with Vim status
func (rl *Instance) computePrompt() (prompt []rune) {

	// If single line prompt, and the prompt is not nil, the user has set it,
	// so we put up everything together, compute legnths and return.
	if rl.prompt != "" && !rl.Multiline {
		rl.mlnPrompt = []rune(rl.prompt)
		rl.promptLen = len(rl.mlnPrompt)
		return rl.mlnPrompt
	}

	// If ModeVimEnabled, append it and compute details.
	var colorPromptOffset int
	if rl.InputMode == Vim && rl.ShowVimMode {

		switch rl.modeViMode {
		case vimKeys:
			prompt = append(prompt, []rune(vimKeysStr)...)
		case vimInsert:
			prompt = append(prompt, []rune(vimInsertStr)...)
		case vimReplaceOnce:
			prompt = append(prompt, []rune(vimReplaceOnceStr)...)
		case vimReplaceMany:
			prompt = append(prompt, []rune(vimReplaceManyStr)...)
		case vimDelete:
			prompt = append(prompt, []rune(vimDeleteStr)...)
		}

		// Process colors, and get offset for correct cursor position
		bwPromptLen := len(prompt)
		prompt = rl.colorizeVimPrompt(prompt)

		colorPromptLen := len(prompt)
		colorPromptOffset = colorPromptLen - bwPromptLen
	}

	// Add custom multiline prompt string if provided by user
	if rl.MultilinePrompt != "" {
		prompt = append(prompt, []rune(rl.MultilinePrompt)...)
	} else {
		// Else add the default arrow
		prompt = append(prompt, rl.mlnArrow...)
	}

	// We have our prompt, adjust for any coloring
	rl.mlnPrompt = prompt
	rl.promptLen = len(rl.mlnPrompt) - colorPromptOffset

	return
}

func moveCursorUp(i int) {
	if i < 1 {
		return
	}

	printf("\x1b[%dA", i)
}

func moveCursorDown(i int) {
	if i < 1 {
		return
	}

	printf("\x1b[%dB", i)
}

func moveCursorForwards(i int) {
	if i < 1 {
		return
	}

	printf("\x1b[%dC", i)
}

func moveCursorBackwards(i int) {
	if i < 1 {
		return
	}

	printf("\x1b[%dD", i)
}

// moveCursorToLinePos - Must calculate the length of the prompt, realtime
// and for all contexts/needs, and move the cursor appropriately
func moveCursorToLinePos(rl *Instance) {
	moveCursorForwards(rl.promptLen + rl.pos)
	return
}

func (rl *Instance) moveCursorByAdjust(adjust int) {
	switch {
	case adjust > 0:
		moveCursorForwards(adjust)
		rl.pos += adjust
	case adjust < 0:
		moveCursorBackwards(adjust * -1)
		rl.pos += adjust
	}

	if rl.modeViMode != vimInsert && rl.pos == len(rl.line) && len(rl.line) > 0 {
		moveCursorBackwards(1)
		rl.pos--
	}
}

func (rl *Instance) insert(r []rune) {
	for {
		// I don't really understand why `0` is creaping in at the end of the
		// array but it only happens with unicode characters.
		if len(r) > 1 && r[len(r)-1] == 0 {
			r = r[:len(r)-1]
			continue
		}
		break
	}

	switch {
	case len(rl.line) == 0:
		rl.line = r
	case rl.pos == 0:
		rl.line = append(r, rl.line...)
	case rl.pos < len(rl.line):
		r := append(r, rl.line[rl.pos:]...)
		rl.line = append(rl.line[:rl.pos], r...)
	default:
		rl.line = append(rl.line, r...)
	}

	rl.echo()

	rl.pos += len(r)
	moveCursorForwards(len(r) - 1)

	if rl.modeViMode == vimInsert {
		rl.updateHelpers()
	}
}

func (rl *Instance) backspace() {
	if len(rl.line) == 0 || rl.pos == 0 {
		return
	}

	moveCursorBackwards(1)
	rl.pos--
	rl.delete()
}

func (rl *Instance) delete() {
	switch {
	case len(rl.line) == 0:
		return
	case rl.pos == 0:
		rl.line = rl.line[1:]
		rl.echo()
		moveCursorBackwards(1)
	case rl.pos > len(rl.line):
		rl.backspace()
	case rl.pos == len(rl.line):
		rl.line = rl.line[:rl.pos]
		rl.echo()
		moveCursorBackwards(1)
	default:
		rl.line = append(rl.line[:rl.pos], rl.line[rl.pos+1:]...)
		rl.echo()
		moveCursorBackwards(1)
	}

	rl.updateHelpers()
}

func (rl *Instance) echo() {

	// We move the cursor back to the very beginning of the line:
	// prompt + cursor position
	moveCursorBackwards(rl.promptLen + rl.pos)

	switch {
	case rl.PasswordMask > 0:
		print(strings.Repeat(string(rl.PasswordMask), len(rl.line)) + " ")

	case rl.SyntaxHighlighter == nil:
		print(string(rl.mlnPrompt))

		// Depending on the presence of a virtually completed item,
		// print either the virtual line or the real one.
		if len(rl.currentComp) > 0 {
			line := rl.lineComp[:rl.pos]
			line = append(line, rl.lineRemain...)
			print(string(line) + " ")
		} else {
			print(string(rl.line) + " ")
			moveCursorBackwards(len(rl.line) - rl.pos)
		}

	default:
		print(string(rl.mlnPrompt))

		// Depending on the presence of a virtually completed item,
		// print either the virtual line or the real one.
		if len(rl.currentComp) > 0 {
			line := rl.lineComp[:rl.pos]
			line = append(line, rl.lineRemain...)
			print(rl.SyntaxHighlighter(line) + " ")
		} else {
			print(rl.SyntaxHighlighter(rl.line) + " ")
			moveCursorBackwards(len(rl.line) - rl.pos)
		}
	}

	// moveCursorBackwards(len(rl.line) - rl.pos)
}

func (rl *Instance) clearLine() {
	if len(rl.line) == 0 {
		return
	}

	var lineLen int
	if len(rl.lineComp) > len(rl.line) {
		lineLen = len(rl.lineComp)
	} else {
		lineLen = len(rl.line)
	}

	moveCursorBackwards(rl.pos)
	print(strings.Repeat(" ", lineLen))
	moveCursorBackwards(lineLen)

	// Real input line
	rl.line = []rune{}
	rl.pos = 0

	// Completions are also reset
	rl.clearVirtualComp()
}

func (rl *Instance) resetHelpers() {
	rl.modeAutoFind = false
	rl.clearHelpers()
	rl.resetHintText()
	rl.resetTabCompletion()
}

func (rl *Instance) clearHelpers() {
	print("\r\n" + seqClearScreenBelow)
	moveCursorUp(1)
	moveCursorToLinePos(rl)

	// Reset some values
	rl.lineComp = []rune{}
	rl.currentComp = []rune{}
}

func (rl *Instance) renderHelpers() {

	rl.echo()

	// If we are waiting for confirmation (too many comps),
	// do not overwrite the confirmation question hint.
	if !rl.compConfirmWait {
		// We also don't overwrite if in tab find mode, which has a special hint.
		if !rl.modeAutoFind {
			rl.getHintText()
		}
		// We write the hint anyway
		rl.writeHintText()
	}

	rl.writeTabCompletion()
	moveCursorUp(rl.tcUsedY)

	if !rl.compConfirmWait {
		moveCursorUp(rl.hintY)
	}
	moveCursorBackwards(GetTermWidth())

	moveCursorToLinePos(rl)
}

// This one has the advantage of not stacking hints and completions, pretty balanced.
// However there is a problem with it when we use completion while being in the middle of the line.
// func (rl *Instance) renderHelpers() {
//
//         rl.echo() // Added by me, so that prompt always appear when new line
//
//         // If we are waiting for confirmation (too many comps), do not overwrite the hints.
//         if !rl.compConfirmWait {
//                 rl.getHintText()
//                 rl.writeHintText()
//                 moveCursorUp(rl.hintY)
//         }
//
//         rl.writeTabCompletion()
//         moveCursorUp(rl.tcUsedY)

//         moveCursorBackwards(GetTermWidth())
//         moveCursorToLinePos(rl)
// }

func (rl *Instance) updateHelpers() {
	rl.tcOffset = 0
	rl.getHintText()
	if rl.modeTabCompletion {
		rl.getTabCompletion()
	}
	rl.clearHelpers()
	rl.renderHelpers()
}

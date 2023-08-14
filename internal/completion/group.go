package completion

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/reeflective/readline/internal/color"
	"github.com/reeflective/readline/internal/term"
	"golang.org/x/exp/slices"
)

// group is used to structure different types of completions with different
// display types, autosuffix removal matchers, under their tag heading.
type group struct {
	tag               string        // Printed on top of the group's completions
	rows              [][]Candidate // Values are grouped by aliases/rows, with computed paddings.
	noSpace           SuffixMatcher // Suffixes to remove if a space or non-nil character is entered after the completion.
	descriptions      []string
	columnsWidth      []int  // Computed width for each column of completions, when aliases
	descriptionsWidth []int  // Computed width for each column of completions, when aliases
	listSeparator     string // This is used to separate completion candidates from their descriptions.
	list              bool   // Force completions to be listed instead of grided
	noSort            bool   // Don't sort completions
	aliased           bool   // Are their aliased completions
	preserveEscapes   bool   // Preserve escape sequences in the completion inserted values.
	isCurrent         bool   // Currently cycling through this group, for highlighting choice
	maxDescWidth      int
	maxCellLength     int

	longestValueLen int // Used when display is map/list, for determining message width
	longestDescLen  int // Used to know how much descriptions can use when there are aliases.

	// Selectors (position/bounds) management
	posX int
	posY int
	maxX int
	maxY int

	// Constants passed by the engine.
	termWidth int
}

func (e *Engine) generateGroup(comps Values) func(tag string, values RawValues) {
	return func(tag string, values RawValues) {
		// Separate the completions that have a description and
		// those which don't, and devise if there are aliases.
		vals, noDescVals, descriptions := e.groupNonDescribed(&comps, values)

		// Create a "first" group with the "first" grouped values
		e.newCompletionGroup(comps, tag, vals, descriptions)

		// If we have a remaining group of values without descriptions,
		// we will print and use them in a separate, anonymous group.
		if len(noDescVals) > 0 {
			e.newCompletionGroup(comps, "", vals, descriptions)
		}
	}
}

// newCompletionGroup initializes a group of completions to be displayed in the same area/header.
func (e *Engine) newCompletionGroup(comps Values, tag string, vals RawValues, descriptions []string) {
	grp := &group{
		tag:            tag,
		noSpace:        comps.NoSpace,
		posX:           -1,
		posY:           -1,
		columnsWidth:   []int{0},
		termWidth:      term.GetWidth(),
		longestDescLen: longest(descriptions, true),
	}

	// Initialize all options for the group.
	grp.initOptions(e, &comps, tag, vals)

	// Global actions to take on all values.
	if !grp.noSort {
		sort.Stable(vals)
	}

	// Initial processing of our assigned values:
	// Compute color/no-color sizes, some max/min, etc.
	for pos, value := range vals {
		if value.Display == "" {
			value.Display = value.Value
		}

		// Only pass for colors regex should be here.
		value.displayLen = len(color.Strip(value.Display))
		value.descLen = len(color.Strip(value.Description))

		if value.displayLen > grp.longestValueLen {
			grp.longestValueLen = value.displayLen
		}

		if value.descLen > grp.longestDescLen {
			grp.longestDescLen = value.descLen
		}

		vals[pos] = value
	}

	// Generate the full grid of completions.
	// Special processing is needed when some values
	// share a common description, they are "aliased".
	if completionsAreAliases(vals) {
		grp.initCompletionAliased(vals)
	} else {
		grp.initCompletionsGrid(vals)
	}

	e.groups = append(e.groups, grp)
}

// groupNonDescribed separates values based on whether they have descriptions, or are aliases of each other.
func (e *Engine) groupNonDescribed(comps *Values, values RawValues) (vals, noDescVals RawValues, descs []string) {
	var descriptions []string

	prefix := ""
	if e.prefix != "\"\"" && e.prefix != "''" {
		prefix = e.prefix
	}

	for _, val := range values {
		// Ensure all values have a display string.
		if val.Display == "" {
			val.Display = val.Value
		}

		// Currently this is because errors are passed as completions.
		if strings.HasPrefix(val.Value, prefix+"ERR") && val.Value == prefix+"_" {
			comps.Messages.Add(color.FgRed + val.Display + val.Description)

			continue
		}

		// Grid completions
		if val.Description == "" {
			noDescVals = append(noDescVals, val)

			continue
		}

		descriptions = append(descriptions, val.Description)
		vals = append(vals, val)
	}

	// if no candidates have a description, swap
	if len(vals) == 0 {
		vals = noDescVals
		noDescVals = make(RawValues, 0)
	}

	return vals, noDescVals, descriptions
}

// initOptions checks for global or group-specific options (display, behavior, grouping, etc).
func (g *group) initOptions(eng *Engine, comps *Values, tag string, vals RawValues) {
	// Override grid/list displays
	_, g.list = comps.ListLong[tag]
	if _, all := comps.ListLong["*"]; all && len(comps.ListLong) == 1 {
		g.list = true
	}

	// Description list separator
	listSep, err := strconv.Unquote(eng.config.GetString("completion-list-separator"))
	if err != nil {
		g.listSeparator = "--"
	} else {
		g.listSeparator = listSep
	}

	// Strip escaped characters in the value component.
	g.preserveEscapes = comps.Escapes[g.tag]
	if !g.preserveEscapes {
		g.preserveEscapes = comps.Escapes["*"]
	}

	// Always list long commands when they have descriptions.
	if strings.HasSuffix(g.tag, "commands") && len(vals) > 0 && vals[0].Description != "" {
		g.list = true
	}

	// Description list separator
	listSep, found := comps.ListSep[tag]
	if !found {
		if allSep, found := comps.ListSep["*"]; found {
			g.listSeparator = allSep
		}
	} else {
		g.listSeparator = listSep
	}

	// Override sorting or sort if needed
	g.noSort = comps.NoSort[tag]
	if noSort, all := comps.NoSort["*"]; noSort && all && len(comps.NoSort) == 1 {
		g.noSort = true
	}
}

// initCompletionsGrid arranges completions when there are no aliases.
func (g *group) initCompletionsGrid(comps RawValues) {
	pairLength := g.longestValueDescribed(comps)
	maxColumns := g.termWidth / pairLength
	rowCount := int(math.Ceil(float64(len(comps)) / (float64(maxColumns))))

	g.rows = createGrid(comps, rowCount, maxColumns)
	g.calculateMaxColumnWidths(g.rows, len(g.rows[0]))

	for _, val := range comps {
		g.descriptions = append(g.descriptions, val.Description)
	}
}

// initCompletionsGrid arranges completions when some of them share the same description.
func (g *group) initCompletionAliased(domains []Candidate) {
	g.aliased = true

	// Filter out all duplicates: group aliased completions together.
	grid, descriptions := g.createDescribedRows(domains)

	// Get the number of columns we use and optimize them.
	var numColumns int
	for i := range grid {
		if len(grid[i]) > numColumns {
			numColumns = len(grid[i])
		}
	}

	g.calculateMaxColumnWidths(grid, numColumns)

	// Recursively pass over the values to
	// find an optimized layout for them.
	breakeven := 0
	maxColumns := numColumns
	for i, width := range g.columnsWidth {
		if (breakeven + width + 2) > g.termWidth/2 {
			maxColumns = i
			break
		}

		breakeven += width + 2
	}

	var rows [][]Candidate
	var descs []string

	for i := range grid {
		row := grid[i]
		split := false
		for len(row) > maxColumns {
			rows = append(rows, row[:maxColumns])
			row = row[maxColumns:]
			descs = append(descs, "|_")
			split = true
		}

		if split {
			descs = append(descs, "|"+descriptions[i])
		} else {
			descs = append(descs, descriptions[i])
		}

		rows = append(rows, row)
	}

	g.rows = rows
	g.columnsWidth = g.columnsWidth[:maxColumns]
	g.descriptions = descs
}

// This createDescribedRows function takes a list of values, a list of descriptions, and the
// terminal width as input, and returns a list of rows based on the provided requirements:.
func (g *group) createDescribedRows(values []Candidate) ([][]Candidate, []string) {
	descriptionMap := make(map[string][]Candidate)
	uniqueDescriptions := make([]string, 0)
	rows := make([][]Candidate, 0)

	// Separate duplicates and store them.
	for i, description := range values {
		if slices.Contains(uniqueDescriptions, description.Description) {
			descriptionMap[description.Description] = append(descriptionMap[description.Description], values[i])
		} else {
			uniqueDescriptions = append(uniqueDescriptions, description.Description)
			descriptionMap[description.Description] = []Candidate{values[i]}
		}
	}

	// Sorting helps with easier grids.
	for _, description := range uniqueDescriptions {
		row := descriptionMap[description]
		// slices.Sort(row)
		// slices.Reverse(row)
		rows = append(rows, row)
	}

	return rows, uniqueDescriptions
}

//
// Usage-time functions (selecting/writing) -----------------------------------------------------------------
//

// updateIsearch - When searching through all completion groups (whether it be command history or not),
// we ask each of them to filter its own items and return the results to the shell for aggregating them.
// The rx parameter is passed, as the shell already checked that the search pattern is valid.
func (g *group) updateIsearch(eng *Engine) {
	if eng.IsearchRegex == nil {
		return
	}

	suggs := make([]Candidate, 0)

	for i := range g.rows {
		row := g.rows[i]

		for _, val := range row {
			if eng.IsearchRegex.MatchString(val.Value) {
				suggs = append(suggs, val)
			} else if val.Description != "" && eng.IsearchRegex.MatchString(val.Description) {
				suggs = append(suggs, val)
			}
		}
	}

	// Reset the group parameters
	g.rows = make([][]Candidate, 0)
	g.posX = -1
	g.posY = -1
	g.columnsWidth = []int{0}

	// Assign the filtered values: we don't need to check
	// for a separate set of non-described values, as the
	// completions have already been triaged when generated.
	vals, _, descriptions := eng.groupNonDescribed(nil, suggs)
	g.aliased = len(slices.Compact(descriptions)) < len(descriptions)

	if len(vals) == 0 {
		return
	}

	// And perform the usual initialization routines.
	// vals = g.checkDisplays(vals)
	// g.computeCells(eng, vals)
	// g.makeMatrix(vals)
}

func (g *group) selected() (comp Candidate) {
	defer func() {
		if !g.preserveEscapes {
			comp.Value = color.Strip(comp.Value)
		}
	}()

	if g.posY == -1 || g.posX == -1 {
		return g.rows[0][0]
	}

	return g.rows[g.posY][g.posX]
}

func (g *group) moveSelector(x, y int) (done, next bool) {
	// When the group has not yet been used, adjust
	if g.posX == -1 && g.posY == -1 {
		if x != 0 {
			g.posY++
		} else {
			g.posX++
		}
	}

	g.posX += x
	g.posY += y
	reverse := (x < 0 || y < 0)

	// 1) Ensure columns is minimum one, if not, either
	// go to previous row, or go to previous group.
	if g.posX < 0 {
		if g.posY == 0 && reverse {
			g.posX = 0
			g.posY = 0

			return true, false
		}

		g.posY--
		g.posX = len(g.rows[g.posY]) - 1
	}

	// 2) If we are reverse-cycling and currently on the first candidate,
	// we are done with this group. Stay on those coordinates still.
	if g.posY < 0 {
		if g.posX == 0 {
			g.posX = 0
			g.posY = 0

			return true, false
		}

		g.posY = len(g.rows) - 1
		g.posX--
	}

	// 3) If we are on the last row, we might have to move to
	// the next column, if there is another one.
	if g.posY > g.maxY-1 {
		g.posY = 0
		if g.posX < g.maxX-1 {
			g.posX++
		} else {
			return true, true
		}
	}

	// 4) If we are on the last column, go to next row or next group
	if g.posX > len(g.rows[g.posY])-1 {
		if g.aliased {
			return g.findFirstCandidate(x, y)
		}

		g.posX = 0

		if g.posY < g.maxY-1 {
			g.posY++
		} else {
			return true, true
		}
	}

	// By default, come back to this group for next item.
	return false, false
}

// Check that there is indeed a completion in the column for a given row,
// otherwise loop in the direction wished until one is found, or go next/
// previous column, and so on.
func (g *group) findFirstCandidate(x, y int) (done, next bool) {
	for g.posX > len(g.rows[g.posY])-1 {
		g.posY += y
		g.posY += x

		// Previous column or group
		if g.posY < 0 {
			if g.posX == 0 {
				g.posX = 0
				g.posY = 0

				return true, false
			}

			g.posY = len(g.rows) - 1
			g.posX--
		}

		// Next column or group
		if g.posY > g.maxY-1 {
			g.posY = 0
			if g.posX < len(g.columnsWidth)-1 {
				g.posX++
			} else {
				return true, true
			}
		}
	}

	return
}

func (g *group) firstCell() {
	g.posX = 0
	g.posY = 0
}

func (g *group) lastCell() {
	g.posY = len(g.rows) - 1
	g.posX = len(g.columnsWidth) - 1

	if g.aliased {
		g.findFirstCandidate(0, -1)
	} else {
		g.posX = len(g.rows[g.posY]) - 1
	}
}

func (g *group) trimDisplay(comp Candidate, pad, col int) (candidate, padded string) {
	val := comp.Display
	if val == "" {
		return "", padSpace(pad)
	}

	maxDisplayWidth := g.columnsWidth[col]

	if maxDisplayWidth > g.termWidth {
		maxDisplayWidth = g.termWidth
	}

	if len(val) > g.columnsWidth[col] {
		val = val[:maxDisplayWidth-3] + "..."
		val = g.listSep() + sanitizer.Replace(val)

		return val, ""
	}

	return val, padSpace(pad)
}

func (g *group) trimDesc(val Candidate, pad int) (desc, padded string) {
	desc = val.Description
	if desc == "" {
		return desc, padSpace(pad)
	}

	if pad > g.maxDescWidth {
		pad = g.maxDescWidth - val.descLen
	}

	if len(desc) > g.maxDescWidth && g.maxDescWidth > 0 {
		desc = desc[:g.maxDescWidth-3] + "..."
		desc = g.listSep() + sanitizer.Replace(desc)

		return desc, ""
	}

	if len(desc)+pad > g.maxDescWidth {
		pad = g.maxDescWidth - len(desc)
	}

	desc = g.listSep() + sanitizer.Replace(desc)

	return desc, padSpace(pad)
}

func (g *group) setMaximumSizes(col int) int {
	// Get the length of the longest description in the same column.
	maxDescLen := g.descriptionsWidth[col]
	valuesRealLen := sum(g.columnsWidth) + len(g.columnsWidth) + len(g.listSep())

	if valuesRealLen+maxDescLen > g.termWidth {
		maxDescLen = g.termWidth - valuesRealLen
	} else if valuesRealLen+maxDescLen < g.termWidth {
		maxDescLen = g.termWidth - valuesRealLen
	}

	return maxDescLen
}

func (g *group) calculateMaxColumnWidths(grid [][]Candidate, numColumns int) {
	maxColumnWidths := make([]int, numColumns)
	maxDescWidths := make([]int, numColumns)

	for _, row := range grid {
		for columnIndex, value := range row {
			if value.displayLen+1 > maxColumnWidths[columnIndex] {
				maxColumnWidths[columnIndex] = value.displayLen + 1
			}

			if value.descLen > maxDescWidths[columnIndex] {
				maxDescWidths[columnIndex] = value.descLen + 1
			}
		}
	}

	g.maxY = len(g.rows)
	g.maxX = len(maxColumnWidths)
	g.columnsWidth = maxColumnWidths
	g.descriptionsWidth = maxDescWidths
}

func (g *group) longestValueDescribed(vals []Candidate) int {
	maxPairLength := 0

	descSeparatorLen := 1 + len(g.listSeparator) + 1

	for _, val := range vals {
		pairLength := val.displayLen

		if val.descLen > 0 {
			pairLength += val.descLen + descSeparatorLen
		}

		if pairLength > maxPairLength {
			maxPairLength = pairLength
		}
	}

	return maxPairLength
}

func (g *group) listSep() string {
	return g.listSeparator + " "
}

func completionsAreAliases(values []Candidate) bool {
	oddValueMap := make(map[string]bool)

	for i, value := range values {
		if i%2 == 0 && value.Description != "" {
			oddValueMap[value.Description] = true
		}
	}

	for i, value := range values {
		if i%2 != 0 && oddValueMap[value.Description] && value.Description != "" {
			return true
		}
	}

	return false
}

func createGrid(values []Candidate, rowCount, maxColumns int) [][]Candidate {
	grid := make([][]Candidate, rowCount)

	for i := 0; i < rowCount; i++ {
		grid[i] = createRow(values, maxColumns, i)
	}

	return grid
}

func createRow(domains []Candidate, maxColumns, rowIndex int) []Candidate {
	rowStart := rowIndex * maxColumns
	rowEnd := (rowIndex + 1) * maxColumns

	if rowEnd > len(domains) {
		rowEnd = len(domains)
	}

	return domains[rowStart:rowEnd]
}

func padSpace(times int) string {
	if times > 0 {
		return strings.Repeat(" ", times)
	}

	return ""
}

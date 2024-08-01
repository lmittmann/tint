package tint

import (
	"log/slog"

	"github.com/fatih/color"
)

// LevelColors defines the name as displayed to the user and color of a log level.
type LevelColor struct {
	// Name is the name of the log level
	Name string
	// Color is the color of the log level
	Color      color.Attribute
	serialized string
	colored    bool
}

// String returns the level name, optionally with color applied.
func (lc *LevelColor) String(colored bool) string {
	if len(lc.serialized) == 0 || lc.colored != colored {
		if colored {
			lc.serialized = color.New(lc.Color).SprintFunc()(lc.Name)
		} else {
			lc.serialized = lc.Name
		}
	}
	return lc.serialized
}

// Copy returns a copy of the LevelColor.
func (lc *LevelColor) Copy() *LevelColor {
	return &LevelColor{
		Name:       lc.Name,
		Color:      lc.Color,
		serialized: lc.serialized,
		colored:    lc.colored,
	}
}

// LevelColorsMapping is a map of log levels to their colors and is what
// the user defines in their configuration.
type LevelColorsMapping map[slog.Level]LevelColor

// min returns the mapped minimum index
func (lm *LevelColorsMapping) min() int {
	idx := 1000
	for check := range *lm {
		if int(check) < idx {
			idx = int(check)
		}
	}
	return idx
}

// size returns the size of the slice needed to store the LevelColors.
func (lm *LevelColorsMapping) size(offset int) int {
	maxIdx := -1000
	for check := range *lm {
		if int(check) > maxIdx {
			maxIdx = int(check)
		}
	}
	return offset + maxIdx + 1
}

// offset returns the index offset needed to map negative log levels.
func (lm *LevelColorsMapping) offset() int {
	min := lm.min()
	if min < 0 {
		min = -min
	}
	return min
}

// LevelColors returns the LevelColors for the LevelColorsMapping.
func (lm *LevelColorsMapping) LevelColors() *LevelColors {
	lcList := make([]*LevelColor, lm.size(lm.offset()))
	for idx, lc := range *lm {
		lcList[int(idx)+lm.offset()] = lc.Copy()
	}
	lc := LevelColors{
		levels: lcList,
		offset: lm.offset(),
	}
	return &lc
}

// LevelColors is our internal representation of the user-defined LevelColorsMapping.
// We map the log levels via their slog.Level to their LevelColor using an offset
// to ensure we can map negative level values to our slice.
type LevelColors struct {
	levels []*LevelColor
	offset int
}

// LevelColor returns the LevelColor for the given log level.
// Returns nil indicating if the log level was not found.
func (lc *LevelColors) LevelColor(level slog.Level) *LevelColor {
	if len(lc.levels) == 0 {
		return nil
	}

	idx := int(level.Level()) + lc.offset
	if len(lc.levels) < idx {
		return &LevelColor{}
	}
	return lc.levels[idx]
}

// Copy returns a copy of the LevelColors.
func (lc *LevelColors) Copy() *LevelColors {
	if len(lc.levels) == 0 {
		return &LevelColors{
			levels: []*LevelColor{},
		}
	}

	lcCopy := LevelColors{
		levels: make([]*LevelColor, len(lc.levels)),
		offset: lc.offset,
	}
	copy(lcCopy.levels, lc.levels)
	return &lcCopy
}

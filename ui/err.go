package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type ErrBox struct {
	height, width int
	err           error
	// info is a non-error notice, shown when there is no error.
	info string
}

var errStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#FF0000",
	Dark:  "#FF0000",
})

var infoStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#B8860B",
	Dark:  "#E5C07B",
})

func NewErrBox() *ErrBox {
	return &ErrBox{}
}

func (e *ErrBox) SetError(err error) {
	e.err = err
}

// SetInfo shows a non-error notice. An error, if set, takes precedence.
func (e *ErrBox) SetInfo(info string) {
	e.info = info
}

func (e *ErrBox) Clear() {
	e.err = nil
	e.info = ""
}

func (e *ErrBox) SetSize(width, height int) {
	e.width = width
	e.height = height
}

func (e *ErrBox) String() string {
	var text string
	style := errStyle
	if e.err != nil {
		text = e.err.Error()
	} else if e.info != "" {
		text = e.info
		style = infoStyle
	}
	if text != "" {
		lines := strings.Split(text, "\n")
		text = strings.Join(lines, "//")
		if runewidth.StringWidth(text) > e.width-3 && e.width-3 >= 0 {
			text = runewidth.Truncate(text, e.width-3, "...")
		}
	}
	return lipgloss.Place(e.width, e.height, lipgloss.Center, lipgloss.Center, style.Render(text))
}

package tmux

import (
	"bytes"
	"claude-squad/log"
	"os/exec"
	"strconv"
	"unicode/utf8"
)

// While attached, the session's status line sits at the top of the terminal and starts
// with a "← back" button. Clicking it detaches, like ctrl+q. tmux only draws the button;
// cs reads the click from the terminal's mouse reports before they reach tmux.

// backLabel is the button's text, drawn at the left of the status line.
const backLabel = " ← back "

// backButtonCols is how many columns from the left a click detaches on. It's one wider
// than the label, in case the terminal draws "←" two columns wide.
var backButtonCols = utf8.RuneCountInString(backLabel) + 1

// showBackButton puts the status line at the top of the attached session with the back
// button on its left, replacing the window list, which only ever has one window.
func (t *TmuxSession) showBackButton() {
	set := func(option, value string) []string {
		return []string{"set-option", "-t", t.sanitizedName, option, value}
	}
	var args []string
	for i, opt := range [][]string{
		set("status", "on"),
		set("status-position", "top"),
		set("status-left", "#[reverse,bold]"+backLabel+"#[default] "),
		set("status-left-length", strconv.Itoa(backButtonCols+1)),
		set("window-status-format", ""),
		set("window-status-current-format", ""),
	} {
		if i > 0 {
			args = append(args, ";")
		}
		args = append(args, opt...)
	}
	if err := t.cmdExec.Run(exec.Command("tmux", args...)); err != nil {
		log.InfoLog.Printf("could not show the back button for session %s: %v", t.sanitizedName, err)
	}
}

// sgrMousePrefix starts an SGR mouse report: ESC [ < button ; column ; row, then M for a
// press or motion, m for a release.
var sgrMousePrefix = []byte("\x1b[<")

// maxMouseReport bounds an incomplete report held back for the next read.
const maxMouseReport = 32

// mouseReport is one parsed SGR mouse report. Columns and rows count from 1.
type mouseReport struct {
	button   int
	col, row int
	release  bool
}

func (r mouseReport) onBackButton() bool { return r.row == 1 && r.col >= 1 && r.col <= backButtonCols }
func (r mouseReport) motion() bool       { return r.button&32 != 0 }

// leftPress reports whether r is a left-button press, with or without modifier keys.
func (r mouseReport) leftPress() bool {
	return !r.release && !r.motion() && r.button&64 == 0 && r.button&3 == 0
}

// parseMouseReport parses the report at the start of b, which starts with sgrMousePrefix.
// n is the report's length; it's 0 if b ends before the report does, and -1 if b
// doesn't hold a report.
func parseMouseReport(b []byte) (r mouseReport, n int) {
	var fields [3]int
	field := 0
	digits := 0
	for i := len(sgrMousePrefix); i < len(b); i++ {
		c := b[i]
		switch {
		case c >= '0' && c <= '9':
			fields[field] = fields[field]*10 + int(c-'0')
			digits++
			if digits > 5 {
				return r, -1
			}
		case c == ';' && field < 2 && digits > 0:
			field++
			digits = 0
		case (c == 'M' || c == 'm') && field == 2 && digits > 0:
			return mouseReport{button: fields[0], col: fields[1], row: fields[2], release: c == 'm'}, i + 1
		default:
			return r, -1
		}
	}
	if len(b) >= maxMouseReport {
		return r, -1
	}
	return r, 0
}

// attachInput filters what's typed while attached: it finds ctrl+q and clicks on the
// back button, and passes everything else on to the session.
type attachInput struct {
	// pending is the start of a mouse report the last read ended in the middle of. A read
	// that ends inside the "ESC [ <" prefix isn't held back, since a trailing ESC is
	// usually the Esc key; such a split click just reaches tmux, where it does nothing.
	pending []byte
	// pressed is set while the left button, pressed on the back button, is held down.
	pressed bool
}

// filter returns the part of input to forward to the session and whether to detach.
// A click on the back button detaches when the button is released over it, like a
// regular button; the press, the drag and the release aren't forwarded.
func (a *attachInput) filter(input []byte) (forward []byte, detach bool) {
	data := append(a.pending, input...)
	a.pending = nil
	data, detach = splitAtDetachKey(data)

	forward = make([]byte, 0, len(data))
	for len(data) > 0 {
		i := bytes.Index(data, sgrMousePrefix)
		if i < 0 {
			forward = append(forward, data...)
			break
		}
		forward = append(forward, data[:i]...)
		data = data[i:]

		r, n := parseMouseReport(data)
		switch {
		case n == 0:
			if !detach {
				a.pending = append([]byte(nil), data...)
			}
			return forward, detach
		case n < 0:
			forward = append(forward, data[:len(sgrMousePrefix)]...)
			data = data[len(sgrMousePrefix):]
			continue
		}

		report := data[:n]
		data = data[n:]
		switch {
		case a.pressed && r.release:
			a.pressed = false
			if r.onBackButton() {
				return forward, true
			}
		case a.pressed && r.motion():
		case r.leftPress() && r.onBackButton():
			a.pressed = true
		default:
			forward = append(forward, report...)
		}
	}
	return forward, detach
}

package ui

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// ErrCancelled is returned when the person leaves a prompt with Esc or Ctrl+C.
var ErrCancelled = errors.New("cancelled")

// Terminal is what the prompts need: keys in, drawing out, raw mode while a prompt runs, and the screen's size.
type Terminal struct {
	In            io.Reader
	Out           io.Writer
	Raw           func() (restore func(), err error)
	Width, Height int
	pending       []key // keys read but not used yet, for the next prompt
}

type keyKind int

const (
	keyText keyKind = iota
	keyUp
	keyDown
	keyEnter
	keyBackspace
	keyCancel
	keyOther
)

type key struct {
	kind keyKind
	text string
}

// parseKeys splits what the terminal sent into keys: arrows, Enter, Backspace, Esc, Ctrl+C and typed text.
func parseKeys(data []byte) []key {
	var keys []key
	for len(data) > 0 {
		switch b := data[0]; {
		case b == 0x1b && len(data) >= 3 && (data[1] == '[' || data[1] == 'O'):
			end := 2
			for end < len(data) && (data[end] < 0x40 || data[end] > 0x7e) {
				end++
			}
			kind := keyOther
			if end < len(data) {
				switch data[end] {
				case 'A':
					kind = keyUp
				case 'B':
					kind = keyDown
				}
			}
			keys = append(keys, key{kind: kind})
			data = data[min(end+1, len(data)):]
			continue
		case b == 0x1b || b == 3 || b == 4:
			keys = append(keys, key{kind: keyCancel})
		case b == '\r' || b == '\n':
			keys = append(keys, key{kind: keyEnter})
			if b == '\r' && len(data) > 1 && data[1] == '\n' {
				data = data[1:]
			}
		case b == 0x7f || b == 8:
			keys = append(keys, key{kind: keyBackspace})
		case b == 16:
			keys = append(keys, key{kind: keyUp})
		case b == 14 || b == '\t':
			keys = append(keys, key{kind: keyDown})
		case b < 0x20:
			keys = append(keys, key{kind: keyOther})
		default:
			r, size := utf8.DecodeRune(data)
			keys = append(keys, key{kind: keyText, text: string(r)})
			data = data[size:]
			continue
		}
		data = data[1:]
	}
	return keys
}

func (t *Terminal) next() (key, error) {
	for len(t.pending) == 0 {
		buffer := make([]byte, 256)
		n, err := t.In.Read(buffer)
		t.pending = append(t.pending, parseKeys(buffer[:n])...)
		if err != nil && len(t.pending) == 0 {
			if err == io.EOF {
				return key{kind: keyCancel}, nil
			}
			return key{}, err
		}
	}
	next := t.pending[0]
	t.pending = t.pending[1:]
	return next, nil
}

// session puts the terminal in raw mode and hides the cursor until the returned function runs.
func (t *Terminal) session() (func(), error) {
	restore := func() {}
	if t.Raw != nil {
		var err error
		if restore, err = t.Raw(); err != nil {
			return nil, err
		}
	}
	fmt.Fprint(t.Out, "\x1b[?25l")
	return func() {
		fmt.Fprint(t.Out, "\x1b[?25h")
		restore()
	}, nil
}

// Choice is one line in a picker.
type Choice struct {
	Label string
	Hint  string
}

func matches(choice Choice, filter string) bool {
	filter = strings.ToLower(filter)
	return strings.Contains(strings.ToLower(choice.Label), filter) || strings.Contains(strings.ToLower(choice.Hint), filter)
}

// Pick lets the person choose with the arrow keys, or narrow the list by typing. It returns the chosen index.
func (s Style) Pick(t *Terminal, title string, choices []Choice) (int, error) {
	done, err := t.session()
	if err != nil {
		return -1, err
	}
	defer done()
	filter, cursor, offset, drawn := "", 0, 0, 0
	labelWidth := 0
	for _, choice := range choices {
		labelWidth = max(labelWidth, utf8.RuneCountInString(choice.Label))
	}
	rows := max(3, min(12, t.Height-4))
	for {
		var shown []int
		for i, choice := range choices {
			if matches(choice, filter) {
				shown = append(shown, i)
			}
		}
		cursor = max(0, min(cursor, len(shown)-1))
		offset = max(min(offset, cursor), cursor-rows+1)
		lines := []string{s.Bold(title) + "  " + s.Dim("↑↓ move · type to filter · enter choose · esc cancel")}
		if filter != "" {
			lines = append(lines, s.Dim("Filter: ")+filter)
		}
		if len(shown) == 0 {
			lines = append(lines, s.Dim("  No match"))
		}
		for row := offset; row < len(shown) && row < offset+rows; row++ {
			choice := choices[shown[row]]
			text := choice.Label + strings.Repeat(" ", labelWidth-utf8.RuneCountInString(choice.Label))
			if row == cursor {
				line := truncate("› "+text+"  "+choice.Hint, t.Width-1)
				if s.on {
					line = s.paint("7", line+strings.Repeat(" ", max(0, min(t.Width-1, labelWidth+60)-Width(line))))
				}
				lines = append(lines, line)
			} else {
				lines = append(lines, "  "+text+"  "+s.Dim(truncate(choice.Hint, t.Width-labelWidth-5)))
			}
		}
		if hidden := len(shown) - offset - rows; hidden > 0 {
			lines = append(lines, s.Dim(fmt.Sprintf("  ↓ %d more", hidden)))
		}
		drawn = t.redraw(lines, drawn)

		k, err := t.next()
		if err != nil {
			return -1, err
		}
		switch k.kind {
		case keyUp:
			cursor = (cursor - 1 + max(1, len(shown))) % max(1, len(shown))
		case keyDown:
			cursor = (cursor + 1) % max(1, len(shown))
		case keyText:
			filter += k.text
			cursor, offset = 0, 0
		case keyBackspace:
			if filter != "" {
				_, size := utf8.DecodeLastRuneInString(filter)
				filter = filter[:len(filter)-size]
			}
		case keyCancel:
			t.redraw(nil, drawn)
			return -1, ErrCancelled
		case keyEnter:
			if len(shown) > 0 {
				t.redraw([]string{s.Done(s.Dim(title) + " " + choices[shown[cursor]].Label)}, drawn)
				return shown[cursor], nil
			}
		}
	}
}

// Ask reads one line of text. An empty answer is allowed; the caller decides what it means.
func (s Style) Ask(t *Terminal, label string) (string, error) {
	done, err := t.session()
	if err != nil {
		return "", err
	}
	defer done()
	value, drawn := "", 0
	for {
		drawn = t.redraw([]string{s.Bold(label) + " " + s.Accent("›") + " " + value + s.paint("7", " ")}, drawn)
		k, err := t.next()
		if err != nil {
			return "", err
		}
		switch k.kind {
		case keyText:
			value += k.text
		case keyBackspace:
			if value != "" {
				_, size := utf8.DecodeLastRuneInString(value)
				value = value[:len(value)-size]
			}
		case keyCancel:
			t.redraw(nil, drawn)
			return "", ErrCancelled
		case keyEnter:
			t.redraw([]string{s.Done(s.Dim(label) + " " + value)}, drawn)
			return value, nil
		}
	}
}

// redraw replaces the lines drawn before with these, and returns how many lines it drew. In raw mode a new line
// needs a carriage return too.
func (t *Terminal) redraw(lines []string, before int) int {
	var out strings.Builder
	if before > 0 {
		fmt.Fprintf(&out, "\x1b[%dF", before)
	}
	out.WriteString("\r\x1b[J")
	for _, line := range lines {
		out.WriteString(line + "\r\n")
	}
	fmt.Fprint(t.Out, out.String())
	return len(lines)
}

func truncate(text string, width int) string {
	if width < 2 || utf8.RuneCountInString(text) <= width {
		return text
	}
	return string([]rune(text)[:width-1]) + "…"
}

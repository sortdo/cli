package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A JSON object with its keys in the server's order.
type object []field

type field struct {
	key   string
	value any
}

func decode(d *json.Decoder) (any, error) {
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	if delim == '[' {
		list := []any{}
		for d.More() {
			value, err := decode(d)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		_, err = d.Token()
		return list, err
	}
	fields := object{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, err
		}
		value, err := decode(d)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field{key.(string), value})
	}
	_, err = d.Token()
	return fields, err
}

func parse(text string) (any, bool) {
	text = strings.TrimSpace(text)
	if text == "" || (text[0] != '{' && text[0] != '[') {
		return nil, false
	}
	d := json.NewDecoder(strings.NewReader(text))
	d.UseNumber()
	value, err := decode(d)
	if err != nil {
		return nil, false
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, false
	}
	return value, true
}

// Render shows a tool's JSON result as a person reads it: lists of items as tables, the rest as "Label: value" lines.
// It returns false for text that is not JSON, which is shown as it is. A section named like title (the tool's name,
// shown on the frame around the result) loses its heading.
func (s Style) Render(text string, width int, title string) (string, bool) {
	value, ok := parse(text)
	if !ok {
		return "", false
	}
	r := renderer{style: s, width: max(width, 40), title: title}
	r.value(value, 0)
	return strings.TrimRight(r.out.String(), "\n") + "\n", true
}

type renderer struct {
	style Style
	width int
	title string
	out   bytes.Buffer
}

func (r *renderer) line(indent int, text string) {
	r.out.WriteString(strings.Repeat(" ", indent) + text + "\n")
}

func (r *renderer) value(value any, indent int) {
	switch v := value.(type) {
	case object:
		r.object(v, indent)
	case []any:
		r.list(v, indent)
	default:
		r.line(indent, scalar(r.style, v))
	}
}

func (r *renderer) object(fields object, indent int) {
	labelWidth := 0
	for _, f := range fields {
		if isInline(f.value) {
			labelWidth = max(labelWidth, utf8.RuneCountInString(label(f.key)))
		}
	}
	block := false // the field before was a block: a blank line follows it
	for i, f := range fields {
		if isInline(f.value) {
			if block {
				r.out.WriteString("\n")
			}
			block = false
			name := label(f.key) + ":" + strings.Repeat(" ", labelWidth-utf8.RuneCountInString(label(f.key))+1)
			r.line(indent, r.style.Dim(name)+r.inline(f.value))
			continue
		}
		if i > 0 {
			r.out.WriteString("\n")
		}
		block = true
		if indent == 0 && r.title != "" && strings.EqualFold(label(f.key), r.title) {
			r.value(f.value, 0)
			continue
		}
		r.line(indent, r.style.Bold(label(f.key)))
		if text, ok := f.value.(string); ok {
			for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
				r.line(indent+2, line)
			}
			continue
		}
		r.value(f.value, indent+2)
	}
}

func (r *renderer) list(items []any, indent int) {
	if len(items) == 0 {
		r.line(indent, r.style.Dim("None"))
		return
	}
	if rows, ok := objects(items); ok {
		if r.table(rows, indent) {
			return
		}
		for i, row := range rows {
			if i > 0 {
				r.out.WriteString("\n")
			}
			r.item(row, indent)
		}
		return
	}
	for _, item := range items {
		if isInline(item) {
			r.line(indent, r.style.Dim("•")+" "+r.inline(item))
		} else {
			r.value(item, indent+2)
		}
	}
}

// table prints items as rows. It returns false when the table cannot fit, and the items are then listed one by one.
func (r *renderer) table(rows []object, indent int) bool {
	// Every key once, in the server's order: a key that only later rows have goes after the key before it.
	var keys []string
	for _, row := range rows {
		after := -1
		for _, f := range row {
			at := slices.Index(keys, f.key)
			if at < 0 {
				at = after + 1
				keys = slices.Insert(keys, at, f.key)
			}
			after = at
		}
	}
	// IDs go last: people copy them into the next command, so they are never shortened.
	var columns, ids []string
	for _, key := range keys {
		if isID(key) {
			ids = append(ids, key)
		} else {
			columns = append(columns, key)
		}
	}
	columns = append(columns, ids...)
	cells := make([][]string, len(rows))
	widths := make([]int, len(columns))
	for c, key := range columns {
		widths[c] = utf8.RuneCountInString(label(key))
	}
	for i, row := range rows {
		cells[i] = make([]string, len(columns))
		for c, key := range columns {
			if value, ok := row.get(key); ok {
				cells[i][c] = strings.Join(strings.Fields(cellText(value)), " ")
			}
			widths[c] = max(widths[c], utf8.RuneCountInString(cells[i][c]))
		}
	}
	// Shorten the widest text columns until the table fits, down to 20 characters each.
	const gap, shortest = 2, 20
	total := func() int {
		sum := indent + gap*(len(widths)-1)
		for _, w := range widths {
			sum += w
		}
		return sum
	}
	for total() > r.width {
		widest := -1
		for c, key := range columns {
			if !isID(key) && widths[c] > shortest && (widest < 0 || widths[c] > widths[widest]) {
				widest = c
			}
		}
		if widest < 0 {
			return false
		}
		widths[widest] = max(shortest, widths[widest]-(total()-r.width))
	}
	pad := func(text string, width int) string {
		if utf8.RuneCountInString(text) > width {
			text = string([]rune(text)[:width-1]) + "…"
		}
		return text + strings.Repeat(" ", width-utf8.RuneCountInString(text))
	}
	header := make([]string, len(columns))
	for c, key := range columns {
		header[c] = r.style.Dim(pad(label(key), widths[c]))
	}
	r.line(indent, strings.TrimRight(strings.Join(header, strings.Repeat(" ", gap)), " "))
	for _, row := range cells {
		line := make([]string, len(columns))
		for c, text := range row {
			line[c] = pad(text, widths[c])
			if isID(columns[c]) {
				line[c] = r.style.Dim(line[c])
			} else if colored := r.style.Status(text); colored != text {
				line[c] = colored + strings.Repeat(" ", widths[c]-utf8.RuneCountInString(text))
			}
		}
		r.line(indent, strings.TrimRight(strings.Join(line, strings.Repeat(" ", gap)), " "))
	}
	return true
}

// item lists one item that does not fit in a table: its name as a heading, then its other fields.
func (r *renderer) item(row object, indent int) {
	for i, f := range row {
		if name, ok := f.value.(string); ok && (f.key == "name" || f.key == "title") && !strings.Contains(name, "\n") {
			r.line(indent, r.style.Bold(name))
			r.object(append(append(object{}, row[:i]...), row[i+1:]...), indent+2)
			return
		}
	}
	r.object(row, indent)
}

func (o object) get(key string) (any, bool) {
	for _, f := range o {
		if f.key == key {
			return f.value, true
		}
	}
	return nil, false
}

func objects(items []any) ([]object, bool) {
	rows := make([]object, 0, len(items))
	for _, item := range items {
		row, ok := item.(object)
		if !ok {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

// isInline is a value that fits after its label: a one-line text, a number, yes or no, or a short list of those.
func isInline(value any) bool {
	switch v := value.(type) {
	case object:
		return len(v) == 0 || flags(v) != ""
	case []any:
		for _, item := range v {
			if _, nested := item.(object); nested {
				return false
			}
			if _, nested := item.([]any); nested {
				return false
			}
		}
		return len(v) <= 8
	case string:
		return !strings.Contains(v, "\n")
	}
	return true
}

func (r *renderer) inline(value any) string {
	switch v := value.(type) {
	case object:
		if len(v) == 0 {
			return r.style.Dim("None")
		}
		return flags(v)
	case []any:
		if len(v) == 0 {
			return r.style.Dim("None")
		}
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = scalar(r.style, item)
		}
		return strings.Join(parts, ", ")
	}
	return scalar(r.style, value)
}

// cellText is a value in one table cell, without colors.
func cellText(value any) string {
	switch v := value.(type) {
	case object:
		if text := flags(v); text != "" {
			return text
		}
		parts := make([]string, 0, len(v))
		for _, f := range v {
			parts = append(parts, label(f.key)+": "+cellText(f.value))
		}
		return strings.Join(parts, ", ")
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = cellText(item)
		}
		return strings.Join(parts, ", ")
	}
	return scalar(Style{}, value)
}

// flags shows an object of yes/no values, such as access rights, as the names that are yes.
func flags(o object) string {
	var on []string
	for _, f := range o {
		yes, ok := f.value.(bool)
		if !ok {
			return ""
		}
		if yes {
			on = append(on, strings.ToLower(label(f.key)))
		}
	}
	if len(on) == 0 {
		return "none"
	}
	return strings.Join(on, ", ")
}

func scalar(s Style, value any) string {
	switch v := value.(type) {
	case nil:
		return s.Dim("—")
	case bool:
		if v {
			return s.Status("yes")
		}
		return s.Status("no")
	case json.Number:
		return v.String()
	case string:
		return s.Status(v)
	}
	return ""
}

func isID(key string) bool {
	return key == "id" || strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "ID")
}

// label turns a JSON key into words: memberCount is "Member count", pageId is "Page ID".
func label(key string) string {
	var words []string
	start := 0
	runes := []rune(key)
	for i := 1; i <= len(runes); i++ {
		if i == len(runes) || unicode.IsUpper(runes[i]) && !unicode.IsUpper(runes[i-1]) || runes[i] == '_' {
			if word := strings.Trim(string(runes[start:i]), "_"); word != "" {
				words = append(words, strings.ToLower(word))
			}
			start = i
		}
	}
	for i, word := range words {
		switch word {
		case "id", "url", "ai", "mcp", "api":
			words[i] = strings.ToUpper(word)
		}
	}
	if len(words) == 0 {
		return key
	}
	words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]
	return strings.Join(words, " ")
}

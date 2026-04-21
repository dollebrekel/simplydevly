// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const (
	keyLeft  = "left"
	keyRight = "right"
)

// FormField is the interface all form field types implement.
type FormField interface {
	Label() string
	Render(width int, focused bool, theme Theme, config RenderConfig) string
	HandleKey(key string) bool
	Value() string
}

// TextField is a single-line text input form field.
type TextField struct {
	label       string
	value       string
	placeholder string
	cursor      int
	validate    func(string) bool
}

// NewTextField creates a single-line text input field.
func NewTextField(label, placeholder string, validate func(string) bool) *TextField {
	return &TextField{label: label, placeholder: placeholder, validate: validate}
}

func (f *TextField) Label() string { return f.label }

func (f *TextField) Value() string { return f.value }

// Validate runs the validation function (if set) against the current value.
// Returns true if valid or no validator is set.
func (f *TextField) Validate() bool {
	if f.validate == nil {
		return true
	}
	return f.validate(f.value)
}

func (f *TextField) Render(width int, focused bool, theme Theme, config RenderConfig) string {
	if width < 1 {
		width = 1
	}
	cs := config.Color
	labelStyle := theme.TextMuted.Resolve(cs)
	prefix := labelStyle.Render(f.label+": ") + " "
	prefixW := ansi.StringWidth(prefix)
	inputW := width - prefixW
	if inputW < 1 {
		inputW = 1
	}

	var text string
	if f.value == "" {
		muted := theme.TextMuted.Resolve(cs)
		text = muted.Render(ansi.Truncate(f.placeholder, inputW, "…"))
	} else {
		runes := []rune(f.value)
		before := string(runes[:f.cursor])
		var after string
		if f.cursor < len(runes) {
			after = string(runes[f.cursor:])
		}
		raw := before
		if focused {
			raw += "█"
		}
		raw += after
		text = ansi.Truncate(raw, inputW, "…")
	}

	line := prefix + text
	if focused {
		focusStyle := theme.Primary.Resolve(cs)
		return focusStyle.Render(ansi.Strip(line))
	}
	return line
}

func (f *TextField) HandleKey(key string) bool {
	runes := []rune(f.value)
	switch key {
	case "backspace", "ctrl+h":
		if f.cursor > 0 {
			f.value = string(runes[:f.cursor-1]) + string(runes[f.cursor:])
			f.cursor--
			return true
		}
	case keyLeft:
		if f.cursor > 0 {
			f.cursor--
			return true
		}
	case keyRight:
		if f.cursor < len(runes) {
			f.cursor++
			return true
		}
	default:
		if utf8.RuneCountInString(key) == 1 {
			r, _ := utf8.DecodeRuneInString(key)
			if r >= 32 {
				f.value = string(runes[:f.cursor]) + string(r) + string(runes[f.cursor:])
				f.cursor++
				return true
			}
		}
	}
	return false
}

// SelectField is a dropdown-style choice field.
type SelectField struct {
	label    string
	options  []string
	selected int
}

// NewSelectField creates a select field with the given options.
func NewSelectField(label string, options []string) *SelectField {
	return &SelectField{label: label, options: options}
}

func (f *SelectField) Label() string { return f.label }

func (f *SelectField) Value() string {
	if len(f.options) == 0 {
		return ""
	}
	return f.options[f.selected]
}

func (f *SelectField) Render(width int, focused bool, theme Theme, config RenderConfig) string {
	if width < 1 {
		width = 1
	}
	cs := config.Color
	labelStyle := theme.TextMuted.Resolve(cs)
	prefix := labelStyle.Render(f.label + ": ")

	val := "<none>"
	if len(f.options) > 0 {
		val = f.options[f.selected]
	}
	line := prefix + "< " + val + " >"
	line = ansi.Truncate(line, width, "…")
	if focused {
		return theme.Primary.Resolve(cs).Render(ansi.Strip(line))
	}
	return line
}

func (f *SelectField) HandleKey(key string) bool {
	if len(f.options) == 0 {
		return false
	}
	switch key {
	case keyLeft, "h":
		if f.selected > 0 {
			f.selected--
			return true
		}
	case keyRight, "l":
		if f.selected < len(f.options)-1 {
			f.selected++
			return true
		}
	}
	return false
}

// CheckboxField is a boolean checkbox form field.
type CheckboxField struct {
	label   string
	checked bool
}

// NewCheckboxField creates a checkbox field.
func NewCheckboxField(label string, checked bool) *CheckboxField {
	return &CheckboxField{label: label, checked: checked}
}

func (f *CheckboxField) Label() string { return f.label }

func (f *CheckboxField) Value() string {
	if f.checked {
		return "true"
	}
	return "false"
}

func (f *CheckboxField) Render(width int, focused bool, theme Theme, config RenderConfig) string {
	if width < 1 {
		width = 1
	}
	cs := config.Color
	var box string
	if f.checked {
		box = "[x]"
	} else {
		box = "[ ]"
	}
	line := box + " " + f.label
	line = ansi.Truncate(line, width, "…")
	if focused {
		return theme.Primary.Resolve(cs).Render(ansi.Strip(line))
	}
	return line
}

func (f *CheckboxField) HandleKey(key string) bool {
	if key == " " || key == "enter" {
		f.checked = !f.checked
		return true
	}
	return false
}

// Form is a composable collection of form fields with keyboard navigation.
type Form struct {
	fields       []FormField
	focused      int
	theme        Theme
	renderConfig RenderConfig
}

// NewForm creates a Form with the given fields, theme, and render config.
func NewForm(fields []FormField, theme Theme, config RenderConfig) *Form {
	return &Form{fields: fields, theme: theme, renderConfig: config}
}

// Render renders all fields vertically, highlighting the focused field.
func (f *Form) Render(width int) string {
	if width < 1 {
		width = 1
	}
	var b strings.Builder
	for i, field := range f.fields {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(field.Render(width, i == f.focused, f.theme, f.renderConfig))
	}
	return b.String()
}

// HandleKey processes tab/shift+tab for field navigation, delegates rest to focused field.
func (f *Form) HandleKey(key string) bool {
	switch key {
	case "tab":
		if f.focused < len(f.fields)-1 {
			f.focused++
		} else {
			f.focused = 0
		}
		return true
	case "shift+tab":
		if f.focused > 0 {
			f.focused--
		} else {
			f.focused = len(f.fields) - 1
		}
		return true
	}
	if len(f.fields) == 0 {
		return false
	}
	return f.fields[f.focused].HandleKey(key)
}

// Values collects all field values by label.
func (f *Form) Values() map[string]string {
	result := make(map[string]string, len(f.fields))
	for _, field := range f.fields {
		result[field.Label()] = field.Value()
	}
	return result
}

// FocusedIndex returns the index of the currently focused field.
func (f *Form) FocusedIndex() int { return f.focused }

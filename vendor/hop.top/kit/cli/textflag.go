package cli

import "strings"

// TextFlag is a pflag.Value that supports +append, ^prepend, =replace
// semantics for text-valued flags.
//
//	--desc "new"        replace (default)
//	--desc ="new"       replace (explicit)
//	--desc +"line"      append on new line
//	--desc +="inline"   append inline (no newline)
//	--desc ^"line"      prepend on new line
//	--desc ^="inline"   prepend inline (no newline)
//	--desc =            clear
type TextFlag struct {
	text string
}

// Set implements pflag.Value.
func (tf *TextFlag) Set(val string) error {
	if val == "" {
		tf.text = ""
		return nil
	}

	switch {
	case strings.HasPrefix(val, "+="):
		tf.text += val[2:]
	case val[0] == '+':
		body := val[1:]
		if tf.text == "" {
			tf.text = body
		} else {
			tf.text += "\n" + body
		}
	case strings.HasPrefix(val, "^="):
		tf.text = val[2:] + tf.text
	case val[0] == '^':
		body := val[1:]
		if tf.text == "" {
			tf.text = body
		} else {
			tf.text = body + "\n" + tf.text
		}
	case val[0] == '=':
		tf.text = val[1:]
	default:
		tf.text = val
	}
	return nil
}

// Append adds val on a new line (no prefix interpretation).
func (tf *TextFlag) Append(val string) {
	if tf.text == "" {
		tf.text = val
	} else {
		tf.text += "\n" + val
	}
}

// AppendInline concatenates val directly (no prefix interpretation).
func (tf *TextFlag) AppendInline(val string) { tf.text += val }

// Prepend adds val before existing text on a new line (no prefix interpretation).
func (tf *TextFlag) Prepend(val string) {
	if tf.text == "" {
		tf.text = val
	} else {
		tf.text = val + "\n" + tf.text
	}
}

// PrependInline concatenates val before existing text (no prefix interpretation).
func (tf *TextFlag) PrependInline(val string) { tf.text = val + tf.text }

// String implements pflag.Value.
func (tf *TextFlag) String() string { return tf.text }

// Type implements pflag.Value.
func (tf *TextFlag) Type() string { return "text" }

// Value returns the current text.
func (tf *TextFlag) Value() string { return tf.text }

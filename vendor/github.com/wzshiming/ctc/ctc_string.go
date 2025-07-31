package ctc

import (
	"bytes"
	"strconv"
	"strings"
	"unsafe"

	_ "github.com/wzshiming/winseq" // Use Unix like Sequences in Windows
)

var cc = []string{
	"Black",
	"Red",
	"Green",
	"Yellow",
	"Blue",
	"Magenta",
	"Cyan",
	"White",
}

var pre = []byte("\x1b[0")

// String returns UnixLike markup
func (c Color) String() string {
	b := c.Bytes()
	return *(*string)(unsafe.Pointer(&b))
}

// Bytes returns UnixLike markup
func (c Color) Bytes() []byte {
	s := make([]byte, 0, 16)
	s = append(s, pre...)
	if c&applyForeground == applyForeground {
		if c&ForegroundBright == ForegroundBright {
			s = appendColor(s, uint8(c&foregroundMask), 90)
		} else {
			s = appendColor(s, uint8(c&foregroundMask), 30)
		}
	}
	if c&applyBackground == applyBackground {
		if c&BackgroundBright == BackgroundBright {
			s = appendColor(s, uint8(c&backgroundMask>>4), 100)
		} else {
			s = appendColor(s, uint8(c&backgroundMask>>4), 40)
		}
	}
	if c&Underline == Underline {
		s = append(s, ';', '4')
	}
	if c&Negative == Negative {
		s = append(s, ';', '7')
	}
	s = append(s, 'm')

	return s
}

// Name returns color name
func (c Color) Name() string {
	if c&(applyForeground|applyBackground) == 0 {
		return "Reset"
	}

	buf := bytes.NewBuffer(nil)
	if c&applyForeground == applyForeground {
		buf.WriteString("Foreground")
		if c&ForegroundBright == ForegroundBright {
			buf.WriteString("Bright")
		}
		buf.WriteString(cc[int(c&foregroundMask&white)])
	}
	if c&applyBackground == applyBackground {
		buf.WriteString("Background")
		if c&BackgroundBright == BackgroundBright {
			buf.WriteString("Bright")
		}
		buf.WriteString(cc[int(((c&backgroundMask)>>4)&white)])
	}
	if c&Underline == Underline {
		buf.WriteString("Underline")
	}
	if c&Negative == Negative {
		buf.WriteString("Negative")
	}
	return buf.String()
}

// Info returns color details info
func (c Color) Info() string {
	if c&(applyForeground|applyBackground) == 0 {
		return "Reset"
	}

	ss := []string{}
	if c&applyForeground == applyForeground {
		if c&ForegroundBright == ForegroundBright {
			ss = append(ss, "ForegroundBright")
		}
		ss = append(ss, "Foreground"+cc[int(c&foregroundMask&white)])
	}
	if c&applyBackground == applyBackground {
		if c&BackgroundBright == BackgroundBright {
			ss = append(ss, "BackgroundBright")
		}
		ss = append(ss, "Background"+cc[int(((c&backgroundMask)>>4)&white)])
	}
	if c&Underline == Underline {
		ss = append(ss, "Underline")
	}
	if c&Negative == Negative {
		ss = append(ss, "Negative")
	}
	return strings.Join(ss, " | ")
}

func appendColor(s []byte, c uint8, off uint8) []byte {
	s = append(s, ';')
	n := uint64(off + c&uint8(white))
	s = strconv.AppendUint(s, n, 10)
	return s
}

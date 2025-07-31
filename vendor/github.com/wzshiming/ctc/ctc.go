package ctc

// Color color sum
type Color uint64

// Color constant
const (
	Reset Color = 0

	ForegroundBright  Color = applyForeground | bright
	ForegroundBlack   Color = applyForeground | black
	ForegroundRed     Color = applyForeground | red
	ForegroundGreen   Color = applyForeground | green
	ForegroundYellow  Color = applyForeground | yellow
	ForegroundBlue    Color = applyForeground | blue
	ForegroundMagenta Color = applyForeground | magenta
	ForegroundCyan    Color = applyForeground | cyan
	ForegroundWhite   Color = applyForeground | white

	BackgroundBright  Color = applyBackground | bright<<4
	BackgroundBlack   Color = applyBackground | black<<4
	BackgroundRed     Color = applyBackground | red<<4
	BackgroundGreen   Color = applyBackground | green<<4
	BackgroundYellow  Color = applyBackground | yellow<<4
	BackgroundBlue    Color = applyBackground | blue<<4
	BackgroundMagenta Color = applyBackground | magenta<<4
	BackgroundCyan    Color = applyBackground | cyan<<4
	BackgroundWhite   Color = applyBackground | white<<4

	Underline Color = 1 << 31 // 0b10000000000000000000000000000000 applyUnderline
	Negative  Color = 1 << 30 // 0b01000000000000000000000000000000 applyNegative

	foregroundMask  = bright | white      // 0b0000???? foreground
	backgroundMask  = foregroundMask << 4 // 0b????0000 background
	applyForeground = 1 << 11             // 0b0000100000000000 applyForeground
	applyBackground = 1 << 15             // 0b1000000000000000 applyBackground

	bright  = 1 << 3             // 0b1000
	black   = 0                  // 0b?000
	red     = 1 << 0             // 0b?001
	green   = 1 << 1             // 0b?010
	yellow  = red | green        // 0b?011
	blue    = 1 << 2             // 0b?100
	magenta = red | blue         // 0b?101
	cyan    = green | blue       // 0b?110
	white   = red | green | blue // 0b?111
)

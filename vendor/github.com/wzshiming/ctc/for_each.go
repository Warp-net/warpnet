package ctc

var ranges = [...][3]Color{
	{
		ForegroundBlack,
		ForegroundWhite | ForegroundBright,
		1,
	},
	{
		BackgroundBlack,
		BackgroundWhite | BackgroundBright,
		1 << 4,
	},
	{
		ForegroundBlack | BackgroundBlack,
		ForegroundWhite | ForegroundBright | BackgroundWhite | BackgroundBright,
		1,
	},
}

// ForEach for go through all colors
func ForEach(f func(Color)) {
	for _, r := range ranges {
		for c := r[0]; c <= r[1]; c += r[2] {
			f(c)
		}
	}
}

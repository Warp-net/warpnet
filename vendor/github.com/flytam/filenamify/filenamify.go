package filenamify

import (
	"errors"
	"math"
	"path/filepath"
	"regexp"
	"sync"
)

type Options struct {
	// String for substitution
	Replacement string
	// maxlength
	MaxLength int
}

const MAX_FILENAME_LENGTH = 100

var (
	reControlCharsRegex = regexp.MustCompile("[\u0000-\u001f\u0080-\u009f]")
	reRelativePathRegex = regexp.MustCompile(`^\.+`)

	// https://github.com/sindresorhus/filename-reserved-regex/blob/master/index.js
	filenameReservedRegex             = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
	filenameReservedWindowsNamesRegex = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[0-9]|lpt[0-9])$`)
)

func FilenamifyV2(str string, optFuns ...func(options *Options)) (string, error) {
	options := Options{
		Replacement: "!", // default remains the same
		MaxLength:   MAX_FILENAME_LENGTH,
	}
	for _, fn := range optFuns {
		fn(&options)
	}

	var replacement = options.Replacement

	if filenameReservedRegex.MatchString(replacement) && reControlCharsRegex.MatchString(replacement) {
		return "", errors.New("replacement string cannot contain reserved filename characters")
	}

	// reserved word
	str = filenameReservedRegex.ReplaceAllString(str, replacement)

	// continue
	str = reControlCharsRegex.ReplaceAllString(str, replacement)
	str = reRelativePathRegex.ReplaceAllString(str, replacement)

	// for repeat
	if len(replacement) > 0 {
		str = trimRepeated(str, replacement)

		if len(str) > 1 {
			str = stripOuter(str, replacement)
		}
	}

	// for windows names
	if filenameReservedWindowsNamesRegex.MatchString(str) {
		str = str + replacement
	}

	// limit length
	var limitLength int
	if options.MaxLength > 0 {
		limitLength = options.MaxLength
	} else {
		limitLength = MAX_FILENAME_LENGTH
	}
	strBuf := []rune(str)
	strBuf = strBuf[0:int(math.Min(float64(limitLength), float64(len(strBuf))))]

	return string(strBuf), nil
}

func Filenamify(str string, options Options) (string, error) {
	return FilenamifyV2(str, genFuncFromOptions(options))
}

func PathV2(filePath string, optFuns ...func(options *Options)) (string, error) {
	p, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}

	p, err = FilenamifyV2(filepath.Base(p), optFuns...)
	if err != nil {
		return "", err
	}

	return filepath.Join(filepath.Dir(p), p), nil
}

func Path(filePath string, options Options) (string, error) {
	return PathV2(filePath, genFuncFromOptions(options))
}

// https://github.com/sindresorhus/escape-string-regexp/blob/master/index.js
var reg = regexp.MustCompile(`[|\\{}()[\]^$+*?.-]`)

func escapeStringRegexp(str string) string {
	str = reg.ReplaceAllStringFunc(str, func(s string) string {
		return `\` + s
	})
	return str
}

type expressionCache struct {
	sync.RWMutex
	exp map[string]*regexp.Regexp
}

func (e *expressionCache) Get(exp string) *regexp.Regexp {
	e.RLock()
	v, ok := e.exp[exp]
	e.RUnlock()
	if ok {
		return v
	}
	e.Lock()
	defer e.Unlock()
	v = regexp.MustCompile(exp)
	e.exp[exp] = v
	return v
}

var cache = expressionCache{exp: make(map[string]*regexp.Regexp)}

func trimRepeated(str string, replacement string) string {
	exp := `(?:` + escapeStringRegexp(replacement) + `){2,}`
	reg := cache.Get(exp)
	return reg.ReplaceAllString(str, replacement)
}

func stripOuter(input string, substring string) string {
	// https://github.com/sindresorhus/strip-outer/blob/master/index.js
	substring = escapeStringRegexp(substring)
	exp := `^` + substring + `|` + substring + `$`
	reg := cache.Get(exp)
	return reg.ReplaceAllString(input, "")
}

func genFuncFromOptions(options Options) func(*Options) {
	var optFun = func(opt *Options) {
		if options.Replacement != "" {
			opt.Replacement = options.Replacement
		}
		if options.MaxLength > 0 {
			opt.MaxLength = options.MaxLength
		} else {
			opt.MaxLength = MAX_FILENAME_LENGTH
		}

		opt = &options
	}
	return optFun
}

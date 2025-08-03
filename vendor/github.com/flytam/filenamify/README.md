## go-filenamify


[![Build Status](https://travis-ci.com/flytam/filenamify.svg?branch=master)](https://travis-ci.com/flytam/filenamify)

Convert a string to a valid safe filename

#### Installation

```bash
$ go get github.com/flytam/filenamify
```

(optional) To run unit tests:

```bash
go test -v
```

#### Usage

```go
package main
import (
	"github.com/flytam/filenamify"
	"fmt"
)

func main() {
	output,err :=filenamify.Filenamify(`<foo/bar>`,filenamify.Options{})
    fmt.Println(output,err) // => foo!bar,nil

    //---
    output,err =filenamify.Filenamify(`foo:"bar"`,filenamify.Options{
    	Replacement:"🐴",
    })
    fmt.Println(output,err) // => foo🐴bar,nil


	output,err =filenamify.FilenamifyV2(`<foo/bar>`)
    fmt.Println(output,err) // => foo!bar,nil

    //---
    output,err =filenamify.FilenamifyV2(`foo:"bar"`,func(options *Options) {
		options.Replacement = "🐴"
	})
    fmt.Println(output,err) // => foo🐴bar,nil

}



```

#### API

- `Filenamify(str string, options Options) (string, error)`

- `func Path(filePath string, options Options) (string, error)`

```go
type Options struct {
	// String for substitution
	Replacement string// default: "!"
	// maxlength
	MaxLength int// default: 100
}
```
FilenamifyV2 and  PathV2 are added in v1.1.0
- `func FilenamifyV2(str string, optFuns ...func(options *Options)) (string, error)`
- `func PathV2(str string, optFuns ...func(options *Options)) (string, error)`

#### Related

- [Node-filenamify](https://github.com/sindresorhus/filenamify)

#### LICENSE
MIT

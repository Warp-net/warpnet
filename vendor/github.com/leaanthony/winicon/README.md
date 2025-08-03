<p align="center" style="text-align: center">
   <img src="logo.png" width="40%"><br/>
</p>
<p align="center">
   Windows .ico file generation library + cli. Works on all platforms supported by Go.<br/><br/>
   <a href="https://github.com/leaanthony/winicon/blob/master/LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg"></a>
   <a href="https://goreportcard.com/report/github.com/leaanthony/winicon"><img src="https://goreportcard.com/badge/github.com/leaanthony/winicon"/></a>
   <a href="http://godoc.org/github.com/leaanthony/winicon"><img src="https://img.shields.io/badge/godoc-reference-blue.svg"/></a>
   <a href="https://www.codefactor.io/repository/github/leaanthony/winicon"><img src="https://www.codefactor.io/repository/github/leaanthony/winicon/badge" alt="CodeFactor" /></a>
   <a href="https://github.com/leaanthony/winicon/issues"><img src="https://img.shields.io/badge/contributions-welcome-brightgreen.svg?style=flat" /></a>
   <a href="https://app.fossa.io/projects/git%2Bgithub.com%2Fleaanthony%2Fwinicon?ref=badge_shield" alt="FOSSA Status"><img src="https://app.fossa.io/api/projects/git%2Bgithub.com%2Fleaanthony%2Fwinicon.svg?type=shield"/></a>
</p>

Features:

* Provide information about an .ico file
* Generate an .ico file at multiple sizes from a PNG 
* Extract PNG images from an existing .ico file

## Installation

To install the cli tool:
`go get github.com/leaanthony/winicon/cmd/winicon`

To use the library:
`go get github.com/leaanthony/winicon`

## CLI Usage

The cli has 3 commands: info (i), generate (g) and extract (x).

### Info

You can get information about an .ico file by issuing the following command:

`winicon i <myicon.png>`. 

Example:

```bash
$ winicon i wails.ico 
WinIcon v1.0.0  (c) 2020-Present Lea Anthony

Filename:        /home/lea/Data/projects/winicon/examples/wails.ico
File Size:       322799 bytes
Icon Count:      9

Icon 1: Size: 16x16     Format: BMP     Bits Per Pixel: 32      Offset: 150
Icon 2: Size: 24x24     Format: BMP     Bits Per Pixel: 32      Offset: 1278
Icon 3: Size: 32x32     Format: BMP     Bits Per Pixel: 32      Offset: 3718
Icon 4: Size: 48x48     Format: BMP     Bits Per Pixel: 32      Offset: 7982
Icon 5: Size: 64x64     Format: BMP     Bits Per Pixel: 32      Offset: 17622
Icon 6: Size: 96x96     Format: BMP     Bits Per Pixel: 32      Offset: 34558
Icon 7: Size: 128x128   Format: BMP     Bits Per Pixel: 32      Offset: 72614
Icon 8: Size: 192x192   Format: BMP     Bits Per Pixel: 32      Offset: 140238
Icon 9: Size: 256x256   Format: PNG     Bits Per Pixel: 32      Offset: 292342
```

Winicon can return you detailed information in json format using the `-json` flag:

```bash
$ winicon i -json wails.ico 
{"Filename":"/home/lea/Data/projects/winicon/examples/wails.ico","FileSize":322799,"NumberOfIcons":9,"Icons":[{"Width":16,"Height":16,"Colours":0,"Planes":1,"BitsPerPixel":32,"Format":"BMP","Offset":150},{"Width":24,"Height":24,"Colours":0,"Planes":1,"BitsPerPixel":32,"Format":"BMP","Offset":1278},{"Width":32,"Height":32,"Colours":0,"Planes":1,"BitsPerPixel":32,"Format":"BMP","Offset":3718},{"Width":48,"Height":48,"Colours":0,"Planes":1,"BitsPerPixel":32,"Format":"BMP","Offset":7982},{"Width":64,"Height":64,"Colours":0,"Planes":1,"BitsPerPixel":32,"Format":"BMP","Offset":17622},{"Width":96,"Height":96,"Colours":0,"Planes":1,"BitsPerPixel":32,"Format":"BMP","Offset":34558},{"Width":128,"Height":128,"Colours":0,"Planes":1,"BitsPerPixel":32,"Format":"BMP","Offset":72614},{"Width":192,"Height":192,"Colours":0,"Planes":1,"BitsPerPixel":32,"Format":"BMP","Offset":140238},{"Width":256,"Height":256,"Colours":0,"Planes":1,"BitsPerPixel":32,"Format":"PNG","Offset":292342}]}
```

### Generate

Generate an .ico file from a PNG image:

```
$ winicon g icon.png 
WinIcon v0.0.1  (c) 2020-Present Lea Anthony

Generated icon file: icon.ico at sizes [256]
```

By default, this will generate an .ico with a single icon at 256x256. To set other sizes, use the `-sizes` flag:

```
$ winicon g -sizes 16,32,48 icon.png 
WinIcon v0.0.1  (c) 2020-Present Lea Anthony

Generated icon file: icon.ico at sizes [16 32 48]
```

Running `winicon i icon.ico` produces the following results:

```
$ winicon i icon.ico 
WinIcon v0.0.1  (c) 2020-Present Lea Anthony

Filename:        /home/lea/Data/projects/winicon/testdata/icon.ico
File Size:       4670 bytes
Icon Count:      3

Icon 1: Size: 16x16     Format: PNG     Bits Per Pixel: 32      Offset: 54
Icon 2: Size: 32x32     Format: PNG     Bits Per Pixel: 32      Offset: 774
Icon 3: Size: 48x48     Format: PNG     Bits Per Pixel: 32      Offset: 2266
```

### Extract images

You can extract images from .ico files using the following command:

```
$ winicon x icon.ico 
WinIcon v0.0.1  (c) 2020-Present Lea Anthony

Extracted 16x16 image to file: icon.16x16.png
Extracted 32x32 image to file: icon.32x32.png
Extracted 48x48 image to file: icon.48x48.png

Extracted 3 images
```

NOTE: Currently BMP images are not supported.

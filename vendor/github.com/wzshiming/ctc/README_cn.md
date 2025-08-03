# ctc - 终端文本颜色

[![Build Status](https://travis-ci.org/wzshiming/ctc.svg?branch=master)](https://travis-ci.org/wzshiming/ctc)
[![Go Report Card](https://goreportcard.com/badge/github.com/wzshiming/ctc)](https://goreportcard.com/report/github.com/wzshiming/ctc)
[![GoDoc](https://godoc.org/github.com/wzshiming/ctc?status.svg)](https://godoc.org/github.com/wzshiming/ctc)
[![GitHub license](https://img.shields.io/github/license/wzshiming/ctc.svg)](https://github.com/wzshiming/ctc/blob/master/LICENSE)
[![gocover.io](https://gocover.io/_badge/github.com/wzshiming/ctc)](https://gocover.io/github.com/wzshiming/ctc)

无侵入的跨平台的终端颜色库不需要修改Print方法

在windows 下靠虚拟成类unix 环境

- [English](https://github.com/wzshiming/ctc/blob/master/README.md)
- [简体中文](https://github.com/wzshiming/ctc/blob/master/README_cn.md)

## 支持

- [x] 终端
  - [x] 类 unix (mac 和 linux)
  - [x] windows

## 示例

``` golang
package main

import (
	"fmt"

	"github.com/wzshiming/ctc"
)

func main() {
	// 无侵入
	fmt.Println(ctc.BackgroundRed|ctc.ForegroundBlue, "Hello world", ctc.Reset)
}

```

## SGR (Select Graphic Rendition)

|   Value | Description | Behavior                 |
| ------: | :---------- | :----------------------- |
|       0 | 恢复默认    | 把所有属性还原到修改之前 |
|       4 | 下划线      | 添加下划线               |
|       7 | 交换颜色    | 交换前景和背景的颜色     |
|   30~37 | 前景        | 非高量的前景颜色         |
|   40~47 | 背景        | 非高量的背景颜色         |
|   90~97 | 高亮前景    | 高量的前景颜色           |
| 100~107 | 高亮背景    | 高量的背景颜色           |

## 许可证

软包根据MIT License。有关完整的许可证文本，请参阅[LICENSE](https://github.com/wzshiming/ctc/blob/master/LICENSE).

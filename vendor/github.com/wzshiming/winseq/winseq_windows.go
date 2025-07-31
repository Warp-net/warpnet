// +build windows

package winseq

import (
	"os"

	"golang.org/x/sys/windows"
)

// This package only works with Windows.
// Virtual terminal sequences are control character sequences that can control cursor movement,
// color/font mode, and other operations when written to the output stream.
// Sequences may also be received on the input stream in response to an output stream query information sequence or
// as an encoding of user input when the appropriate mode is set.

func init() {
	enableVirtualTerminalProcessing()
}

var (
	hStdout = windows.Handle(os.Stdout.Fd())
)

func enableVirtualTerminalProcessing() (err error) {
	var mode uint32
	err = windows.GetConsoleMode(hStdout, &mode)
	if err != nil {
		return err
	}

	if 0 != mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING {
		return nil
	}
	err = windows.SetConsoleMode(hStdout, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	if err != nil {
		return err
	}
	return nil
}

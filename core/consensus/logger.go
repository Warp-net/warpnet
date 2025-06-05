/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package consensus

import (
	"github.com/hashicorp/go-hclog"
	golog "github.com/ipfs/go-log/v2"
	"go.uber.org/zap/zapcore"
	"io"
	"log"
	"os"
	"strings"
)

const systemName = "raft"

var raftLogger = golog.Logger(systemName)

type consensusLogger struct {
	l                               *golog.ZapEventLogger
	name                            string
	decodeCount, failedRequestCount int
}

func newConsensusLogger() *consensusLogger {
	return &consensusLogger{
		l: raftLogger,
	}
}

func (c *consensusLogger) Log(level hclog.Level, msg string, args ...interface{}) {
	lvl, _ := zapcore.ParseLevel(strings.TrimSpace(strings.ToLower(level.String())))
	c.l.Logln(lvl, msg, args)
}

func (c *consensusLogger) Trace(msg string, args ...interface{}) {
	c.l.Debugln(msg, args)
}

func (c *consensusLogger) Debug(msg string, args ...interface{}) {
	c.l.Debugln(msg, args)
}

func (c *consensusLogger) Info(msg string, args ...interface{}) {
	c.l.Infoln(msg, args)
}

func (c *consensusLogger) Warn(msg string, args ...interface{}) {
	c.l.Warnln(msg, args)
}

func (c *consensusLogger) Error(msg string, args ...interface{}) {
	if strings.Contains(msg, "failed to take snapshot") {
		return
	}
	if strings.Contains(msg, "failed to make requestVote RPC") && c.failedRequestCount < 10 {
		c.failedRequestCount++
		return
	}
	if strings.Contains(msg, "failed to heartbeat") && c.failedRequestCount < 10 {
		c.failedRequestCount++
		return
	}

	if strings.Contains(msg, "failed to decode incoming command") && c.decodeCount < 3 {
		c.decodeCount++
		return
	}
	c.l.Errorln(msg, args)
	c.decodeCount = 0
	c.failedRequestCount = 0
}

func (c *consensusLogger) IsTrace() bool {
	return c.l.Level() == zapcore.DebugLevel
}

func (c *consensusLogger) IsDebug() bool {
	return c.l.Level() == zapcore.DebugLevel
}

func (c *consensusLogger) IsInfo() bool {
	return c.l.Level() == zapcore.InfoLevel
}

func (c *consensusLogger) IsWarn() bool {
	return c.l.Level() == zapcore.WarnLevel
}

func (c *consensusLogger) IsError() bool {
	return c.l.Level() == zapcore.ErrorLevel
}

func (c *consensusLogger) ImpliedArgs() []interface{} {
	return []interface{}{}
}

func (c *consensusLogger) With(args ...interface{}) hclog.Logger {
	gl := golog.Logger(systemName)
	gl.SugaredLogger = *gl.With(args)

	return &consensusLogger{l: gl, name: c.name}
}

func (c *consensusLogger) Name() string {
	return c.name
}

func (c *consensusLogger) Named(name string) hclog.Logger {
	c.l.Named(name)
	c.name = name
	return c
}

func (c *consensusLogger) ResetNamed(name string) hclog.Logger {
	c.name = name
	return c
}

func (c *consensusLogger) SetLevel(level hclog.Level) {
	//_ = golog.SetLogLevel(systemName, strings.TrimSpace(strings.ToLower(level.String())))
}

func (c *consensusLogger) GetLevel() hclog.Level {
	lvl := c.l.Level()
	return hclog.LevelFromString(lvl.String())
}

func (c *consensusLogger) StandardLogger(opts *hclog.StandardLoggerOptions) *log.Logger {
	return log.New(os.Stderr, c.name, log.LstdFlags)
}

func (c *consensusLogger) StandardWriter(opts *hclog.StandardLoggerOptions) io.Writer {
	return os.Stderr
}

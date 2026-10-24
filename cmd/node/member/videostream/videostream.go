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

package videostream

import (
	"bufio"
	"io"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/media-meta"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

const (
	FramePath  = "video-frame"
	CreditPath = "video-credit"
	StopPath   = "video-stop"

	frameReadTimeout = time.Minute
	maxCredits       = 8
)

type FrameSender interface {
	SendFrame(streamId string, frame event.VideoFrame) error
}

type Sessions struct {
	mx       sync.Mutex
	sessions map[string]*session
}

type session struct {
	stream  warpnet.WarpStream
	credits chan struct{}
	done    chan struct{}
	once    sync.Once
}

func NewSessions() *Sessions {
	return &Sessions{sessions: make(map[string]*session)}
}

func (ss *Sessions) Start(
	id string, offset int64, s warpnet.WarpStream, sender FrameSender,
) (domain.VideoManifest, error) {
	_ = s.SetReadDeadline(time.Now().Add(frameReadTimeout))
	data := bufio.NewReaderSize(s, media_meta.MaxManifestSize)
	manifest, err := media_meta.ReadVideoManifest(data)
	if err != nil {
		_ = s.Reset()
		return manifest, err
	}

	sess := &session{stream: s, credits: make(chan struct{}, maxCredits), done: make(chan struct{})}
	ss.mx.Lock()
	old := ss.sessions[id]
	ss.sessions[id] = sess
	ss.mx.Unlock()
	if old != nil {
		old.stop()
	}

	go ss.sendFrames(id, sess, data, manifest, max(offset, 0), sender)
	return manifest, nil
}

func (ss *Sessions) AddCredits(id string, count int) {
	ss.mx.Lock()
	sess := ss.sessions[id]
	ss.mx.Unlock()
	if sess == nil {
		return
	}
	for range min(count, maxCredits) {
		select {
		case sess.credits <- struct{}{}:
		default:
			return
		}
	}
}

func (ss *Sessions) Stop(id string) {
	ss.mx.Lock()
	sess := ss.sessions[id]
	delete(ss.sessions, id)
	ss.mx.Unlock()
	if sess != nil {
		sess.stop()
	}
}

func (ss *Sessions) StopAll() {
	ss.mx.Lock()
	all := ss.sessions
	ss.sessions = make(map[string]*session)
	ss.mx.Unlock()
	for _, sess := range all {
		sess.stop()
	}
}

func (ss *Sessions) sendFrames(
	id string, sess *session, data *bufio.Reader, manifest domain.VideoManifest, offset int64, sender FrameSender,
) {
	defer ss.remove(id, sess)

	first := int(offset / manifest.ChunkSize)
	if first >= len(manifest.Hashes) {
		_ = sender.SendFrame(id, event.VideoFrame{Offset: manifest.Size, IsLast: true})
		return
	}
	for idx := first; idx < len(manifest.Hashes); idx++ {
		select {
		case <-sess.credits:
		case <-sess.done:
			return
		}

		frame := event.VideoFrame{Offset: int64(idx) * manifest.ChunkSize}
		chunk := make([]byte, manifest.ChunkLen(idx))
		_ = sess.stream.SetReadDeadline(time.Now().Add(frameReadTimeout))
		if _, err := io.ReadFull(data, chunk); err != nil {
			if !sess.isStopped() {
				frame.Error = err.Error()
				_ = sender.SendFrame(id, frame)
			}
			return
		}
		frame.Data, frame.IsLast = chunk, idx == len(manifest.Hashes)-1
		if err := sender.SendFrame(id, frame); err != nil {
			return
		}
	}
}

func (ss *Sessions) remove(id string, sess *session) {
	ss.mx.Lock()
	if ss.sessions[id] == sess {
		delete(ss.sessions, id)
	}
	ss.mx.Unlock()
	sess.stop()
}

func (s *session) stop() {
	s.once.Do(func() {
		close(s.done)
		_ = s.stream.Reset()
	})
}

func (s *session) isStopped() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

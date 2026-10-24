/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, afterEach } from 'vitest';
import { canStreamVideo, VideoUnavailableError } from '@/lib/video-stream';

afterEach(() => {
  delete window.MediaSource;
});

describe('video streaming support', () => {
  it('streams only where Media Source Extensions exist', () => {
    expect(canStreamVideo()).toBe(false);

    window.MediaSource = function MediaSource() {};
    expect(canStreamVideo()).toBe(true);
  });

  it('tells an unavailable video apart from a playback failure', () => {
    expect(new VideoUnavailableError('gone')).toBeInstanceOf(Error);
  });
});

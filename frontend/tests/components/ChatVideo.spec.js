import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getChatVideo: vi.fn(),
  },
}));

import ChatVideo from '@/components/ChatVideo.vue';
import { warpnetService } from '@/service/service';

let errSpy;
beforeAll(() => {
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
});
afterAll(() => {
  errSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getChatVideo.mockResolvedValue({
    file: 'data:video/mp4;base64,AAAA',
    size: 1234,
    deferred: false,
  });
});

const renderVideo = (props = {}) =>
  render(ChatVideo, {
    props: {videoKey: 'vkey1', senderId: 'sender1', ...props},
  });

describe('ChatVideo', () => {
  it('does not fetch the clip until the user presses play', async () => {
    const {getByLabelText, container} = renderVideo();

    expect(warpnetService.getChatVideo).not.toHaveBeenCalled();
    expect(container.querySelector('video')).toBeNull();
    expect(getByLabelText('Play video')).toBeTruthy();
  });

  it('fetches from the sender node and shows the player on play', async () => {
    const {getByLabelText, container} = renderVideo();

    await fireEvent.click(getByLabelText('Play video'));

    await waitFor(() => expect(container.querySelector('video')).not.toBeNull());
    expect(warpnetService.getChatVideo).toHaveBeenCalledWith({
      userId: 'sender1',
      key: 'vkey1',
    });
    expect(container.querySelector('video').getAttribute('src'))
      .toBe('data:video/mp4;base64,AAAA');
  });

  it('plays the clip from a blob and frees it when the message goes away', async () => {
    const {createObjectURL, revokeObjectURL} = URL;
    URL.createObjectURL = vi.fn(() => 'blob:clip');
    URL.revokeObjectURL = vi.fn();
    try {
      const {getByLabelText, container, unmount} = renderVideo();

      await fireEvent.click(getByLabelText('Play video'));

      await waitFor(() => expect(container.querySelector('video')).not.toBeNull());
      expect(container.querySelector('video').getAttribute('src')).toBe('blob:clip');
      expect(URL.createObjectURL.mock.calls[0][0].type).toBe('video/mp4');

      unmount();
      expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:clip');
    } finally {
      URL.createObjectURL = createObjectURL;
      URL.revokeObjectURL = revokeObjectURL;
    }
  });

  it('shows the poster frame instead of a blank placeholder', () => {
    const {container} = renderVideo({poster: 'data:image/jpeg;base64,BBBB'});

    const poster = container.querySelector('img');
    expect(poster).not.toBeNull();
    expect(poster.getAttribute('src')).toBe('data:image/jpeg;base64,BBBB');
  });

  it('reports an unreachable sender instead of an empty player', async () => {
    warpnetService.getChatVideo.mockResolvedValue({file: '', size: 0, deferred: false});
    const {getByLabelText, getByText, container} = renderVideo();

    await fireEvent.click(getByLabelText('Play video'));

    await waitFor(() => expect(getByText(/isn't available right now/)).toBeTruthy());
    expect(container.querySelector('video')).toBeNull();
  });

  it('recovers the play button after a failed fetch', async () => {
    warpnetService.getChatVideo.mockRejectedValue(new Error('stream failed'));
    const {getByLabelText, getByText} = renderVideo();

    await fireEvent.click(getByLabelText('Play video'));

    await waitFor(() => expect(getByText('Failed to load the video.')).toBeTruthy());

    warpnetService.getChatVideo.mockResolvedValue({file: 'data:video/mp4;base64,AAAA'});
    await fireEvent.click(getByText('Try again'));

    await waitFor(() => expect(warpnetService.getChatVideo).toHaveBeenCalledTimes(2));
  });
});

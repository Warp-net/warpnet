import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getPollResults: vi.fn(),
    voteInPoll: vi.fn(),
    getOwnerProfile: vi.fn(),
  },
}));

import PollWidget from '@/components/PollWidget.vue';
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
  warpnetService.getOwnerProfile.mockReturnValue({user_id: 'viewer1'});
  warpnetService.getPollResults.mockResolvedValue({
    tweet_id: 't1', votes: [0, 0], total_votes: 0,
  });
});

const pollTweet = (poll) => ({
  id: 't1',
  user_id: 'author1',
  username: 'author',
  text: 'pick one',
  created_at: '2026-05-04T00:00:00Z',
  poll,
});

const openPoll = () => pollTweet({
  options: ['Cats', 'Dogs'],
  expires_at: new Date(Date.now() + 3600000).toISOString(),
});

const closedPoll = () => pollTweet({
  options: ['Cats', 'Dogs'],
  expires_at: new Date(Date.now() - 3600000).toISOString(),
});

const renderPoll = (tweet) => render(PollWidget, {props: {tweet}});

describe('PollWidget', () => {
  it('offers the choices as buttons while the poll is open and unanswered', async () => {
    const {getByText} = renderPoll(openPoll());

    await waitFor(() => expect(warpnetService.getPollResults).toHaveBeenCalledWith('t1', 'author1', 2));
    expect(getByText('Cats').tagName).toBe('BUTTON');
    expect(getByText('Dogs').tagName).toBe('BUTTON');
  });

  it('hides the tally until the reader has voted', async () => {
    warpnetService.getPollResults.mockResolvedValue({
      tweet_id: 't1', votes: [7, 3], total_votes: 10,
    });
    const {container} = renderPoll(openPoll());

    // The total is public — which option is winning is not.
    await waitFor(() => expect(container.textContent).toContain('10 votes'));
    expect(container.textContent).not.toContain('%');
  });

  it('reveals the tally and marks the choice after voting', async () => {
    warpnetService.voteInPoll.mockResolvedValue({
      tweet_id: 't1', votes: [1, 0], total_votes: 1, voted_option: 0,
    });
    const {getByText, container} = renderPoll(openPoll());

    await waitFor(() => expect(warpnetService.getPollResults).toHaveBeenCalled());
    await fireEvent.click(getByText('Cats'));

    await waitFor(() => expect(container.textContent).toContain('100%'));
    expect(warpnetService.voteInPoll).toHaveBeenCalledWith('t1', 'author1', 0, 2);
    expect(container.textContent).toContain('1 vote ·');
    expect(container.querySelector('.fa-check-circle')).not.toBeNull();
  });

  it('shows the final results without vote buttons once the poll has closed', async () => {
    warpnetService.getPollResults.mockResolvedValue({
      tweet_id: 't1', votes: [1, 3], total_votes: 4,
    });
    const {getByText, container} = renderPoll(closedPoll());

    await waitFor(() => expect(container.textContent).toContain('4 votes'));
    expect(container.textContent).toContain('Final results');
    expect(getByText('Cats').tagName).not.toBe('BUTTON');
    expect(container.textContent).toContain('25%');
    expect(container.textContent).toContain('75%');
  });

  it('keeps the choices clickable when the vote fails', async () => {
    warpnetService.voteInPoll.mockRejectedValue(new Error('node offline'));
    const {getByText} = renderPoll(openPoll());

    await waitFor(() => expect(warpnetService.getPollResults).toHaveBeenCalled());
    await fireEvent.click(getByText('Cats'));

    await waitFor(() => expect(warpnetService.voteInPoll).toHaveBeenCalled());
    expect(getByText('Cats').tagName).toBe('BUTTON');
  });

  it('does not pretend the vote landed when the node answers with an empty body', async () => {
    // sendToNode turns a node-side error into `{}` rather than throwing.
    warpnetService.voteInPoll.mockResolvedValue({});
    const {getByText, container} = renderPoll(openPoll());

    await waitFor(() => expect(warpnetService.getPollResults).toHaveBeenCalled());
    await fireEvent.click(getByText('Cats'));

    await waitFor(() => expect(warpnetService.voteInPoll).toHaveBeenCalled());
    expect(getByText('Cats').tagName).toBe('BUTTON');
    expect(container.textContent).not.toContain('%');
  });
});

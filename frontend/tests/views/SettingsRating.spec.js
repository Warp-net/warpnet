import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, within, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getOwnRating: vi.fn(),
  },
}));

import Rating from '@/views/Settings/Rating.vue';
import { warpnetService } from '@/service/service';

const TEXT_COLORS = ['text-green-500', 'text-yellow-500', 'text-orange-500', 'text-red-500'];
const BAR_COLORS = ['bg-green-500', 'bg-yellow-500', 'bg-orange-500', 'bg-red-500'];

const rating = (over = {}) => ({
  node_id: '12D3KooWOwnerNode',
  overall: 1000,
  tier: 'trusted',
  dimensions: [],
  observers: 0,
  updated_at: '2026-09-30T00:00:00Z',
  ...over,
});
const dim = (name, over = {}) => ({ name, score: 1000, tier: 'trusted', recent: [], ...over });
const tally = (kind, count = 1) => ({ kind, count, last_at: '2026-09-29T23:00:00Z' });

// Wire names from the Go catalogue itself, so a kind added there without a
// label here fails the audit below.
const backendOffenceKinds = async () => {
  const { default: source } = await import('../../../core/rating/offence.go?raw');
  const catalogue = source.slice(source.indexOf('var catalogue'));
  const body = catalogue.slice(0, catalogue.indexOf('\n}'));
  return [...body.matchAll(/Kind\w+:\s*\{"([^"]+)"/g)].map((m) => m[1]);
};

const renderRating = () => {
  const router = { push: vi.fn() };
  render(Rating, {
    global: {
      mocks: { $router: router },
      stubs: {
        SideNav: true,
        DefaultRightBar: true,
        Loader: {
          props: ['loading'],
          template: '<div v-if="loading" data-testid="loader" />',
        },
      },
    },
  });
  return router;
};

const renderLoaded = async (resp) => {
  warpnetService.getOwnRating.mockResolvedValue(resp);
  const router = renderRating();
  await screen.findByText('/ 1000');
  return router;
};

const colors = (el, palette) => palette.filter((c) => el.classList.contains(c));
const dimensionBlock = (label) => screen.getByText(label).closest('.py-4');
const barOf = (block) => block.querySelector('[style*="width"]');
const tallies = (block) =>
  within(block)
    .queryAllByText(/^\d+×$/)
    .map((count) => [count.previousElementSibling.textContent, count.textContent]);

let logSpy, errSpy, warnSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
});
afterAll(() => {
  logSpy.mockRestore();
  errSpy.mockRestore();
  warnSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'me', node_id: '12D3KooWOwnerNode' });
  warpnetService.getOwnRating.mockResolvedValue(rating());
});

describe('Settings/Rating.vue loading and failure', () => {
  it('shows only the loader while the rating is on its way', async () => {
    warpnetService.getOwnRating.mockImplementation(() => new Promise(() => {}));
    renderRating();

    expect(await screen.findByTestId('loader')).toBeInTheDocument();
    expect(screen.getByText('Node rating')).toBeInTheDocument();
    expect(screen.queryByText('/ 1000')).not.toBeInTheDocument();
    expect(screen.queryByText('Rating is not available')).not.toBeInTheDocument();
  });

  it.each([
    ['no answer', null],
    ['an undefined answer', undefined],
    ['an empty answer', {}],
    ['an error body', { code: 500, message: 'rating is not available on this node' }],
    ['an answer without overall', { tier: 'trusted', observers: 2, dimensions: [] }],
  ])('says the rating is not available on %s', async (_, resp) => {
    warpnetService.getOwnRating.mockResolvedValue(resp);
    renderRating();

    expect(await screen.findByText('Rating is not available')).toBeInTheDocument();
    expect(screen.queryByTestId('loader')).not.toBeInTheDocument();
    expect(screen.queryByText('/ 1000')).not.toBeInTheDocument();
    expect(screen.queryByText('Nothing to report')).not.toBeInTheDocument();
  });

  it('says the rating is not available and logs why when the request fails', async () => {
    const err = new Error('stream reset');
    warpnetService.getOwnRating.mockRejectedValue(err);
    renderRating();

    expect(await screen.findByText('Rating is not available')).toBeInTheDocument();
    expect(screen.queryByTestId('loader')).not.toBeInTheDocument();
    expect(screen.queryByText('/ 1000')).not.toBeInTheDocument();
    expect(errSpy).toHaveBeenCalledWith('Failed to load node rating:', err);
  });

  it('goes back to Settings', async () => {
    const router = await renderLoaded(rating());

    await fireEvent.click(screen.getByRole('button', { name: 'Back' }));

    expect(router.push).toHaveBeenCalledWith({ name: 'Settings' });
  });
});

describe('Settings/Rating.vue overall standing', () => {
  it.each([
    ['trusted', 'Trusted', 'text-green-500'],
    ['watched', 'Watched', 'text-yellow-500'],
    ['degraded', 'Degraded', 'text-orange-500'],
    ['floor', 'Severely degraded', 'text-red-500'],
    ['unknown', 'unknown', 'text-green-500'],
  ])('shows a %s node as "%s" in %s', async (tier, label, color) => {
    await renderLoaded(rating({ overall: 642, tier }));

    expect(colors(screen.getByText('642'), TEXT_COLORS)).toEqual([color]);
    expect(colors(screen.getByText(label), TEXT_COLORS)).toEqual([color]);
    expect(warpnetService.getOwnRating).toHaveBeenCalledTimes(1);
  });

  it('shows an overall of 0 instead of treating it as missing', async () => {
    await renderLoaded(rating({ overall: 0, tier: 'floor', observers: 4 }));

    expect(screen.getByText('0')).toHaveClass('text-red-500');
    expect(screen.getByText('Severely degraded')).toBeInTheDocument();
    expect(screen.queryByText('Rating is not available')).not.toBeInTheDocument();
  });

  it.each([
    [0, '0 other nodes have observed'],
    [1, '1 other node has observed'],
    [2, '2 other nodes have observed'],
  ])('words the sentence for %i observers', async (observers, phrase) => {
    await renderLoaded(rating({ observers }));

    const sentence = screen.getByText(/does not rate itself/);
    expect(sentence).toHaveTextContent(`this is what ${phrase}.`);
    expect(within(sentence).getByText(String(observers))).toHaveClass('font-bold');
  });
});

describe('Settings/Rating.vue dimensions', () => {
  it.each([
    ['null', null],
    ['empty', []],
    ['missing', undefined],
  ])('says there is nothing to report when dimensions are %s', async (_, dimensions) => {
    await renderLoaded(rating({ dimensions }));

    expect(screen.getByText('Nothing to report')).toBeInTheDocument();
    expect(screen.queryByText('Recently observed')).not.toBeInTheDocument();
  });

  it('drops the nothing-to-report note once a dimension is present', async () => {
    await renderLoaded(rating({ dimensions: [dim('net')] }));

    expect(screen.getByText('Network')).toBeInTheDocument();
    expect(screen.queryByText('Nothing to report')).not.toBeInTheDocument();
  });

  it.each([
    ['net', 'Network', /on the wire/],
    ['app', 'Application', /upheld moderation decisions/],
    ['mod', 'Moderation', /verdicts they cast/],
  ])('names the %s dimension "%s" and says what it covers', async (name, label, hint) => {
    await renderLoaded(rating({ dimensions: [dim(name, { score: 730, tier: 'watched' })] }));

    const block = dimensionBlock(label);
    expect(within(block).getByText(hint)).toBeInTheDocument();
    expect(within(block).getByText('730')).toBeInTheDocument();
  });

  it('shows a dimension it does not know by its wire name, with no hint', async () => {
    await renderLoaded(rating({ dimensions: [dim('storage', { score: 880 })] }));

    expect(dimensionBlock('storage')).toHaveTextContent(/^storage\s*880$/);
  });

  it('keeps the dimensions in the order the node sent them', async () => {
    await renderLoaded(rating({
      dimensions: [dim('mod'), dim('storage'), dim('net'), dim('app')],
    }));

    const labels = screen
      .getAllByText(/^(Network|Application|Moderation|storage)$/)
      .map((el) => el.textContent);
    expect(labels).toEqual(['Moderation', 'storage', 'Network', 'Application']);
  });

  it.each([
    ['trusted', 'text-green-500', 'bg-green-500'],
    ['watched', 'text-yellow-500', 'bg-yellow-500'],
    ['degraded', 'text-orange-500', 'bg-orange-500'],
    ['floor', 'text-red-500', 'bg-red-500'],
    ['unknown', 'text-green-500', 'bg-green-500'],
  ])('colors the score and bar of a %s dimension', async (tier, text, bar) => {
    await renderLoaded(rating({ overall: 999, dimensions: [dim('net', { score: 612, tier })] }));

    const block = dimensionBlock('Network');
    expect(colors(within(block).getByText('612'), TEXT_COLORS)).toEqual([text]);
    expect(colors(barOf(block), BAR_COLORS)).toEqual([bar]);
  });

  it.each([
    [742, '74.2%'],
    [0, '0%'],
    [1000, '100%'],
    [-50, '0%'],
    [1500, '100%'],
    ['742', '74.2%'],
    ['abc', '0%'],
    [null, '0%'],
    [undefined, '0%'],
  ])('fills the bar for a score of %s to %s', async (score, width) => {
    await renderLoaded(rating({ dimensions: [dim('net', { score })] }));

    expect(barOf(dimensionBlock('Network')).style.width).toBe(width);
  });
});

describe('Settings/Rating.vue recent offences', () => {
  it.each([
    ['null', null],
    ['empty', []],
    ['missing', undefined],
  ])('leaves the tally list out when recent is %s', async (_, recent) => {
    await renderLoaded(rating({ dimensions: [dim('net', { score: 700, tier: 'watched', recent })] }));

    expect(dimensionBlock('Network')).toBeInTheDocument();
    expect(screen.queryByText('Recently observed')).not.toBeInTheDocument();
  });

  it('lists each offence under its own dimension, in the order sent, with its count', async () => {
    await renderLoaded(rating({
      overall: 612,
      tier: 'watched',
      observers: 3,
      dimensions: [
        dim('net', {
          score: 612,
          tier: 'watched',
          recent: [tally('rate_limit_hit', 12), tally('bad_signature', 3), tally('dial_failure', 1)],
        }),
        dim('app', { score: 940, recent: [tally('write_flood', 2)] }),
      ],
    }));

    const net = dimensionBlock('Network');
    expect(within(net).getByText('Recently observed')).toBeInTheDocument();
    expect(tallies(net)).toEqual([
      ['Requests over the rate limit', '12×'],
      ['Messages with an invalid signature', '3×'],
      ['Failed connection attempts', '1×'],
    ]);
    expect(tallies(dimensionBlock('Application'))).toEqual([['Excessive posting', '2×']]);
  });

  it('shows an offence it has no label for by its wire name', async () => {
    await renderLoaded(rating({
      dimensions: [dim('net', { recent: [tally('quantum_tunnelling', 5)] })],
    }));

    expect(tallies(dimensionBlock('Network'))).toEqual([['quantum_tunnelling', '5×']]);
  });

  it('gives every offence in the backend catalogue a label of its own', async () => {
    const kinds = await backendOffenceKinds();
    expect(kinds, 'no offence kinds parsed from core/rating/offence.go').not.toHaveLength(0);

    await renderLoaded(rating({
      dimensions: [dim('net', { recent: kinds.map((kind) => tally(kind)) })],
    }));

    const unlabeled = kinds.filter((kind) => screen.queryByText(kind) !== null);
    expect(unlabeled).toEqual([]);
    const labels = tallies(dimensionBlock('Network')).map(([label]) => label);
    expect(labels).toHaveLength(kinds.length);
    expect(new Set(labels).size).toBe(kinds.length);
  });
});

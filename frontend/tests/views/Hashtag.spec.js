import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

import Hashtag from '@/views/Hashtag.vue';

const scrollDirective = {
  mounted() {},
  updated() {},
  unmounted() {},
};

const routerPush = vi.fn();

const renderHashtag = ({ query = {} } = {}) =>
  render(Hashtag, {
    global: {
      mocks: {
        $router: { push: routerPush },
        $route: { query },
      },
      directives: { scroll: scrollDirective },
      stubs: { SideNav: true, Results: true, Loader: true },
    },
  });

let logSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
});
afterAll(() => {
  logSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  routerPush.mockClear();
});

describe('Hashtag.vue', () => {
  it('renders the search input and tab buttons', () => {
    renderHashtag();

    expect(screen.getByPlaceholderText(/Search Warpnet/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Top' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Latest' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'People' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Photos' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Videos' })).toBeInTheDocument();
  });

  it('prefills the search input from the route query', () => {
    renderHashtag({ query: { q: '#vue' } });

    expect(screen.getByPlaceholderText(/Search Warpnet/i)).toHaveValue('#vue');
  });

  it('defaults the active tab to Latest when no mode is in the route', () => {
    renderHashtag();

    const latest = screen.getByRole('button', { name: 'Latest' });
    const people = screen.getByRole('button', { name: 'People' });
    expect(latest.className).toMatch(/border-blue/);
    expect(people.className).not.toMatch(/border-blue/);
  });

  it('uses the mode from the route query when provided', () => {
    renderHashtag({ query: { m: 'People' } });

    const people = screen.getByRole('button', { name: 'People' });
    expect(people.className).toMatch(/border-blue/);
  });

  it('lets the user type in the search input', async () => {
    renderHashtag();

    const input = screen.getByPlaceholderText(/Search Warpnet/i);
    await fireEvent.update(input, '#testing');
    expect(input).toHaveValue('#testing');
  });

  it('navigates home when the back button is clicked', async () => {
    renderHashtag();

    const backButton = screen.getAllByRole('button').find((btn) =>
      btn.querySelector('.fa-arrow-left')
    );
    await fireEvent.click(backButton);

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({ name: 'Home' });
    });
  });

  it('renders with an empty input when no route query is provided (edge case)', () => {
    renderHashtag();

    expect(screen.getByPlaceholderText(/Search Warpnet/i)).toHaveValue('');
  });
});

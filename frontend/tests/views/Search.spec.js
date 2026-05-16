import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

import Search from '@/views/Search.vue';

const scrollDirective = {
  mounted() {},
  updated() {},
  unmounted() {},
};

const routerPush = vi.fn();

const renderSearch = ({ query = {} } = {}) =>
  render(Search, {
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

describe('Search.vue', () => {
  it('renders the search input and the People tab', () => {
    renderSearch();

    expect(screen.getByPlaceholderText(/Search Warpnet/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'People' })).toBeInTheDocument();
  });

  it('prefills the search input from the route query', () => {
    renderSearch({ query: { q: 'vitest' } });

    expect(screen.getByPlaceholderText(/Search Warpnet/i)).toHaveValue('vitest');
  });

  it('marks the People tab active by default', () => {
    renderSearch();

    const people = screen.getByRole('button', { name: 'People' });
    expect(people.className).toMatch(/border-blue/);
  });

  it('keeps People active when route query selects it', () => {
    renderSearch({ query: { m: 'People' } });

    const people = screen.getByRole('button', { name: 'People' });
    expect(people.className).toMatch(/border-blue/);
  });

  it('lets the user type into the search box', async () => {
    renderSearch();

    const input = screen.getByPlaceholderText(/Search Warpnet/i);
    await fireEvent.update(input, 'warpnet');
    expect(input).toHaveValue('warpnet');
  });

  it('navigates home when the back button is clicked', async () => {
    renderSearch();

    const backButton = screen.getAllByRole('button').find((btn) =>
      btn.querySelector('.fa-arrow-left')
    );
    await fireEvent.click(backButton);

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({ name: 'Home' });
    });
  });

  it('renders with an empty input when no route query is provided (edge case)', () => {
    renderSearch();

    expect(screen.getByPlaceholderText(/Search Warpnet/i)).toHaveValue('');
  });
});

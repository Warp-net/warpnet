import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    searchUsers: vi.fn(),
    getImage: vi.fn(),
    getGatewaySettings: vi.fn(),
    getProfile: vi.fn(),
  },
}));

import Search from '@/views/Search.vue';
import { warpnetService } from '@/service/service';

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
      stubs: {
        SideNav: true,
        Results: true,
        Loader: true,
        Users: {
          props: ['users', 'loading'],
          template:
            '<ul data-testid="user-list"><li v-for="u in users" :key="u.id">{{ u.username }}</li></ul>',
        },
      },
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
  warpnetService.searchUsers.mockResolvedValue({ users: [], cursor: 'end' });
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getGatewaySettings.mockResolvedValue({ node_id: 'GW' });
  warpnetService.getProfile.mockResolvedValue({});
});

const listed = () =>
  Array.from(screen.getByTestId('user-list').querySelectorAll('li')).map((li) => li.textContent);

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

  it('renders search results without waiting for hanging avatar blobs', async () => {
    warpnetService.searchUsers.mockResolvedValue({
      users: [
        { id: 'bob', username: 'Bobby', avatar_key: 'k1' },
        { id: 'carol', username: 'Caroline', avatar_key: 'k2' },
      ],
      cursor: 'end',
    });
    warpnetService.getImage.mockImplementation(() => new Promise(() => {}));

    renderSearch({ query: { q: 'bo' } });

    expect(
      await screen.findByText('Bobby', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
    expect(screen.getByText('Caroline')).toBeInTheDocument();
  });

  it('renders with an empty input when no route query is provided (edge case)', () => {
    renderSearch();

    expect(screen.getByPlaceholderText(/Search Warpnet/i)).toHaveValue('');
  });
});

describe('Search.vue fediverse lookup', () => {
  it('resolves a handle through the gateway and lists it first', async () => {
    warpnetService.searchUsers.mockResolvedValue({
      users: [{ id: 'carol', username: 'Caroline' }],
      cursor: 'end',
    });
    warpnetService.getProfile.mockResolvedValue({ id: 'bob@mastodon.social', username: 'Bob' });

    renderSearch({ query: { q: '@bob@mastodon.social' } });

    await screen.findByText('Bob');
    expect(warpnetService.getProfile).toHaveBeenCalledWith('bob@mastodon.social', 'GW');
    expect(listed()).toEqual(['Bob', 'Caroline']);
  });

  it('resolves on Enter and keeps the result when the pending debounce would fire', async () => {
    warpnetService.getProfile.mockResolvedValue({ id: 'mosseri@threads.net', username: 'Adam' });
    renderSearch();

    const input = screen.getByPlaceholderText(/Search Warpnet/i);
    await fireEvent.update(input, 'https://www.threads.net/@mosseri');
    await fireEvent.keyUp(input, { key: 'Enter' });

    await screen.findByText('Adam');
    expect(warpnetService.getProfile).toHaveBeenCalledWith('mosseri@threads.net', 'GW');
    await new Promise((r) => setTimeout(r, 500));
    expect(listed()).toEqual(['Adam']);
    expect(warpnetService.searchUsers).toHaveBeenCalledTimes(1);
  });

  it('keeps debounced typing local', async () => {
    renderSearch();

    await fireEvent.update(screen.getByPlaceholderText(/Search Warpnet/i), 'bob@mastodon.social');

    await waitFor(() => expect(warpnetService.searchUsers).toHaveBeenCalled(), { timeout: 2000 });
    expect(warpnetService.getProfile).not.toHaveBeenCalled();
  });

  it('shows only the local results when the handle does not resolve', async () => {
    warpnetService.searchUsers.mockResolvedValue({
      users: [{ id: 'carol', username: 'Caroline' }],
      cursor: 'end',
    });

    renderSearch({ query: { q: 'ghost@mastodon.social' } });

    await waitFor(() => expect(warpnetService.getProfile).toHaveBeenCalled());
    await screen.findByText('Caroline');
    expect(listed()).toEqual(['Caroline']);
  });

  it('drops a lookup that resolves after a newer search', async () => {
    let resolveLookup;
    warpnetService.getProfile.mockImplementation(() => new Promise((r) => { resolveLookup = r; }));
    warpnetService.searchUsers.mockImplementation((q) =>
      Promise.resolve({ users: q === 'carol' ? [{ id: 'carol', username: 'Caroline' }] : [], cursor: 'end' })
    );

    renderSearch({ query: { q: 'bob@mastodon.social' } });
    await waitFor(() => expect(warpnetService.getProfile).toHaveBeenCalled());

    const input = screen.getByPlaceholderText(/Search Warpnet/i);
    await fireEvent.update(input, 'carol');
    await fireEvent.keyUp(input, { key: 'Enter' });
    await screen.findByText('Caroline');

    resolveLookup({ id: 'bob@mastodon.social', username: 'Bob' });
    await new Promise((r) => setTimeout(r, 50));
    expect(listed()).toEqual(['Caroline']);
  });
});

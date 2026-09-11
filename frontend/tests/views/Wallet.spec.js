import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getWallet: vi.fn(),
    getWalletAddress: vi.fn(),
    getWalletHistory: vi.fn(),
    getWalletContacts: vi.fn(),
    sendUsdt: vi.fn(),
    exportWalletKey: vi.fn(),
  },
}));

vi.mock('@/lib/qr', () => ({ buildQRCode: vi.fn(async () => 'data:image/png;base64,qr') }));

import Wallet from '@/views/Wallet.vue';
import { warpnetService } from '@/service/service';

const line = (re) => (_, el) =>
  !!el && el.tagName === 'P' && re.test(el.textContent.replace(/\s+/g, ' ').trim());

const deferred = () => {
  let resolve;
  const promise = new Promise((r) => { resolve = r; });
  return { promise, resolve };
};

const renderWallet = () =>
  render(Wallet, {
    global: {
      mocks: { $router: { back: vi.fn() } },
      stubs: {
        SideNav: true,
        DefaultRightBar: true,
        Loader: { props: ['loading'], template: '<div v-if="loading" data-testid="loader"></div>' },
      },
    },
  });

const wallet = {
  address: 'TZEDAzs41eK4ipB9gaUnKbaWLvjyn2paQa',
  usdt_balance: '50000000',
  trx_balance: '100000000',
  decimals: 6,
  network: 'testnet',
  activated: true,
};

let logSpy, errSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
});
afterAll(() => {
  logSpy.mockRestore();
  errSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'me' });
  warpnetService.getWallet.mockResolvedValue(wallet);
  warpnetService.getWalletAddress.mockResolvedValue({
    address: wallet.address,
    token: 'TXYZ',
    decimals: 6,
    network: 'testnet',
  });
  warpnetService.getWalletHistory.mockResolvedValue([]);
  warpnetService.getWalletContacts.mockResolvedValue([]);
});

describe('Wallet.vue', () => {
  it('renders the page before any request answers', async () => {
    warpnetService.getWalletAddress.mockReturnValue(new Promise(() => {}));
    warpnetService.getWallet.mockReturnValue(new Promise(() => {}));
    warpnetService.getWalletHistory.mockReturnValue(new Promise(() => {}));
    warpnetService.getWalletContacts.mockReturnValue(new Promise(() => {}));
    renderWallet();
    expect(screen.getByText('Wallet')).toBeTruthy();
    expect(screen.getByText('Balance')).toBeTruthy();
    expect(screen.getByText('History')).toBeTruthy();
    expect(screen.getAllByText(line(/^— USDT$/)).length).toBe(1);
  });

  it('shows the address while the balance is still loading', async () => {
    const slow = deferred();
    warpnetService.getWallet.mockReturnValue(slow.promise);
    renderWallet();
    await waitFor(() => expect(screen.getByText(wallet.address)).toBeTruthy());
    expect(screen.getByText(line(/^— USDT$/))).toBeTruthy();
    expect(warpnetService.getWalletAddress).toHaveBeenCalled();
    slow.resolve(wallet);
    await waitFor(() => expect(screen.getByText(line(/^50 USDT$/))).toBeTruthy());
  });

  it('keeps the address when the balance call fails', async () => {
    warpnetService.getWallet.mockRejectedValue(new Error('engine unavailable'));
    renderWallet();
    await waitFor(() => expect(screen.getByText('engine unavailable')).toBeTruthy());
    expect(screen.getByText(wallet.address)).toBeTruthy();
  });

  it('shows the history while the balance is still loading', async () => {
    const slow = deferred();
    warpnetService.getWallet.mockReturnValue(slow.promise);
    warpnetService.getWalletHistory.mockResolvedValue([
      { tx: 'abc', from: 'TFrom', to: 'TTo', value: '2000000', incoming: true },
    ]);
    renderWallet();
    await waitFor(() => expect(screen.getByText(line(/^\+2 USDT$/))).toBeTruthy());
    expect(screen.queryByText(line(/^50 USDT$/))).toBeNull();
    slow.resolve(wallet);
    await waitFor(() => expect(screen.getByText(line(/^50 USDT$/))).toBeTruthy());
  });

  it('shows the balance while the contact sweep is still running', async () => {
    const slow = deferred();
    warpnetService.getWalletContacts.mockReturnValue(slow.promise);
    renderWallet();
    await waitFor(() => expect(screen.getByText(line(/^50 USDT$/))).toBeTruthy());
    expect(screen.getByText(/Looking for the wallets/)).toBeTruthy();
    slow.resolve([]);
    await waitFor(() => expect(screen.queryByText(/Looking for the wallets/)).toBeNull());
  });

  it('keeps the page up when the wallet call fails', async () => {
    warpnetService.getWallet.mockRejectedValue(new Error('engine unavailable'));
    renderWallet();
    await waitFor(() => expect(screen.getByText('engine unavailable')).toBeTruthy());
    expect(screen.getByText('History')).toBeTruthy();
  });

  it('keeps polling the history until the sent transfer solidifies', async () => {
    vi.useFakeTimers();
    try {
      warpnetService.sendUsdt.mockResolvedValue({ tx: 'newtx' });
      warpnetService.getWalletHistory.mockResolvedValue([]);
      warpnetService.getWalletContacts.mockResolvedValue([
        { user_id: 'u2', username: 'Vadim', address: 'TMFCti1AJ7VYQ6QDetHHZu8AkfzMd3P5R6' },
      ]);
      renderWallet();
      await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Send' }).disabled).toBe(false));

      await fireEvent.update(screen.getByRole('combobox'), 'TMFCti1AJ7VYQ6QDetHHZu8AkfzMd3P5R6');
      await fireEvent.update(screen.getByPlaceholderText('0.0'), '1');
      await fireEvent.click(screen.getByRole('button', { name: 'Send' }));
      await vi.waitFor(() => expect(warpnetService.sendUsdt).toHaveBeenCalled());

      const afterSend = warpnetService.getWalletHistory.mock.calls.length;
      await vi.advanceTimersByTimeAsync(15000);
      expect(warpnetService.getWalletHistory.mock.calls.length).toBe(afterSend + 1);

      warpnetService.getWalletHistory.mockResolvedValue([
        { tx: 'newtx', from: 'TMe', to: 'TThem', value: '1000000', incoming: false },
      ]);
      await vi.advanceTimersByTimeAsync(15000);
      const settled = warpnetService.getWalletHistory.mock.calls.length;
      await vi.advanceTimersByTimeAsync(60000);
      expect(warpnetService.getWalletHistory.mock.calls.length).toBe(settled);
    } finally {
      vi.useRealTimers();
    }
  });

  it('clears every section loader once the calls answer', async () => {
    renderWallet();
    await waitFor(() => expect(screen.queryAllByTestId('loader').length).toBe(0));
    expect(screen.getByText(line(/^50 USDT$/))).toBeTruthy();
    expect(screen.getByText('No USDT transfers yet.')).toBeTruthy();
  });
});

import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/vue';

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

  it('clears every section loader once the calls answer', async () => {
    renderWallet();
    await waitFor(() => expect(screen.queryAllByTestId('loader').length).toBe(0));
    expect(screen.getByText(line(/^50 USDT$/))).toBeTruthy();
    expect(screen.getByText('No USDT transfers yet.')).toBeTruthy();
  });
});

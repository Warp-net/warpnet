/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: { turnWalletOn: vi.fn() },
}));
vi.mock('@/lib/toast', () => ({ toast: { error: vi.fn() } }));

import WalletSwitchDialog from '@/components/WalletSwitchDialog.vue';
import { warpnetService } from '@/service/service';
import { toast } from '@/lib/toast';

const renderDialog = () => render(WalletSwitchDialog, { props: { show: true, reason: 'Selling needs a wallet.' } });

describe('WalletSwitchDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  it('says why the wallet is needed and turns it on when confirmed', async () => {
    warpnetService.turnWalletOn.mockResolvedValue({ enabled: true });
    const { emitted } = renderDialog();

    expect(await screen.findByText(/Selling needs a wallet\./)).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));

    await waitFor(() => expect(emitted().on).toHaveLength(1));
    expect(warpnetService.turnWalletOn).toHaveBeenCalledTimes(1);
  });

  it('stays open and says so when the node refuses', async () => {
    warpnetService.turnWalletOn.mockRejectedValue(new Error('db closed'));
    const { emitted } = renderDialog();

    await fireEvent.click(await screen.findByRole('button', { name: 'Turn on' }));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('db closed'));
    expect(emitted().on).toBeUndefined();
    expect(screen.getByRole('button', { name: 'Turn on' })).not.toBeDisabled();
  });
});

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getNotificationSettings: vi.fn(),
    updateNotificationSettings: vi.fn(),
  },
}));
vi.mock('@/lib/toast', () => ({ toast: { error: vi.fn() } }));

import SettingsNotifications from '@/views/Settings/Notifications.vue';
import { warpnetService } from '@/service/service';

// Wire names from the Go type itself, so a renamed type fails here instead of
// silently never matching an email toggle.
const backendNotificationTypes = async () => {
  const { default: source } = await import('../../../domain/warpnet.go?raw');
  return [...source.matchAll(/Notification\w+Type\s+NotificationType = "([^"]+)"/g)].map((m) => m[1]);
};

const renderLoaded = async (saved) => {
  warpnetService.getNotificationSettings.mockResolvedValue(saved);
  const { container } = render(SettingsNotifications, {
    global: {
      mocks: { $router: { push: vi.fn() } },
      stubs: { SideNav: true, DefaultRightBar: true, Loader: true },
    },
  });
  await screen.findByText('Enable email notifications');
  return container;
};

describe('Settings/Notifications.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    warpnetService.updateNotificationSettings.mockResolvedValue({});
  });

  it('keys every email toggle by a backend notification type', async () => {
    const backend = await backendNotificationTypes();
    expect(backend).toContain('reaction');
    const { types } = SettingsNotifications.data();
    for (const t of types) {
      expect(backend).toContain(t.key);
    }
  });

  it('saves the reactions toggle under the reaction type', async () => {
    await renderLoaded({ email_enabled: true, recipient: 'me@example.com', types: {} });
    await fireEvent.click(screen.getByLabelText('Reactions to your tweets'));
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(warpnetService.updateNotificationSettings).toHaveBeenCalledWith(
      expect.objectContaining({ types: { reaction: true } }),
    );
  });

  it('requires a recipient only while email is enabled', async () => {
    const container = await renderLoaded({ email_enabled: true, recipient: '', types: {} });
    const recipient = container.querySelector('input[type="email"]');
    expect(recipient.required).toBe(true);
    await fireEvent.click(screen.getByLabelText('Enable email notifications'));
    expect(recipient.required).toBe(false);
  });
});

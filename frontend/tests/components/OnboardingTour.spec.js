/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/vue';

import OnboardingTour from '@/components/OnboardingTour.vue';
import {
  finishOnboarding,
  onboarding,
  onboardingSteps,
  queueOnboarding,
  startOnboarding,
  startQueuedOnboarding,
} from '@/lib/onboarding';

// jsdom lays nothing out, so an anchor counts as on screen only when a test
// gives it a size.
const placeAnchor = (name) => {
  const el = document.createElement('button');
  el.dataset.tour = name;
  el.getBoundingClientRect = () => ({ top: 10, left: 10, width: 40, height: 40, right: 50, bottom: 50 });
  el.scrollIntoView = () => {};
  document.body.appendChild(el);
  return el;
};

const frames = async (ms) => {
  await vi.advanceTimersByTimeAsync(ms);
};

beforeEach(() => {
  vi.useFakeTimers();
  localStorage.clear();
  finishOnboarding();
});

afterEach(() => {
  finishOnboarding();
  document.querySelectorAll('[data-tour]').forEach((el) => el.remove());
  vi.useRealTimers();
});

describe('onboarding queue', () => {
  it('starts only after sign-up queued it, and only once', () => {
    startQueuedOnboarding();
    expect(onboarding.isActive).toBe(false);

    queueOnboarding();
    startQueuedOnboarding();
    expect(onboarding.isActive).toBe(true);

    finishOnboarding();
    startQueuedOnboarding();
    expect(onboarding.isActive).toBe(false);
  });
});

describe('OnboardingTour', () => {
  it('skips a tip whose element is not on screen', async () => {
    placeAnchor('node-info');
    render(OnboardingTour);
    startOnboarding();
    await frames(50);
    expect(screen.getByText('Welcome to Warpnet')).toBeInTheDocument();

    // The network banner shows on testnet only; here it is absent.
    await fireEvent.click(screen.getByText('Next'));
    await frames(1000);

    expect(onboarding.index).toBe(onboardingSteps.findIndex((s) => s.anchor === 'node-info'));
    expect(screen.getByText('Your node')).toBeInTheDocument();
  });

  it('walks back over the same missing tip', async () => {
    placeAnchor('node-info');
    render(OnboardingTour);
    startOnboarding();
    await frames(50);
    await fireEvent.click(screen.getByText('Next'));
    await frames(1000);

    await fireEvent.click(screen.getByText('Back'));
    await frames(1000);

    expect(onboarding.index).toBe(0);
  });

  it('closes on Skip and does not come back', async () => {
    queueOnboarding();
    render(OnboardingTour);
    startQueuedOnboarding();
    await frames(50);

    await fireEvent.click(screen.getByText('Skip tour'));

    expect(onboarding.isActive).toBe(false);
    expect(screen.queryByTestId('onboarding')).not.toBeInTheDocument();
    startQueuedOnboarding();
    expect(onboarding.isActive).toBe(false);
  });
});

import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    signInUser: vi.fn(),
    isFirstRun: vi.fn(),
  },
}));

import Root from '@/views/Root.vue';
import { warpnetService } from '@/service/service';

const routerPush = vi.fn();

const renderRoot = ({ firstRun = true } = {}) => {
  warpnetService.isFirstRun.mockResolvedValue(firstRun);
  return render(Root, {
    global: {
      mocks: {
        $router: { push: routerPush },
      },
      directives: { escape: () => {} },
      stubs: {
        LogInComponent: { template: '<div data-testid="login-stub">login</div>' },
        ProgressBar: true,
      },
    },
  });
};

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
  routerPush.mockClear();
  warpnetService.signInUser.mockResolvedValue(undefined);
  warpnetService.isFirstRun.mockResolvedValue(true);
  sessionStorage.clear();
});

describe('Root.vue', () => {
  it('renders the marketing copy on the left panel', async () => {
    renderRoot({ firstRun: true });

    expect(
      await screen.findByText(/Follow your interests/i)
    ).toBeInTheDocument();
    expect(screen.getByText(/Join the conversation/i)).toBeInTheDocument();
  });

  it('shows the Sign up button on first run', async () => {
    renderRoot({ firstRun: true });

    expect(
      await screen.findByRole('button', { name: /^Sign up$/ })
    ).toBeInTheDocument();
    expect(screen.queryByTestId('login-stub')).not.toBeInTheDocument();
  });

  it('shows the log-in component when not first run', async () => {
    renderRoot({ firstRun: false });

    expect(await screen.findByTestId('login-stub')).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /^Sign up$/ })
    ).not.toBeInTheDocument();
  });

  it('opens step 1 of the sign-up modal when Sign up is clicked', async () => {
    renderRoot({ firstRun: true });

    await fireEvent.click(
      await screen.findByRole('button', { name: /^Sign up$/ })
    );

    expect(await screen.findByText(/Create your account/i)).toBeInTheDocument();
    expect(screen.getByText(/Step 1 of 4/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Username/i)).toBeInTheDocument();
  });

  it('keeps the Next button disabled in step 1 while username is empty (disabled state)', async () => {
    renderRoot({ firstRun: true });

    await fireEvent.click(
      await screen.findByRole('button', { name: /^Sign up$/ })
    );

    const nextBtn = await screen.findByRole('button', { name: 'Next' });
    expect(nextBtn).toBeDisabled();
  });

  it('advances through the sign-up flow and calls signInUser on final Sign up', async () => {
    renderRoot({ firstRun: true });

    // Open modal
    await fireEvent.click(
      await screen.findByRole('button', { name: /^Sign up$/ })
    );

    // Step 1: username
    const usernameField = await screen.findByLabelText(/Username/i);
    await fireEvent.update(usernameField, 'alice');
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));

    // Step 2: three checkboxes, then Next appears
    const checkboxes = await screen.findAllByRole('checkbox');
    expect(checkboxes).toHaveLength(3);
    for (const cb of checkboxes) {
      await fireEvent.click(cb);
    }
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Next' })
    );

    // Step 3: password + confirm
    const passwordField = await screen.findByLabelText('Password');
    const passwordConfirmField = await screen.findByLabelText('Confirm password');
    await fireEvent.update(passwordField, 's3cret');
    await fireEvent.update(passwordConfirmField, 's3cret');
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));

    // Step 4: final Sign up (two "Sign up" buttons exist now — the landing one
    // and the modal one; the latter appears after the flow reaches step 4).
    await screen.findByText(/Step 4 of 4/i);
    const signUpButtons = screen.getAllByRole('button', { name: /^Sign up$/ });
    await fireEvent.click(signUpButtons[signUpButtons.length - 1]);

    await waitFor(() => {
      expect(warpnetService.signInUser).toHaveBeenCalledWith({
        username: 'alice',
        password: 's3cret',
      });
      expect(routerPush).toHaveBeenCalledWith({ name: 'Home' });
    });
  });

  it('shows a sign-up error when signInUser rejects (error state)', async () => {
    warpnetService.signInUser.mockRejectedValueOnce(new Error('Already taken'));

    renderRoot({ firstRun: true });
    await fireEvent.click(
      await screen.findByRole('button', { name: /^Sign up$/ })
    );

    // Step 1
    await fireEvent.update(await screen.findByLabelText(/Username/i), 'alice');
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));

    // Step 2
    const checkboxes = await screen.findAllByRole('checkbox');
    for (const cb of checkboxes) await fireEvent.click(cb);
    await fireEvent.click(await screen.findByRole('button', { name: 'Next' }));

    // Step 3
    await fireEvent.update(
      await screen.findByLabelText('Password'),
      's3cret'
    );
    await fireEvent.update(
      await screen.findByLabelText('Confirm password'),
      's3cret'
    );
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));

    // Step 4
    await screen.findByText(/Step 4 of 4/i);
    const signUpButtons = screen.getAllByRole('button', { name: /^Sign up$/ });
    await fireEvent.click(signUpButtons[signUpButtons.length - 1]);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(/Already taken/);
    expect(routerPush).not.toHaveBeenCalled();
  });

  it('keeps step 3 Next disabled while password and confirm-password differ', async () => {
    renderRoot({ firstRun: true });

    await fireEvent.click(
      await screen.findByRole('button', { name: /^Sign up$/ })
    );
    await fireEvent.update(await screen.findByLabelText(/Username/i), 'alice');
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));

    const checkboxes = await screen.findAllByRole('checkbox');
    for (const cb of checkboxes) await fireEvent.click(cb);
    await fireEvent.click(await screen.findByRole('button', { name: 'Next' }));

    const passwordField = await screen.findByLabelText('Password');
    const passwordConfirmField = await screen.findByLabelText('Confirm password');
    await fireEvent.update(passwordField, 's3cret');
    await fireEvent.update(passwordConfirmField, 'mismatch');

    const nextBtn = await screen.findByRole('button', { name: 'Next' });
    expect(nextBtn).toBeDisabled();
    expect(await screen.findByRole('alert')).toHaveTextContent(/do not match/i);

    await fireEvent.update(passwordConfirmField, 's3cret');
    expect(nextBtn).not.toBeDisabled();
  });

  // ----- pairing onboarding flag -----
  //
  // Root.vue signals SideNav to auto-open the pairing dialog by
  // writing a sessionStorage key after a successful first-run signup.
  // Drive the modal sign-up to the final step so we exercise the
  // real signMeUp() handler.
  const completeSignUp = async ({ username = 'alice', password = 's3cret' } = {}) => {
    await fireEvent.click(
      await screen.findByRole('button', { name: /^Sign up$/ })
    );
    await fireEvent.update(await screen.findByLabelText(/Username/i), username);
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));

    const checkboxes = await screen.findAllByRole('checkbox');
    for (const cb of checkboxes) await fireEvent.click(cb);
    await fireEvent.click(await screen.findByRole('button', { name: 'Next' }));

    await fireEvent.update(await screen.findByLabelText('Password'), password);
    await fireEvent.update(await screen.findByLabelText('Confirm password'), password);
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));

    await screen.findByText(/Step 4 of 4/i);
    const signUpButtons = screen.getAllByRole('button', { name: /^Sign up$/ });
    await fireEvent.click(signUpButtons[signUpButtons.length - 1]);
  };

  it('writes the pairing-onboarding flag after a first-run signup', async () => {
    renderRoot({ firstRun: true });
    await completeSignUp();

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({ name: 'Home' });
    });
    expect(sessionStorage.getItem('warpnet:show-pairing-onboarding')).toBe('1');
  });

  it('does not write the pairing-onboarding flag when signInUser rejects', async () => {
    warpnetService.signInUser.mockRejectedValueOnce(new Error('Already taken'));
    renderRoot({ firstRun: true });
    await completeSignUp();

    expect(await screen.findByRole('alert')).toHaveTextContent(/Already taken/);
    expect(sessionStorage.getItem('warpnet:show-pairing-onboarding')).toBeNull();
  });

  it('completes signup even when sessionStorage.setItem throws (privacy mode)', async () => {
    const origSetItem = sessionStorage.setItem.bind(sessionStorage);
    sessionStorage.setItem = vi.fn(() => {
      throw new Error('storage disabled');
    });
    try {
      renderRoot({ firstRun: true });
      await completeSignUp();

      await waitFor(() => {
        expect(routerPush).toHaveBeenCalledWith({ name: 'Home' });
      });
    } finally {
      sessionStorage.setItem = origSetItem;
    }
  });

  it('falls back to the log-in component when isFirstRun rejects', async () => {
    warpnetService.isFirstRun.mockRejectedValueOnce(new Error('boom'));
    renderRoot({ firstRun: true });

    expect(await screen.findByTestId('login-stub')).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /^Sign up$/ })
    ).not.toBeInTheDocument();
  });

  it('toggles the password reveal button between password and text input', async () => {
    renderRoot({ firstRun: true });

    await fireEvent.click(
      await screen.findByRole('button', { name: /^Sign up$/ })
    );
    await fireEvent.update(await screen.findByLabelText(/Username/i), 'alice');
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    const checkboxes = await screen.findAllByRole('checkbox');
    for (const cb of checkboxes) await fireEvent.click(cb);
    await fireEvent.click(await screen.findByRole('button', { name: 'Next' }));

    const passwordField = await screen.findByLabelText('Password');
    const passwordConfirmField = await screen.findByLabelText('Confirm password');
    expect(passwordField).toHaveAttribute('type', 'password');
    expect(passwordConfirmField).toHaveAttribute('type', 'password');

    await fireEvent.click(screen.getByRole('button', { name: /Reveal password/i }));
    expect(passwordField).toHaveAttribute('type', 'text');
    expect(passwordConfirmField).toHaveAttribute('type', 'text');
  });
});

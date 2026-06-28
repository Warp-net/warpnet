import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    signInUser: vi.fn(),
  },
}));

import LogInComponent from '@/components/LogInComponent.vue';
import { warpnetService } from '@/service/service';

const routerPush = vi.fn();

const renderLogin = () =>
  render(LogInComponent, {
    global: {
      mocks: {
        $router: { push: routerPush },
      },
      stubs: { ProgressBar: true },
    },
  });

let consoleSpy;
beforeAll(() => {
  consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
});
afterAll(() => {
  consoleSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.signInUser.mockResolvedValue(undefined);
});

describe('LogInComponent', () => {
  it('renders username and password fields with a login button', () => {
    renderLogin();

    expect(screen.getByLabelText(/Username/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Log in/i })).toBeInTheDocument();
  });

  it('starts with empty inputs and no error visible', () => {
    renderLogin();

    expect(screen.getByLabelText(/Username/i)).toHaveValue('');
    expect(screen.getByLabelText(/Password/i)).toHaveValue('');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('lets the user type into the username and password fields', async () => {
    renderLogin();

    const username = screen.getByLabelText(/Username/i);
    const password = screen.getByLabelText(/Password/i);

    await fireEvent.update(username, 'alice');
    await fireEvent.update(password, 's3cret');

    expect(username).toHaveValue('alice');
    expect(password).toHaveValue('s3cret');
  });

  it('uses a password input type for the password field', () => {
    renderLogin();

    expect(screen.getByLabelText(/Password/i)).toHaveAttribute(
      'type',
      'password'
    );
  });

  it('calls signInUser with the entered credentials when Log in is clicked', async () => {
    renderLogin();

    await fireEvent.update(screen.getByLabelText(/Username/i), 'alice');
    await fireEvent.update(screen.getByLabelText(/Password/i), 's3cret');
    await fireEvent.click(screen.getByRole('button', { name: /Log in/i }));

    await waitFor(() => {
      expect(warpnetService.signInUser).toHaveBeenCalledWith({
        username: 'alice',
        password: 's3cret',
      });
    });
  });

  it('navigates to Home on successful sign in', async () => {
    renderLogin();

    await fireEvent.update(screen.getByLabelText(/Username/i), 'alice');
    await fireEvent.update(screen.getByLabelText(/Password/i), 's3cret');
    await fireEvent.click(screen.getByRole('button', { name: /Log in/i }));

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({ name: 'Home' });
    });
  });

  it('submits when the user presses Enter in the password field', async () => {
    renderLogin();

    await fireEvent.update(screen.getByLabelText(/Username/i), 'alice');
    const password = screen.getByLabelText(/Password/i);
    await fireEvent.update(password, 's3cret');
    await fireEvent.keyDown(password, { key: 'Enter', code: 'Enter' });

    await waitFor(() => {
      expect(warpnetService.signInUser).toHaveBeenCalledWith({
        username: 'alice',
        password: 's3cret',
      });
    });
  });

  it('hides the login button while the sign-in request is in flight', async () => {
    let resolveSignIn;
    warpnetService.signInUser.mockReturnValue(
      new Promise((resolve) => {
        resolveSignIn = resolve;
      })
    );

    renderLogin();

    await fireEvent.update(screen.getByLabelText(/Username/i), 'alice');
    await fireEvent.update(screen.getByLabelText(/Password/i), 's3cret');
    await fireEvent.click(screen.getByRole('button', { name: /Log in/i }));

    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: /Log in/i })
      ).not.toBeInTheDocument();
    });

    resolveSignIn();
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Log in/i })
      ).toBeInTheDocument();
    });
  });

  it('shows an error alert when sign-in fails and restores the login button', async () => {
    warpnetService.signInUser.mockRejectedValue(new Error('Invalid password'));

    renderLogin();

    await fireEvent.update(screen.getByLabelText(/Username/i), 'alice');
    await fireEvent.update(screen.getByLabelText(/Password/i), 'wrong');
    await fireEvent.click(screen.getByRole('button', { name: /Log in/i }));

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(/Invalid password/);
    expect(routerPush).not.toHaveBeenCalled();
    expect(
      screen.getByRole('button', { name: /Log in/i })
    ).toBeInTheDocument();
  });

  it('falls back to a generic error message when the error has no message', async () => {
    warpnetService.signInUser.mockRejectedValue({});

    renderLogin();

    await fireEvent.click(screen.getByRole('button', { name: /Log in/i }));

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(/Unknown error during sign-in/i);
  });

  it('dismisses the error alert when the user clicks Dismiss', async () => {
    warpnetService.signInUser.mockRejectedValue(new Error('Invalid password'));

    renderLogin();

    await fireEvent.click(screen.getByRole('button', { name: /Log in/i }));

    const alert = await screen.findByRole('alert');
    expect(alert).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: /Dismiss/i }));

    await waitFor(() => {
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    });
  });

  it('hides any previous error when submitting again', async () => {
    warpnetService.signInUser.mockRejectedValueOnce(
      new Error('Invalid password')
    );

    renderLogin();

    await fireEvent.click(screen.getByRole('button', { name: /Log in/i }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();

    // Second attempt succeeds and keeps the UI in flight until resolved.
    let resolveSignIn;
    warpnetService.signInUser.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveSignIn = resolve;
      })
    );

    await fireEvent.click(screen.getByRole('button', { name: /Log in/i }));

    await waitFor(() => {
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    });

    resolveSignIn();
  });

  it('submits with empty credentials and forwards them unchanged (edge case)', async () => {
    renderLogin();

    await fireEvent.click(screen.getByRole('button', { name: /Log in/i }));

    await waitFor(() => {
      expect(warpnetService.signInUser).toHaveBeenCalledWith({
        username: '',
        password: '',
      });
    });
  });
});

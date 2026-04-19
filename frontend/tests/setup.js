import { vi } from 'vitest';
import '@testing-library/jest-dom/vitest';

// Stub the Wails runtime bridge so components that transitively import it
// don't blow up when loaded in jsdom.
vi.mock('../wailsjs/go/main/App', () => ({
  Call: vi.fn().mockResolvedValue({ body: {} }),
}));

vi.mock('@/lib/backend-mock', () => ({}));

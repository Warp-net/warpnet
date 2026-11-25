import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/vue';
import ReportDialog from '@/components/ReportDialog.vue';

const NODE_REASON_LIMIT = 256;

describe('ReportDialog', () => {
  it('never composes a reason longer than the node accepts', async () => {
    const { getAllByRole, getByRole, emitted } = render(ReportDialog, { props: { show: true } });
    for (const box of getAllByRole('checkbox')) {
      await fireEvent.click(box);
    }
    const details = getByRole('textbox');
    const maxLength = Number(details.getAttribute('maxlength'));
    await fireEvent.update(details, 'x'.repeat(maxLength));
    await fireEvent.click(getByRole('button', { name: 'Submit report' }));

    const [[reason]] = emitted().submit;
    expect(reason.endsWith('x'.repeat(maxLength))).toBe(true);
    expect(reason.length).toBeLessThanOrEqual(NODE_REASON_LIMIT);
  });

  it('blocks submit when a later selection pushes the reason past the limit', async () => {
    const { getAllByRole, getByRole } = render(ReportDialog, { props: { show: true } });
    const [first, ...rest] = getAllByRole('checkbox');
    await fireEvent.click(first);
    const details = getByRole('textbox');
    await fireEvent.update(details, 'x'.repeat(Number(details.getAttribute('maxlength'))));
    for (const box of rest) {
      await fireEvent.click(box);
    }
    expect(getByRole('button', { name: 'Submit report' }).disabled).toBe(true);
  });
});

import { describe, it, expect, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import ProgressBar from './ProgressBar.vue';
import * as api from '../api';

vi.mock('../api', () => ({
  EventsOn: vi.fn(),
}));

describe('ProgressBar.vue', () => {
  it('updates on scan_progress event', async () => {
    let progressCallback: (data: any) => void = () => {};
    vi.mocked(api.EventsOn).mockImplementation((event, cb) => {
      if (event === 'scan_progress') {
        progressCallback = cb;
      }
    });

    const wrapper = mount(ProgressBar, {
      props: { scanning: true }
    });

    await wrapper.vm.$nextTick();

    // Trigger update
    progressCallback({
      phase: 'scanning',
      current: 50,
      total: 100,
      last_file: '/tmp/file.mp4',
      duration_seconds: 10
    });

    await wrapper.vm.$nextTick();

    expect(wrapper.text()).toContain('Scanned: 50 / 100');
    expect(wrapper.text()).toContain('/tmp/file.mp4');
    expect(wrapper.find('.pct').text()).toContain('50%');
  });

  it('shows indeterminate state for discovery phase', async () => {
    let progressCallback: (data: any) => void = () => {};
    vi.mocked(api.EventsOn).mockImplementation((event, cb) => {
        if (event === 'scan_progress') progressCallback = cb;
    });

    const wrapper = mount(ProgressBar, { props: { scanning: true } });
    await wrapper.vm.$nextTick();

    progressCallback({
        phase: 'discovery',
        current: 0,
        total: 1000,
        last_file: '',
        duration_seconds: 1
    });
    await wrapper.vm.$nextTick();

    expect(wrapper.text()).toContain('Discovering files');
    const fill = wrapper.find('.progress-fill');
    expect(fill.classes()).toContain('indeterminate');
  });
});

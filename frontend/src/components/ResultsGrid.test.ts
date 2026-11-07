import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import ResultsGrid from './ResultsGrid.vue';
import * as api from '../api';

vi.mock('../api', () => ({
  GetResults: vi.fn(),
  GetThumbnails: vi.fn(),
  GetSettings: vi.fn(),
  ExcludeGroup: vi.fn(),
  DeleteFiles: vi.fn(),
  OpenFile: vi.fn(),
  RenameFile: vi.fn(),
  GetStreamUrl: vi.fn(),
  EventsOn: vi.fn(),
  getConnectionConfig: vi.fn(() => ({ mode: 'local', url: '' })),
}));

describe('ResultsGrid.vue selection labels', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.GetSettings).mockResolvedValue({
      auto_fetch_thumbnails: false,
      thumbnails: 4,
      show_thumbnails: true,
      show_similarity: true,
      show_media_info: true,
    } as any);
    vi.mocked(api.GetResults).mockResolvedValue({
      groups: [
        {
          id: 1,
          files: [
            { path: '/a.mp4', size: 1000, duration: 10, width: 1920, height: 1080, similarity: 100 },
            { path: '/b.mp4', size: 2000, duration: 10, width: 1920, height: 1080, similarity: 100 },
          ],
        },
      ],
      total_groups: 1,
      total_files: 2,
    } as any);
  });

  it('renders correctly worded selection buttons', async () => {
    const wrapper = mount(ResultsGrid, {
      global: {
        provide: { showModal: vi.fn() },
      },
      props: { preview: false, scanning: false },
    });

    await new Promise((resolve) => setTimeout(resolve, 20));
    await wrapper.vm.$nextTick();

    const buttons = wrapper.findAll('.dropdown-content button');
    const buttonTexts = buttons.map((b) => b.text());

    expect(buttonTexts).toContain('Select Best (Largest)');
    expect(buttonTexts).toContain('Select Lowest Quality (Keep largest)');
    expect(buttonTexts).not.toContain('Select Best (Keep largest)');
  });
});


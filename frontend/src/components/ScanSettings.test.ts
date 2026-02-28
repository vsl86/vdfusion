import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import ScanSettings from './ScanSettings.vue';
import * as api from '../api';

// Mock the API module
vi.mock('../api', () => ({
  GetSettings: vi.fn(),
  SaveSettings: vi.fn(),
  ResetDB: vi.fn(),
  CleanupDB: vi.fn(),
}));

const mockSettings = {
  include_list: ['/tmp/videos'],
  black_list: [],
  percent: 96,
  percent_duration_difference: 20,
  duration_difference_min_seconds: 0,
  duration_difference_max_seconds: 3600,
  thumbnails: 4,
  concurrency: 4,
  auto_fetch_thumbnails: true,
  filter_by_file_size: false,
  minimum_file_size: 0,
  maximum_file_size: 0
};

describe('ScanSettings.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.GetSettings).mockResolvedValue({ ...mockSettings });
    vi.mocked(api.SaveSettings).mockResolvedValue(undefined);
  });

  it('renders correctly and loads settings', async () => {
    const wrapper = mount(ScanSettings, {
      global: {
        provide: { showModal: vi.fn() },
        stubs: { DirPicker: true, NumberInput: true }
      },
      props: { compact: false }
    });
    
    // Allow onMounted async calls to finish
    await new Promise(resolve => setTimeout(resolve, 10));
    await wrapper.vm.$nextTick();

    expect(api.GetSettings).toHaveBeenCalled();
    expect(wrapper.text()).toContain('Scan Configuration');
    
    // Check if input value matches loaded settings
    const rangeInputs = wrapper.findAll('input[type="range"]');
    // First range is similarity (percent)
    expect((rangeInputs[0].element as HTMLInputElement).value).toBe('96');
  });

  it('updates settings and enables save button', async () => {
    const wrapper = mount(ScanSettings, {
      global: {
        provide: { showModal: vi.fn() },
        stubs: { DirPicker: true, NumberInput: true }
      },
      props: { compact: false }
    });
    
    await new Promise(resolve => setTimeout(resolve, 10));
    await wrapper.vm.$nextTick();
    
    const rangeInput = wrapper.findAll('input[type="range"]')[0];
    await rangeInput.setValue(90);
    
    // Check save button state
    const saveBtn = wrapper.find('.save-btn');
    expect(saveBtn.exists()).toBe(true);
    expect(saveBtn.attributes('disabled')).toBeUndefined();
    
    await saveBtn.trigger('click');
    
    expect(api.SaveSettings).toHaveBeenCalledWith(expect.objectContaining({
      percent: 90
    }));
  });

  it('handles database reset', async () => {
    const showModalMock = vi.fn().mockResolvedValue(true);
    const wrapper = mount(ScanSettings, {
      global: {
        provide: { 
            showModal: showModalMock 
        },
        stubs: { DirPicker: true, NumberInput: true }
      },
      props: { compact: false }
    });

    await new Promise(resolve => setTimeout(resolve, 10));
    
    const resetBtn = wrapper.findAll('button.bl-del-btn.danger')[0];
    await resetBtn.trigger('click');
    
    // Since showModal is mocked to resolve true, it should call ResetDB
    expect(showModalMock).toHaveBeenCalled();
    
    // Wait for async action
    await new Promise(resolve => setTimeout(resolve, 10));
    
    expect(api.ResetDB).toHaveBeenCalled();
  });
});

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
  GetStats: vi.fn(),
  GetDebugInfo: vi.fn(() => Promise.resolve({ version: '1.0.0' })),
  getConnectionConfig: vi.fn(() => ({ mode: 'local', url: '' })),
  setConnectionConfig: vi.fn(),
  CheckForUpdates: vi.fn(),
  TestNeuralBackend: vi.fn(),
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
  maximum_file_size: 0,
  neural_backend_enabled: false,
  neural_backend_url: 'http://127.0.0.1:8765',
  neural_phash_gate: 60
};

describe('ScanSettings.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.GetSettings).mockResolvedValue({ ...mockSettings });
    vi.mocked(api.SaveSettings).mockResolvedValue(undefined);
    vi.mocked(api.GetStats).mockResolvedValue({
      total_files: 10,
      total_size: 1000,
      total_duration: 100,
      suspicious_count: 0,
      max_thumbnails: 21,
    });
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
    const saveBtn = wrapper.find('button.action-btn.primary');
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
    
    // Find the Reset Database button (second .bl-del-btn.danger in DB management)
    const buttons = wrapper.findAll('button.bl-del-btn.danger');
    const resetBtn = buttons.find(b => b.text().includes('Reset'));
    expect(resetBtn).toBeDefined();
    await resetBtn!.trigger('click');
    
    // Since showModal is mocked to resolve true, it should call ResetDB
    expect(showModalMock).toHaveBeenCalled();
    
    // Wait for async action
    await new Promise(resolve => setTimeout(resolve, 20));
    
    expect(api.ResetDB).toHaveBeenCalled();
  });

  it('shows instant recalculation when changing thumbnails back to <= max existing in DB', async () => {
    // DB has files with up to 21 thumbnails
    vi.mocked(api.GetSettings).mockResolvedValue({ ...mockSettings, thumbnails: 10 });
    vi.mocked(api.GetStats).mockResolvedValue({
      total_files: 50,
      total_size: 50000,
      total_duration: 1200,
      suspicious_count: 0,
      max_thumbnails: 21
    });

    const wrapper = mount(ScanSettings, {
      global: {
        provide: { showModal: vi.fn() },
        stubs: { DirPicker: true, NumberInput: true }
      },
      props: { compact: false }
    });

    await new Promise(resolve => setTimeout(resolve, 20));
    await wrapper.vm.$nextTick();

    // User changes thumbnails from 10 to 21
    wrapper.vm.settings.thumbnails = 21;
    await wrapper.vm.$nextTick();

    // scanImpact should say Instant Recalculation because 21 is already in DB
    expect(wrapper.vm.scanImpact).not.toBeNull();
    expect(wrapper.vm.scanImpact.title).toBe('Instant Recalculation');
    expect(wrapper.vm.scanImpact.message).toContain('No new extraction needed');
    expect(wrapper.vm.scanImpact.message).toContain('first 21 existing hashes');

    // If changed above 21 (e.g. to 25), it should show Partial Rescan for only 4 frames
    wrapper.vm.settings.thumbnails = 25;
    await wrapper.vm.$nextTick();

    expect(wrapper.vm.scanImpact.title).toBe('Partial Rescan (Incremental)');
    expect(wrapper.vm.scanImpact.message).toContain('Only 4 new frame(s)');
    expect(wrapper.vm.scanImpact.message).toContain('Existing 21 hashes will be reused');
  });

  it('renders and updates neural_phash_gate setting', async () => {
    vi.mocked(api.GetSettings).mockResolvedValue({
      ...mockSettings,
      neural_backend_enabled: true,
      neural_phash_gate: 60,
    });

    const wrapper = mount(ScanSettings, {
      global: {
        provide: { showModal: vi.fn() },
        stubs: { DirPicker: true, NumberInput: true }
      },
      props: { compact: false }
    });

    await new Promise(resolve => setTimeout(resolve, 20));
    await wrapper.vm.$nextTick();

    expect(wrapper.text()).toContain('pHash Pre-Filter Gate');
    expect(wrapper.vm.settings.neural_phash_gate).toBe(60);

    // Update the gate to 25
    wrapper.vm.settings.neural_phash_gate = 25;
    await wrapper.vm.$nextTick();

    const saveBtn = wrapper.find('button.action-btn.primary');
    await saveBtn.trigger('click');

    expect(api.SaveSettings).toHaveBeenCalledWith(expect.objectContaining({
      neural_phash_gate: 25
    }));
  });
});

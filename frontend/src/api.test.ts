import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import * as WailsApp from '../wailsjs/go/main/App';

// We need to mock the module before importing api.ts
vi.mock('../wailsjs/go/main/App', () => ({
    StartScan: vi.fn(),
    StopScan: vi.fn(),
    GetSettings: vi.fn(),
    SaveSettings: vi.fn(),
    GetResults: vi.fn(),
    GetThumbnails: vi.fn(),
    DeleteFiles: vi.fn(),
    ExcludeGroup: vi.fn(),
    GetIgnoredGroups: vi.fn(),
    DeleteIgnoredGroup: vi.fn(),
    RenameFile: vi.fn(),
    OpenFile: vi.fn(),
    ResetDB: vi.fn(),
    CleanupDB: vi.fn(),
    GetSuspiciousFiles: vi.fn(),
}));

describe('API Service (Wails Mode)', () => {
    beforeEach(() => {
        // Mock window.go to simulate Wails environment
        window.go = { main: { App: {} } };
        window.runtime = { EventsOn: vi.fn() };
        vi.resetModules();
    });

    afterEach(() => {
        delete window.go;
        delete window.runtime;
        vi.clearAllMocks();
    });

    it('StartScan calls WailsApp.StartScan', async () => {
        // We need to re-import api to pick up the window.go change
        const api = await import('./api');
        const paths = ['/tmp'];
        
        await api.StartScan(paths);
        expect(WailsApp.StartScan).toHaveBeenCalledWith(paths);
    });

    it('StopScan calls WailsApp.StopScan', async () => {
        const api = await import('./api');
        await api.StopScan();
        expect(WailsApp.StopScan).toHaveBeenCalled();
    });

    it('GetSettings calls WailsApp.GetSettings', async () => {
        const api = await import('./api');
        const mockSettings = { include_list: [] } as any;
        vi.mocked(WailsApp.GetSettings).mockResolvedValue(mockSettings);

        const result = await api.GetSettings();
        expect(WailsApp.GetSettings).toHaveBeenCalled();
        expect(result).toBe(mockSettings);
    });
});

describe('API Service (Web Mode)', () => {
    beforeEach(() => {
        // Ensure window.go is undefined
        delete window.go;
        delete window.runtime;
        vi.resetModules();
        
        // Mock global fetch
        global.fetch = vi.fn();
    });

    afterEach(() => {
        vi.clearAllMocks();
    });

    it('StartScan calls fetch', async () => {
        const api = await import('./api');
        const paths = ['/tmp'];
        
        vi.mocked(global.fetch).mockResolvedValue({
            json: async () => ({})
        } as Response);

        await api.StartScan(paths);
        
        expect(global.fetch).toHaveBeenCalledWith('/api/scan/start', expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({ paths })
        }));
    });

    it('GetSettings calls fetch', async () => {
        const api = await import('./api');
        const mockSettings = { include_list: [] };
        
        vi.mocked(global.fetch).mockResolvedValue({
            json: async () => mockSettings
        } as Response);

        const result = await api.GetSettings();
        
        expect(global.fetch).toHaveBeenCalledWith('/api/settings');
        expect(result).toEqual(mockSettings);
    });
});

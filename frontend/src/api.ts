import * as WailsApp from '../wailsjs/go/main/App';
import { EventsOn as WailsEventsOn } from '../wailsjs/runtime/runtime';
import { config, db, engine, main } from '../wailsjs/go/models';

declare global {
    interface Window {
        go: any;
        runtime: any;
    }
}

import { getDatabase, syncSession } from './db';

const isWailsApp = !!window.go;

export interface ConnectionConfig {
    mode: 'local' | 'remote';
    url: string;
}

export function getConnectionConfig(): ConnectionConfig {
    const saved = localStorage.getItem('vdf_connection_config');
    if (saved) {
        try {
            return JSON.parse(saved);
        } catch (e) {
            console.error('Failed to parse connection config', e);
        }
    }
    return { mode: isWailsApp ? 'local' : 'remote', url: isWailsApp ? '' : `${window.location.protocol}//${window.location.host}` };
}

export function setConnectionConfig(config: ConnectionConfig) {
    localStorage.setItem('vdf_connection_config', JSON.stringify(config));
    window.location.reload(); // Re-initialize everything
}

const connConfig = getConnectionConfig();
const isWails = connConfig.mode === 'local' && isWailsApp;

function normalizeUrl(url: string) {
    if (!url) return "";
    let normalized = url.trim();
    if (!/^https?:\/\//i.test(normalized)) {
        normalized = `http://${normalized}`;
    }
    return normalized.replace(/\/$/, '');
}

const apiBase = connConfig.mode === 'remote' ? normalizeUrl(connConfig.url) : "";

// Initialization is lazy via ensureSessionSynced()
let sessionSyncPromise: Promise<void> | null = null;
async function ensureSessionSynced() {
    if (sessionSyncPromise) return sessionSyncPromise;
    sessionSyncPromise = (async () => {
        try {
            if (!isWails && !apiBase) {
                console.log('RxDB: API not ready, skipping session sync');
                return;
            }
            const dbInfo = await GetDebugInfo();
            const instId = dbInfo.instance_id || 'default';
            console.log(`RxDB: Starting session sync for ${instId}`);
            await syncSession(instId);
        } catch (e) {
            console.error('RxDB: Failed to sync session', e);
            sessionSyncPromise = null; // Allow retry
        }
    })();
    return sessionSyncPromise;
}

async function generateLogId(type: string, data: any): Promise<string> {
    const content = type + JSON.stringify(data);
    const msgUint8 = new TextEncoder().encode(content);
    const hashBuffer = await crypto.subtle.digest('SHA-256', msgUint8);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    const hashHex = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
    return `${type.startsWith('app') ? 'app' : 'sys'}_${hashHex}`;
}

export function GetStreamUrl(path: string): string {
    return `${apiBase}/api/files/stream?path=${encodeURIComponent(path)}`;
}

async function saveLog(type: string, data: any) {
    try {
        await ensureSessionSynced();
        const db = await getDatabase();
        if (type === 'app_log') {
            await db.activity_logs.upsert({
                id: await generateLogId('app_log', data),
                time: data.time || new Date().toLocaleTimeString(),
                severity: data.severity || 'info',
                message: data.message
            });
            return true;
        } else if (type === 'system_log') {
            const line = typeof data === 'string' ? data : data.line;
            const l = line.toLowerCase();

            const isHttp = /"(GET|POST|PUT|DELETE|PATCH) .* HTTP\/[12]\.\d"/.test(line);
            const isNonCritical = /\s([1-4]\d\d)\s/.test(line) || line.includes('- 404');
            const isError = l.includes('error') || l.includes('failed') || l.includes('panic') || l.includes(' 50');

            if (isHttp && (isNonCritical || l.includes('favicon.ico')) && !isError) {
                return false;
            }

            if (l.includes('broken pipe') || l.includes('ws write error')) {
                return false;
            }

            await db.system_logs.upsert({
                id: await generateLogId('system_log', line),
                time: new Date().toLocaleTimeString(),
                line: line
            });
            return true;
        }
    } catch (e) {
        console.error('RxDB: Failed to save log', e);
    }
    return true;
}

const eventCallbacks: Record<string, Function[]> = {};
let ws: WebSocket | null = null;
let reconnectTimer: any = null;

function getWsUrl(config: any) {
    const remoteUrl = config.mode === 'remote' ? normalizeUrl(config.url) : `${window.location.protocol}//${window.location.host}`;
    if (!remoteUrl || remoteUrl === 'http://') return null;
    return remoteUrl.replace(/^http/, 'ws') + '/ws';
}

if (!isWails) {
    const wsUrl = getWsUrl(connConfig);

    const connectWS = () => {
        if (!wsUrl || wsUrl === '/ws') return;
        try {
            ws = new WebSocket(wsUrl);
            ws.onmessage = (event) => {
                const data = JSON.parse(event.data);
                if (data.type === "progress" && eventCallbacks["scan_progress"]) {
                    eventCallbacks["scan_progress"].forEach(cb => cb(data));
                }
                if (data.type === "app_log") {
                    saveLog('app_log', data);
                    if (eventCallbacks["app_log"]) {
                        eventCallbacks["app_log"].forEach(cb => cb(data));
                    }
                }
                if (data.type === "system_log") {
                    saveLog('system_log', data.line).then(shouldForward => {
                        if (shouldForward && eventCallbacks["system_log"]) {
                            eventCallbacks["system_log"].forEach(cb => cb(data.line));
                        }
                    });
                }
            };
            ws.onclose = () => {
                console.log("WS closed, retrying...");
                if (reconnectTimer) clearTimeout(reconnectTimer);
                reconnectTimer = setTimeout(connectWS, 2000);
            };
            ws.onerror = (e) => {
                console.error("WS error", e);
            };
        } catch (e) {
            console.error("Failed to connect WS", e);
        }
    };
    connectWS();
}
else {
    // Wails mode: Global listeners for persistence (Guarded against HMR duplicates)
    if (!(window as any)._vdf_listeners_init) {
        WailsEventsOn('app_log', (data) => {
            saveLog('app_log', data);
        });
        WailsEventsOn('system_log', (data) => {
            saveLog('system_log', data);
        });
        (window as any)._vdf_listeners_init = true;
        console.log('RxDB: Global persistence listeners initialized');
    }
}

export async function GetActivityHistory(): Promise<any[]> {
    await ensureSessionSynced();
    const db = await getDatabase();
    const docs = await db.activity_logs.find({
        sort: [{ time: 'asc' }]
    }).exec();
    return docs.map(d => ({
        time: d.time,
        severity: d.severity,
        message: d.message
    }));
}

export async function GetSystemHistory(): Promise<string[]> {
    await ensureSessionSynced();
    const db = await getDatabase();
    const docs = await db.system_logs.find({
        sort: [{ time: 'asc' }]
    }).exec();
    return docs.map(d => d.line);
}


export async function ClearPersistedLogs(): Promise<void> {
    const db = await getDatabase();
    await db.activity_logs.find().remove();
    await db.system_logs.find().remove();
}

export async function StartScan(paths: string[]): Promise<void> {
    if (isWails) return WailsApp.StartScan(paths);
    await fetch(`${apiBase}/api/scan/start`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths })
    }).then(r => r.json());
}

export async function StopScan(): Promise<void> {
    if (isWails) return WailsApp.StopScan();
    await fetch(`${apiBase}/api/scan/stop`, { method: 'POST' }).then(r => r.json());
}

export async function GetScanStatus(): Promise<{ running: boolean }> {
    if (isWails) return WailsApp.GetScanStatus().then(data => ({ running: !!data.running }));
    return fetch(`${apiBase}/api/scan/status`).then(r => r.json()).then(data => ({ running: !!data.running }));
}

export async function GetSettings(): Promise<config.Settings> {
    if (isWails) return WailsApp.GetSettings();
    return fetch(`${apiBase}/api/settings`).then(r => r.json());
}

export async function SaveSettings(settings: config.Settings): Promise<void> {
    if (isWails) return WailsApp.SaveSettings(settings);
    await fetch(`${apiBase}/api/settings`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings)
    }).then(r => r.json());
}

export async function GetResults(offset: number = 0, limit: number = 50): Promise<engine.ResultsResponse> {
    if (isWails) {
        return WailsApp.GetResults(offset, limit);
    }
    const params = new URLSearchParams({ offset: offset.toString(), limit: limit.toString() });
    return fetch(`${apiBase}/api/results?${params}`).then(r => r.json());
}

export async function GetDuplicateStats(): Promise<{ total_groups: number, total_files: number }> {
    if (isWails) return WailsApp.GetDuplicateStats();
    return fetch(`${apiBase}/api/results/stats`).then(r => r.json());
}

export async function GetThumbnails(path: string, duration: number, count: number, signal?: AbortSignal): Promise<string[]> {
    if (isWails) return WailsApp.GetThumbnails(path, duration, count);
    const params = new URLSearchParams({ path, duration: duration.toString(), count: count.toString() });
    return fetch(`${apiBase}/api/thumbnails?${params}`, { signal }).then(r => r.json());
}

export async function DeleteFiles(paths: string[]): Promise<void> {
    if (isWails) return WailsApp.DeleteFiles(paths);
    await fetch(`${apiBase}/api/files/delete`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths })
    });
}

export async function ExcludeGroup(label: string, files: string[]): Promise<void> {
    if (isWails) return WailsApp.ExcludeGroup(label, files);
    await fetch(`${apiBase}/api/exclude`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ label, files })
    }).then(r => r.json());
}

export async function GetIgnoredGroups(): Promise<db.IgnoredGroup[]> {
    if (isWails) return WailsApp.GetIgnoredGroups();
    return fetch(`${apiBase}/api/ignored-groups`).then(r => r.json());
}

export async function DeleteIgnoredGroup(id: number): Promise<void> {
    if (isWails) return WailsApp.DeleteIgnoredGroup(id);
    await fetch(`${apiBase}/api/ignored-groups/${id}`, { method: 'DELETE' });
}

export function EventsOn(eventName: string, callback: (data: any) => void) {
    if (isWails) {
        return WailsEventsOn(eventName, (data: any) => {
            if (eventName === 'system_log') {
                const line = typeof data === 'string' ? data : data.line;
                const l = line.toLowerCase();
                const isHttp = /"(GET|POST|PUT|DELETE|PATCH) .* HTTP\/[12]\.\d"/.test(line);
                const isSuccess = l.includes(' 200 ') || l.includes(' 304 ') || l.includes(' 204 ');
                const isError = l.includes('error') || l.includes('failed') || l.includes('panic') || l.includes(' 50');

                if (isHttp && (isSuccess || l.includes('favicon.ico')) && !isError) {
                    return;
                }
            }
            callback(data);
        });
    }
    if (!eventCallbacks[eventName]) {
        eventCallbacks[eventName] = [];
    }
    eventCallbacks[eventName].push(callback);
}

// Support functions that might be called
export async function RenameFile(oldPath: string, newPath: string): Promise<void> {
    if (isWails) return WailsApp.RenameFile(oldPath, newPath);
    await fetch(`${apiBase}/api/files/rename`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ oldPath, newPath })
    });
}

export async function OpenFile(path: string): Promise<void> {
    if (isWails) return WailsApp.OpenFile(path);
    await fetch(`${apiBase}/api/files/open`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path })
    });
}

export async function ResetDB(): Promise<void> {
    await ClearPersistedLogs();
    if (isWails) return WailsApp.ResetDB();
    await fetch(`${apiBase}/api/db/reset`, { method: 'POST' });
}

export async function ExportDB(): Promise<void> {
    if (isWails) return WailsApp.ExportDB();
    throw new Error('Export is only supported in desktop mode');
}

export async function SaveLogToFile(content: string): Promise<void> {
    if (isWails) return WailsApp.SaveLogToFile(content);
    // Web fallback: download as blob
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `vdfusion_logs_${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.txt`;
    a.click();
    URL.revokeObjectURL(url);
}

export async function ImportDB(): Promise<void> {
    if (isWails) return WailsApp.ImportDB();
    throw new Error('Import is only supported in desktop mode');
}

export async function CleanupDB(): Promise<number> {
    if (isWails) return WailsApp.CleanupDB();
    return fetch(`${apiBase}/api/db/cleanup`, { method: 'POST' }).then(r => r.json());
}

export async function PurgeBlacklist(): Promise<void> {
    if (isWails) return WailsApp.PurgeBlacklist();
    await fetch(`${apiBase}/api/db/purge-blacklist`, { method: 'POST' });
}

export async function ResetSettings(): Promise<void> {
    if (isWails) return WailsApp.ResetSettings();
    await fetch(`${apiBase}/api/db/reset-settings`, { method: 'POST' });
}

export async function GetSuspiciousFiles(): Promise<main.SuspiciousFile[]> {
    if (isWails) return WailsApp.GetSuspiciousFiles();
    return fetch(`${apiBase}/api/suspicious-files`).then(r => r.json());
}

export async function ListDirs(path: string): Promise<any> {
    if (isWails) return WailsApp.ListDirs(path);
    const params = new URLSearchParams({ path });
    return fetch(`${apiBase}/api/fs/ls?${params}`).then(r => r.json());
}

export async function GetStats(): Promise<any> {
    if (isWails) return WailsApp.GetStats();
    return fetch(`${apiBase}/api/stats`).then(r => r.json());
}

export async function CheckDependencies(): Promise<any> {
    if (isWails) return WailsApp.CheckDependencies();
    // In remote mode, the client doesn't strictly need local FFmpeg for core operations,
    // but the remote server does. Since this checks CLIENT state:
    return { ffmpeg: true, ffprobe: true, ffplay: true, missing: false, remote: true };
}

export async function DownloadDependencies(): Promise<void> {
    if (isWails) return WailsApp.DownloadDependencies();
    throw new Error('Download is only supported in desktop mode');
}

export async function GetDebugInfo(baseUrl?: string): Promise<any> {
    if (isWails && !baseUrl) return WailsApp.GetDebugInfo();
    const base = baseUrl ? normalizeUrl(baseUrl) : apiBase;
    return fetch(`${base}/api/debug`).then(r => r.json());
}

export async function CheckForUpdates(): Promise<{ current: string, latest: string, url: string, notes: string, update_available: boolean }> {
    if (isWails) {
    }
    return fetch(`${apiBase}/api/updates/check`).then(r => r.json());
}

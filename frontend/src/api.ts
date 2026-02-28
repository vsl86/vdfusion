import * as WailsApp from '../wailsjs/go/main/App';
import { EventsOn as WailsEventsOn } from '../wailsjs/runtime/runtime';
import { config, db, engine, main } from '../wailsjs/go/models';

declare global {
    interface Window {
        go: any;
        runtime: any;
    }
}

const isWails = !!window.go;
const apiBase = ""; // Same host by default

export function GetStreamUrl(path: string): string {
    return `${apiBase}/api/files/stream?path=${encodeURIComponent(path)}`;
}

const eventCallbacks: Record<string, Function[]> = {};
let ws: WebSocket | null = null;

if (!isWails) {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    const connectWS = () => {
        ws = new WebSocket(wsUrl);
        ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            if (data.type === "progress" && eventCallbacks["scan_progress"]) {
                eventCallbacks["scan_progress"].forEach(cb => cb(data));
            }
            if (data.type === "system_log" && eventCallbacks["system_log"]) {
                console.log("api: caught system_log event", data.line);
                eventCallbacks["system_log"].forEach(cb => cb(data.line));
            }
        };
        ws.onclose = () => {
            console.log("WS closed, retrying...");
            setTimeout(connectWS, 2000);
        };
    };
    connectWS();
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
    if (isWails) return { running: false }; // Wails events handle status updates usually, or we assume false if not tracking
    return fetch(`${apiBase}/api/scan/status`).then(r => r.json());
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
        WailsEventsOn(eventName, (data: any) => {
            console.log(`api: [Wails Event] ${eventName}`, data);
            callback(data);
        });
        return;
    }
    if (!eventCallbacks[eventName]) {
        eventCallbacks[eventName] = [];
    }
    eventCallbacks[eventName].push((data: any) => {
        console.log(`api: [Web Event] ${eventName}`, data);
        callback(data);
    });
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

export async function ListDirs(path: string): Promise<string[]> {
    if (isWails) return []; // Not implemented for Wails yet
    const params = new URLSearchParams({ path });
    return fetch(`${apiBase}/api/fs/ls?${params}`).then(r => r.json());
}

export async function GetStats(): Promise<any> {
    if (isWails) return { total_files: 0, total_size: 0, total_duration: 0, suspicious_count: 0 };
    return fetch(`${apiBase}/api/stats`).then(r => r.json());
}

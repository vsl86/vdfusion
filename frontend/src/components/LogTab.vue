<template>
    <div class="log-tab-container">
        <div class="dashboard-header" style="display: flex; justify-content: space-between; align-items: flex-end;">
            <div>
                <h1>System Logs</h1>
                <p>Real-time backend stdout/stderr stream for debugging and monitoring.</p>
            </div>
            <div class="log-actions">
                <div class="log-info"
                    style="margin-right: 12px; color: var(--text-muted); font-size: 13px; align-self: center;">
                    {{ logs.length }} lines received
                </div>
                <button class="action-btn secondary small" :class="{ active: showErrorsOnly }"
                    @click="showErrorsOnly = !showErrorsOnly">
                    {{ showErrorsOnly ? '🚨 Errors Only' : '📜 Show All' }}
                </button>
                <button class="action-btn secondary small" @click="exportLogs" :disabled="logs.length === 0">
                    📥 Export Logs
                </button>
                <button class="action-btn danger small" @click="clearLogs" :disabled="logs.length === 0">
                    🗑 Clear
                </button>
            </div>
        </div>

        <div class="terminal-container" ref="terminalRef">
            <div v-if="filteredLogs.length === 0" class="empty-logs">
                {{ emptyLogsMessage }}
            </div>
            <div v-for="(log, i) in filteredLogs" :key="i" class="log-line" :class="getLogClass(log)">
                <span class="log-index">{{ i + 1 }}</span>
                <span class="log-text">{{ log }}</span>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick, watch, computed } from 'vue'
import { EventsOn, SaveLogToFile, GetSystemHistory, ClearPersistedLogs } from '../api'

const logs = ref([])
const terminalRef = ref(null)
const autoScroll = ref(true)
const showErrorsOnly = ref(false)

// Persist logs limit
const MAX_LOGS = 2000

const filteredLogs = computed(() => {
    if (!showErrorsOnly.value) return logs.value;
    return logs.value.filter(line => {
        const l = line.toLowerCase();
        return l.includes('error') || l.includes('failed') || l.includes('panic') || l.includes('warn');
    });
});

const emptyLogsMessage = computed(() => {
    return showErrorsOnly.value ? 'No errors found in current session.' : 'Waiting for logs... (Backend output will appear here)';
});

let unlistenLog = null

onMounted(async () => {
    // Load history from RxDB
    try {
        const history = await GetSystemHistory()
        if (history && history.length > 0) {
            logs.value = history
        }
    } catch (e) {
        console.warn('LogTab: Failed to load log history', e)
    }

    unlistenLog = EventsOn('system_log', (line) => {
        logs.value.push(line)
        if (logs.value.length > MAX_LOGS) {
            logs.value.shift()
        }

        if (autoScroll.value) {
            scrollToBottom()
        }
    })

    // Initial scroll
    nextTick(scrollToBottom)
})

const scrollToBottom = () => {
    if (terminalRef.value) {
        terminalRef.value.scrollTop = terminalRef.value.scrollHeight
    }
}

const clearLogs = async () => {
    logs.value = []
    await ClearPersistedLogs()
}

const exportLogs = async () => {
    try {
        await SaveLogToFile(logs.value.join('\n'))
    } catch (e) {
        console.error('Failed to export logs:', e)
    }
}

const getLogClass = (line) => {
    const l = line.toLowerCase()
    if (l.includes('error') || l.includes('failed') || l.includes('panic')) return 'log-error'
    if (l.includes('warn')) return 'log-warn'
    if (l.includes('success') || l.includes('completed')) return 'log-success'
    if (l.includes('debug')) return 'log-debug'
    return ''
}

// Check if user has scrolled up to toggle auto-scroll
const handleScroll = () => {
    if (!terminalRef.value) return
    const { scrollTop, scrollHeight, clientHeight } = terminalRef.value
    const atBottom = scrollHeight - scrollTop - clientHeight < 50
    autoScroll.value = atBottom
}

watch(terminalRef, (newVal) => {
    if (newVal) newVal.addEventListener('scroll', handleScroll)
})

onUnmounted(() => {
    if (unlistenLog) unlistenLog()
    if (terminalRef.value) terminalRef.value.removeEventListener('scroll', handleScroll)
})

</script>

<style scoped>
.log-tab-container {
    display: flex;
    flex-direction: column;
    height: calc(100vh - 140px);
}

.log-actions {
    display: flex;
    gap: 8px;
    margin-bottom: 8px;
}

.terminal-container {
    flex: 1;
    background: #0d1117;
    color: #e6edf3;
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    font-size: 13px;
    padding: 16px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    overflow-y: auto;
    line-height: 1.5;
    box-shadow: inset 0 2px 10px rgba(0, 0, 0, 0.3);
}

.log-line {
    display: flex;
    white-space: pre-wrap;
    word-break: break-all;
    border-bottom: 1px solid rgba(255, 255, 255, 0.05);
    padding: 2px 0;
}

.log-index {
    color: #484f58;
    min-width: 40px;
    user-select: none;
    text-align: right;
    padding-right: 12px;
    font-size: 11px;
}

.log-text {
    flex: 1;
}

.empty-logs {
    opacity: 0.5;
    text-align: center;
    padding-top: 40px;
    font-style: italic;
}

/* Coloring */
.log-error .log-text {
    color: #ff7b72;
}

.log-warn .log-text {
    color: #d29922;
}

.log-success .log-text {
    color: #3fb950;
}

.log-debug .log-text {
    color: #8b949e;
    opacity: 0.8;
}

.terminal-container::-webkit-scrollbar {
    width: 10px;
}

.terminal-container::-webkit-scrollbar-track {
    background: #0d1117;
}

.terminal-container::-webkit-scrollbar-thumb {
    background: #30363d;
    border-radius: 5px;
}

.terminal-container::-webkit-scrollbar-thumb:hover {
    background: #484f58;
}

.small {
    padding: 4px 12px;
    font-size: 12px;
}
</style>

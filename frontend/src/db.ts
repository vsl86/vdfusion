import {
    createRxDatabase,
    addRxPlugin,
    RxDatabase,
    RxCollection,
    RxDocument
} from 'rxdb';
import { getRxStorageDexie } from 'rxdb/plugins/storage-dexie';
import { RxDBDevModePlugin } from 'rxdb/plugins/dev-mode';
import { RxDBQueryBuilderPlugin } from 'rxdb/plugins/query-builder';
import { wrappedValidateAjvStorage } from 'rxdb/plugins/validate-ajv';
import { sha256 } from 'js-sha256';

// Polyfill Web Crypto for non-secure contexts (e.g. IP-based access)
if (typeof window !== 'undefined' && window.crypto && !window.crypto.subtle) {
    console.warn('RxDB: crypto.subtle is unavailable, using js-sha256 polyfill');
    (window.crypto as any).subtle = {
        digest: async (algo: string, data: any) => {
            const a = algo.replace('-', '').toLowerCase();
            if (a === 'sha256' || a === 'sha1') {
                // KISS: Use SHA-256 even if SHA-1 is requested by legacy RxDB parts
                return (sha256 as any).arrayBuffer(data);
            }
            throw new Error(`Web Crypto Polyfill: ${algo} not supported (KISS: only SHA-256 implemented)`);
        }
    };
}

// Add plugins
addRxPlugin(RxDBQueryBuilderPlugin);
if (process.env.NODE_ENV === 'development') {
    addRxPlugin(RxDBDevModePlugin);
}

const activityLogSchema = {
    title: 'activity_log',
    version: 0,
    primaryKey: 'id',
    type: 'object',
    properties: {
        id: { type: 'string', maxLength: 100 },
        time: { type: 'string', maxLength: 20 },
        severity: { type: 'string' },
        message: { type: 'string' }
    },
    required: ['id', 'time', 'severity', 'message'],
    indexes: ['time']
};

const systemLogSchema = {
    title: 'system_log',
    version: 0,
    primaryKey: 'id',
    type: 'object',
    properties: {
        id: { type: 'string', maxLength: 100 },
        time: { type: 'string', maxLength: 20 },
        line: { type: 'string' }
    },
    required: ['id', 'time', 'line'],
    indexes: ['time']
};

const settingsSchema = {
    title: 'settings',
    version: 0,
    primaryKey: 'id',
    type: 'object',
    properties: {
        id: { type: 'string', maxLength: 100 },
        instance_id: { type: 'string' }
    },
    required: ['id', 'instance_id']
};

export type ActivityLogDoc = {
    id: string;
    time: string;
    severity: 'info' | 'warn' | 'error' | 'success';
    message: string;
};

export type SystemLogDoc = {
    id: string;
    time: string;
    line: string;
};

export type SettingsDoc = {
    id: string;
    instance_id: string;
};

type VDFDatabaseCollections = {
    activity_logs: RxCollection<ActivityLogDoc>;
    system_logs: RxCollection<SystemLogDoc>;
    settings: RxCollection<SettingsDoc>;
};

export type VDFDatabase = RxDatabase<VDFDatabaseCollections>;

let dbPromise: Promise<VDFDatabase> | null = null;

async function _create() {
    console.log('DatabaseService: creating database..');
    const db = await createRxDatabase<VDFDatabaseCollections>({
        name: 'vdfusion_db',
        storage: wrappedValidateAjvStorage({
            storage: getRxStorageDexie()
        })
    });

    console.log('DatabaseService: creating collections..');
    await db.addCollections({
        activity_logs: {
            schema: activityLogSchema
        },
        system_logs: {
            schema: systemLogSchema
        },
        settings: {
            schema: settingsSchema
        }
    });

    return db;
}

export const getDatabase = (): Promise<VDFDatabase> => {
    if (!dbPromise) {
        dbPromise = _create();
    }
    return dbPromise;
};

export async function syncSession(currentInstanceId: string) {
    console.log(`RxDB: Syncing session with instance_id: ${currentInstanceId}`);
    const db = await getDatabase();
    const sessionDoc = await db.settings.findOne('session').exec();

    if (!sessionDoc) {
        console.log('RxDB: No previous session found. Storing instance_id.');
        await db.settings.insert({ id: 'session', instance_id: currentInstanceId });
        return;
    }

    console.log(`RxDB: Existing session instance_id: ${sessionDoc.instance_id}`);

    if (sessionDoc.instance_id !== currentInstanceId) {
        console.warn(`RxDB: Instance ID mismatch (${sessionDoc.instance_id} vs ${currentInstanceId}). Clearing logs.`);

        await db.activity_logs.find().remove();
        await db.system_logs.find().remove();

        await sessionDoc.incrementalPatch({ instance_id: currentInstanceId });
        console.log('RxDB: Logs cleared and session updated.');
    } else {
        console.log('RxDB: Session match. Logs preserved.');
    }
}

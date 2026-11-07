export namespace config {
	
	export class Settings {
	    include_list: string[];
	    black_list: string[];
	    filter_by_duration: boolean;
	    percent: number;
	    percent_duration_difference: number;
	    duration_difference_min_seconds: number;
	    duration_difference_max_seconds: number;
	    filter_by_file_size: boolean;
	    minimum_file_size: number;
	    maximum_file_size: number;
	    thumbnails: number;
	    concurrency: number;
	    auto_fetch_thumbnails: boolean;
	    recheck_suspicious: boolean;
	    show_media_info: boolean;
	    show_similarity: boolean;
	    show_thumbnails: boolean;
	    cleanup_orphans: boolean;
	    debug_logging: boolean;
	    neural_backend_enabled: boolean;
	    neural_backend_url: string;
	    neural_phash_gate: number;
	    local_neural_engine: boolean;
	    local_neural_lan_server: boolean;
	    local_neural_port: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.include_list = source["include_list"];
	        this.black_list = source["black_list"];
	        this.filter_by_duration = source["filter_by_duration"];
	        this.percent = source["percent"];
	        this.percent_duration_difference = source["percent_duration_difference"];
	        this.duration_difference_min_seconds = source["duration_difference_min_seconds"];
	        this.duration_difference_max_seconds = source["duration_difference_max_seconds"];
	        this.filter_by_file_size = source["filter_by_file_size"];
	        this.minimum_file_size = source["minimum_file_size"];
	        this.maximum_file_size = source["maximum_file_size"];
	        this.thumbnails = source["thumbnails"];
	        this.concurrency = source["concurrency"];
	        this.auto_fetch_thumbnails = source["auto_fetch_thumbnails"];
	        this.recheck_suspicious = source["recheck_suspicious"];
	        this.show_media_info = source["show_media_info"];
	        this.show_similarity = source["show_similarity"];
	        this.show_thumbnails = source["show_thumbnails"];
	        this.cleanup_orphans = source["cleanup_orphans"];
	        this.debug_logging = source["debug_logging"];
	        this.neural_backend_enabled = source["neural_backend_enabled"];
	        this.neural_backend_url = source["neural_backend_url"];
	        this.neural_phash_gate = source["neural_phash_gate"];
	        this.local_neural_engine = source["local_neural_engine"];
	        this.local_neural_lan_server = source["local_neural_lan_server"];
	        this.local_neural_port = source["local_neural_port"];
	    }
	}

}

export namespace db {
	
	export class IgnoredGroup {
	    id: number;
	    label: string;
	    identifier_hashes: string[];
	    resolved_paths: string[];
	
	    static createFrom(source: any = {}) {
	        return new IgnoredGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.identifier_hashes = source["identifier_hashes"];
	        this.resolved_paths = source["resolved_paths"];
	    }
	}

}

export namespace engine {
	
	export class FileInfo {
	    path: string;
	    size: number;
	    modified: number;
	    duration: number;
	    width: number;
	    height: number;
	    codec: string;
	    bitrate: number;
	    fps: number;
	    similarity: number;
	    identifier_hash: string;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	        this.modified = source["modified"];
	        this.duration = source["duration"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.codec = source["codec"];
	        this.bitrate = source["bitrate"];
	        this.fps = source["fps"];
	        this.similarity = source["similarity"];
	        this.identifier_hash = source["identifier_hash"];
	    }
	}
	export class DuplicateGroup {
	    id: string;
	    files: FileInfo[];
	
	    static createFrom(source: any = {}) {
	        return new DuplicateGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.files = this.convertValues(source["files"], FileInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ResultsResponse {
	    items: DuplicateGroup[];
	    total: number;
	    total_files: number;
	    offset: number;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new ResultsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], DuplicateGroup);
	        this.total = source["total"];
	        this.total_files = source["total_files"];
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class DirListResponse {
	    path: string;
	    dirs: string[];
	
	    static createFrom(source: any = {}) {
	        return new DirListResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.dirs = source["dirs"];
	    }
	}
	export class DuplicateStats {
	    total_groups: number;
	    total_files: number;
	
	    static createFrom(source: any = {}) {
	        return new DuplicateStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_groups = source["total_groups"];
	        this.total_files = source["total_files"];
	    }
	}
	export class LocalNeuralStatus {
	    supported: boolean;
	    model_downloaded: boolean;
	    model_path: string;
	    server_running: boolean;
	    server_addr: string;
	    local_ips: string[];
	    download_progress: neural.DownloadProgress;
	
	    static createFrom(source: any = {}) {
	        return new LocalNeuralStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.model_downloaded = source["model_downloaded"];
	        this.model_path = source["model_path"];
	        this.server_running = source["server_running"];
	        this.server_addr = source["server_addr"];
	        this.local_ips = source["local_ips"];
	        this.download_progress = this.convertValues(source["download_progress"], neural.DownloadProgress);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WarningInfo {
	    message: string;
	    fix: string;
	
	    static createFrom(source: any = {}) {
	        return new WarningInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.fix = source["fix"];
	    }
	}
	export class SuspiciousFile {
	    path: string;
	    warnings: WarningInfo[];
	
	    static createFrom(source: any = {}) {
	        return new SuspiciousFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.warnings = this.convertValues(source["warnings"], WarningInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace media {
	
	export class DependencyStatus {
	    ffmpeg: boolean;
	    ffprobe: boolean;
	    ffplay: boolean;
	    missing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DependencyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ffmpeg = source["ffmpeg"];
	        this.ffprobe = source["ffprobe"];
	        this.ffplay = source["ffplay"];
	        this.missing = source["missing"];
	    }
	}

}

export namespace neural {
	
	export class DownloadProgress {
	    active: boolean;
	    downloaded: number;
	    total: number;
	    percent: number;
	    status: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.downloaded = source["downloaded"];
	        this.total = source["total"];
	        this.percent = source["percent"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}

}


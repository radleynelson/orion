export namespace config {
	
	export class AgentConfig {
	    Command: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Command = source["Command"];
	    }
	}
	export class CredentialsConfig {
	    Copy: string[];
	
	    static createFrom(source: any = {}) {
	        return new CredentialsConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Copy = source["Copy"];
	    }
	}
	export class ServerConfig {
	    Command: string;
	    Dir: string;
	    DefaultPort: number;
	    PortEnv: string;
	    Env: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Command = source["Command"];
	        this.Dir = source["Dir"];
	        this.DefaultPort = source["DefaultPort"];
	        this.PortEnv = source["PortEnv"];
	        this.Env = source["Env"];
	    }
	}
	export class OrionConfig {
	    BranchPrefix: string;
	    WorktreesDir: string;
	    Credentials: CredentialsConfig;
	    Servers: Record<string, ServerConfig>;
	    Agents: Record<string, AgentConfig>;
	
	    static createFrom(source: any = {}) {
	        return new OrionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.BranchPrefix = source["BranchPrefix"];
	        this.WorktreesDir = source["WorktreesDir"];
	        this.Credentials = this.convertValues(source["Credentials"], CredentialsConfig);
	        this.Servers = this.convertValues(source["Servers"], ServerConfig, true);
	        this.Agents = this.convertValues(source["Agents"], AgentConfig, true);
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

export namespace diag {
	
	export class GoStats {
	    heapAllocMB: number;
	    heapSysMB: number;
	    stackInUseMB: number;
	    sysMB: number;
	    numGC: number;
	    numGoroutine: number;
	
	    static createFrom(source: any = {}) {
	        return new GoStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.heapAllocMB = source["heapAllocMB"];
	        this.heapSysMB = source["heapSysMB"];
	        this.stackInUseMB = source["stackInUseMB"];
	        this.sysMB = source["sysMB"];
	        this.numGC = source["numGC"];
	        this.numGoroutine = source["numGoroutine"];
	    }
	}
	export class Totals {
	    orionMB: number;
	    webviewMB: number;
	    helpersMB: number;
	    sessionsMB: number;
	    grandMB: number;
	
	    static createFrom(source: any = {}) {
	        return new Totals(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.orionMB = source["orionMB"];
	        this.webviewMB = source["webviewMB"];
	        this.helpersMB = source["helpersMB"];
	        this.sessionsMB = source["sessionsMB"];
	        this.grandMB = source["grandMB"];
	    }
	}
	export class SessionMem {
	    sessionName: string;
	    kind: string;
	    panePID: number;
	    processes: ProcessStats[];
	    totalRSSMB: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionMem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionName = source["sessionName"];
	        this.kind = source["kind"];
	        this.panePID = source["panePID"];
	        this.processes = this.convertValues(source["processes"], ProcessStats);
	        this.totalRSSMB = source["totalRSSMB"];
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
	export class ProcessStats {
	    pid: number;
	    ppid: number;
	    name: string;
	    rssMB: number;
	    cpuPct: number;
	
	    static createFrom(source: any = {}) {
	        return new ProcessStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.ppid = source["ppid"];
	        this.name = source["name"];
	        this.rssMB = source["rssMB"];
	        this.cpuPct = source["cpuPct"];
	    }
	}
	export class MemorySnapshot {
	    go: GoStats;
	    orion: ProcessStats;
	    webview: ProcessStats[];
	    helpers: ProcessStats[];
	    sessions: SessionMem[];
	    totals: Totals;
	    timestamp: number;
	
	    static createFrom(source: any = {}) {
	        return new MemorySnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.go = this.convertValues(source["go"], GoStats);
	        this.orion = this.convertValues(source["orion"], ProcessStats);
	        this.webview = this.convertValues(source["webview"], ProcessStats);
	        this.helpers = this.convertValues(source["helpers"], ProcessStats);
	        this.sessions = this.convertValues(source["sessions"], SessionMem);
	        this.totals = this.convertValues(source["totals"], Totals);
	        this.timestamp = source["timestamp"];
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

export namespace files {
	
	export class FileEntry {
	    name: string;
	    path: string;
	    isDir: boolean;
	    size: number;
	    children?: FileEntry[];
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.children = this.convertValues(source["children"], FileEntry);
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
	export class GrepResult {
	    file: string;
	    line: number;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new GrepResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.line = source["line"];
	        this.content = source["content"];
	    }
	}
	export class SearchResult {
	    name: string;
	    path: string;
	    isDir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	    }
	}

}

export namespace git {
	
	export class ChangedFile {
	    path: string;
	    status: string;
	    statusText: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.statusText = source["statusText"];
	    }
	}
	export class FileDiff {
	    originalContent: string;
	    modifiedContent: string;
	    language: string;
	
	    static createFrom(source: any = {}) {
	        return new FileDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.originalContent = source["originalContent"];
	        this.modifiedContent = source["modifiedContent"];
	        this.language = source["language"];
	    }
	}

}

export namespace main {
	
	export class AgentTypeInfo {
	    name: string;
	    command: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentTypeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.command = source["command"];
	        this.label = source["label"];
	    }
	}

}

export namespace server {
	
	export class ServerStatus {
	    name: string;
	    port: number;
	    running: boolean;
	    tmuxSession: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.port = source["port"];
	        this.running = source["running"];
	        this.tmuxSession = source["tmuxSession"];
	    }
	}

}

export namespace state {
	
	export class SavedTab {
	    label: string;
	    tabType: string;
	    tmuxSession: string;
	    workspacePath: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedTab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.tabType = source["tabType"];
	        this.tmuxSession = source["tmuxSession"];
	        this.workspacePath = source["workspacePath"];
	    }
	}
	export class SessionInfo {
	    tmuxName: string;
	    type: string;
	    label: string;
	    workspacePath: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tmuxName = source["tmuxName"];
	        this.type = source["type"];
	        this.label = source["label"];
	        this.workspacePath = source["workspacePath"];
	    }
	}

}

export namespace web {
	
	export class AgentType {
	    name: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentType(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	    }
	}

}

export namespace workspace {
	
	export class ProjectInfo {
	    name: string;
	    root: string;
	    mainBranch: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.root = source["root"];
	        this.mainBranch = source["mainBranch"];
	    }
	}
	export class Workspace {
	    name: string;
	    path: string;
	    branch: string;
	    isMain: boolean;
	    hasAgent: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Workspace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.branch = source["branch"];
	        this.isMain = source["isMain"];
	        this.hasAgent = source["hasAgent"];
	    }
	}

}


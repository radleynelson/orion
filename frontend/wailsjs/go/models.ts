export namespace chatattachments {
	
	export class Attachment {
	    id?: string;
	    name?: string;
	    path: string;
	    mimeType?: string;
	    size?: number;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.mimeType = source["mimeType"];
	        this.size = source["size"];
	    }
	}

}

export namespace claudesdk {
	
	export class HistoryThread {
	    threadId: string;
	    workspacePath?: string;
	    model?: string;
	    updatedAt: string;
	    messageCount: number;
	    preview?: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryThread(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.threadId = source["threadId"];
	        this.workspacePath = source["workspacePath"];
	        this.model = source["model"];
	        this.updatedAt = source["updatedAt"];
	        this.messageCount = source["messageCount"];
	        this.preview = source["preview"];
	    }
	}
	export class Message {
	    id: string;
	    sessionId: string;
	    threadId?: string;
	    type: string;
	    subtype?: string;
	    role?: string;
	    text?: string;
	    status?: string;
	    toolUseId?: string;
	    toolName?: string;
	    details?: string;
	    planPath?: string;
	    attachments?: chatattachments.Attachment[];
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sessionId = source["sessionId"];
	        this.threadId = source["threadId"];
	        this.type = source["type"];
	        this.subtype = source["subtype"];
	        this.role = source["role"];
	        this.text = source["text"];
	        this.status = source["status"];
	        this.toolUseId = source["toolUseId"];
	        this.toolName = source["toolName"];
	        this.details = source["details"];
	        this.planPath = source["planPath"];
	        this.attachments = this.convertValues(source["attachments"], chatattachments.Attachment);
	        this.createdAt = source["createdAt"];
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
	export class SessionInfo {
	    id: string;
	    type: string;
	    label: string;
	    workspacePath: string;
	    status: string;
	    threadId?: string;
	    provider?: string;
	    icon?: string;
	    viewMode?: string;
	    runtimeSessionId?: string;
	    model?: string;
	    reasoningEffort?: string;
	    approvalPolicy?: string;
	    sandboxMode?: string;
	    permissionMode?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.label = source["label"];
	        this.workspacePath = source["workspacePath"];
	        this.status = source["status"];
	        this.threadId = source["threadId"];
	        this.provider = source["provider"];
	        this.icon = source["icon"];
	        this.viewMode = source["viewMode"];
	        this.runtimeSessionId = source["runtimeSessionId"];
	        this.model = source["model"];
	        this.reasoningEffort = source["reasoningEffort"];
	        this.approvalPolicy = source["approvalPolicy"];
	        this.sandboxMode = source["sandboxMode"];
	        this.permissionMode = source["permissionMode"];
	    }
	}

}

export namespace codexchat {
	
	export class HistoryThread {
	    threadId: string;
	    workspacePath?: string;
	    model?: string;
	    updatedAt: string;
	    messageCount: number;
	    preview?: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryThread(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.threadId = source["threadId"];
	        this.workspacePath = source["workspacePath"];
	        this.model = source["model"];
	        this.updatedAt = source["updatedAt"];
	        this.messageCount = source["messageCount"];
	        this.preview = source["preview"];
	    }
	}
	export class Message {
	    id: string;
	    sessionId: string;
	    threadId?: string;
	    type: string;
	    subtype?: string;
	    role?: string;
	    text?: string;
	    status?: string;
	    toolUseId?: string;
	    toolName?: string;
	    details?: string;
	    attachments?: chatattachments.Attachment[];
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sessionId = source["sessionId"];
	        this.threadId = source["threadId"];
	        this.type = source["type"];
	        this.subtype = source["subtype"];
	        this.role = source["role"];
	        this.text = source["text"];
	        this.status = source["status"];
	        this.toolUseId = source["toolUseId"];
	        this.toolName = source["toolName"];
	        this.details = source["details"];
	        this.attachments = this.convertValues(source["attachments"], chatattachments.Attachment);
	        this.createdAt = source["createdAt"];
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
	export class SessionInfo {
	    id: string;
	    type: string;
	    label: string;
	    workspacePath: string;
	    status: string;
	    threadId?: string;
	    provider?: string;
	    icon?: string;
	    viewMode?: string;
	    runtimeSessionId?: string;
	    model?: string;
	    reasoningEffort?: string;
	    approvalPolicy?: string;
	    sandboxMode?: string;
	    collaborationMode?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.label = source["label"];
	        this.workspacePath = source["workspacePath"];
	        this.status = source["status"];
	        this.threadId = source["threadId"];
	        this.provider = source["provider"];
	        this.icon = source["icon"];
	        this.viewMode = source["viewMode"];
	        this.runtimeSessionId = source["runtimeSessionId"];
	        this.model = source["model"];
	        this.reasoningEffort = source["reasoningEffort"];
	        this.approvalPolicy = source["approvalPolicy"];
	        this.sandboxMode = source["sandboxMode"];
	        this.collaborationMode = source["collaborationMode"];
	    }
	}

}

export namespace config {
	
	export class AgentConfig {
	    Label: string;
	    Provider: string;
	    Icon: string;
	    Command: string;
	    Model: string;
	    ReasoningEffort: string;
	    ApprovalPolicy: string;
	    SandboxMode: string;
	    PermissionMode: string;
	    CollaborationMode: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Label = source["Label"];
	        this.Provider = source["Provider"];
	        this.Icon = source["Icon"];
	        this.Command = source["Command"];
	        this.Model = source["Model"];
	        this.ReasoningEffort = source["ReasoningEffort"];
	        this.ApprovalPolicy = source["ApprovalPolicy"];
	        this.SandboxMode = source["SandboxMode"];
	        this.PermissionMode = source["PermissionMode"];
	        this.CollaborationMode = source["CollaborationMode"];
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
	
	export class FDDirCount {
	    dir: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new FDDirCount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.count = source["count"];
	    }
	}
	export class FDEntry {
	    fd: string;
	    type: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new FDEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fd = source["fd"];
	        this.type = source["type"];
	        this.name = source["name"];
	    }
	}
	export class FDTypeCount {
	    type: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new FDTypeCount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.count = source["count"];
	    }
	}
	export class FDStats {
	    count: number;
	    softLimit: number;
	    hardLimit: number;
	    usagePct: number;
	    byType: FDTypeCount[];
	    topEntries: FDEntry[];
	    groupedDirs: FDDirCount[];
	    truncated: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new FDStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.softLimit = source["softLimit"];
	        this.hardLimit = source["hardLimit"];
	        this.usagePct = source["usagePct"];
	        this.byType = this.convertValues(source["byType"], FDTypeCount);
	        this.topEntries = this.convertValues(source["topEntries"], FDEntry);
	        this.groupedDirs = this.convertValues(source["groupedDirs"], FDDirCount);
	        this.truncated = source["truncated"];
	        this.error = source["error"];
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
	    fds?: FDStats;
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
	        this.fds = this.convertValues(source["fds"], FDStats);
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
	    provider?: string;
	    icon?: string;
	    model?: string;
	    reasoningEffort?: string;
	    approvalPolicy?: string;
	    sandboxMode?: string;
	    permissionMode?: string;
	    collaborationMode?: string;
	    chatCapable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentTypeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.command = source["command"];
	        this.label = source["label"];
	        this.provider = source["provider"];
	        this.icon = source["icon"];
	        this.model = source["model"];
	        this.reasoningEffort = source["reasoningEffort"];
	        this.approvalPolicy = source["approvalPolicy"];
	        this.sandboxMode = source["sandboxMode"];
	        this.permissionMode = source["permissionMode"];
	        this.collaborationMode = source["collaborationMode"];
	        this.chatCapable = source["chatCapable"];
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
	    provider?: string;
	    icon?: string;
	    viewMode?: string;
	    runtimeSessionId?: string;
	    threadId?: string;
	    model?: string;
	    reasoningEffort?: string;
	    approvalPolicy?: string;
	    sandboxMode?: string;
	    permissionMode?: string;
	    collaborationMode?: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedTab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.tabType = source["tabType"];
	        this.tmuxSession = source["tmuxSession"];
	        this.workspacePath = source["workspacePath"];
	        this.provider = source["provider"];
	        this.icon = source["icon"];
	        this.viewMode = source["viewMode"];
	        this.runtimeSessionId = source["runtimeSessionId"];
	        this.threadId = source["threadId"];
	        this.model = source["model"];
	        this.reasoningEffort = source["reasoningEffort"];
	        this.approvalPolicy = source["approvalPolicy"];
	        this.sandboxMode = source["sandboxMode"];
	        this.permissionMode = source["permissionMode"];
	        this.collaborationMode = source["collaborationMode"];
	    }
	}
	export class SessionInfo {
	    tmuxName: string;
	    type: string;
	    label: string;
	    workspacePath: string;
	    provider?: string;
	    icon?: string;
	    viewMode?: string;
	    runtimeSessionId?: string;
	    threadId?: string;
	    model?: string;
	    reasoningEffort?: string;
	    approvalPolicy?: string;
	    sandboxMode?: string;
	    permissionMode?: string;
	    collaborationMode?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tmuxName = source["tmuxName"];
	        this.type = source["type"];
	        this.label = source["label"];
	        this.workspacePath = source["workspacePath"];
	        this.provider = source["provider"];
	        this.icon = source["icon"];
	        this.viewMode = source["viewMode"];
	        this.runtimeSessionId = source["runtimeSessionId"];
	        this.threadId = source["threadId"];
	        this.model = source["model"];
	        this.reasoningEffort = source["reasoningEffort"];
	        this.approvalPolicy = source["approvalPolicy"];
	        this.sandboxMode = source["sandboxMode"];
	        this.permissionMode = source["permissionMode"];
	        this.collaborationMode = source["collaborationMode"];
	    }
	}

}

export namespace web {
	
	export class AgentType {
	    name: string;
	    label: string;
	    provider?: string;
	    icon?: string;
	    model?: string;
	    reasoningEffort?: string;
	    approvalPolicy?: string;
	    sandboxMode?: string;
	    permissionMode?: string;
	    collaborationMode?: string;
	    chatCapable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentType(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.provider = source["provider"];
	        this.icon = source["icon"];
	        this.model = source["model"];
	        this.reasoningEffort = source["reasoningEffort"];
	        this.approvalPolicy = source["approvalPolicy"];
	        this.sandboxMode = source["sandboxMode"];
	        this.permissionMode = source["permissionMode"];
	        this.collaborationMode = source["collaborationMode"];
	        this.chatCapable = source["chatCapable"];
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


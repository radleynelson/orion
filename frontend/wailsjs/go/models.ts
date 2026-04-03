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
	    Credentials: CredentialsConfig;
	    Servers: Record<string, ServerConfig>;
	    Agents: Record<string, AgentConfig>;
	
	    static createFrom(source: any = {}) {
	        return new OrionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
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


export namespace api {
	
	export class DispatchResult {
	    session_hash: string;
	    output: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DispatchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_hash = source["session_hash"];
	        this.output = source["output"];
	        this.error = source["error"];
	    }
	}
	export class Listener {
	    hash: string;
	    host: string;
	    port: number;
	    encrypted: boolean;
	    group_dispatch: boolean;
	    disable_history: boolean;
	    public_ip: string;
	    shell_path: string;
	    // Go type: time
	    timestamp: any;
	    interfaces: string[];
	
	    static createFrom(source: any = {}) {
	        return new Listener(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.encrypted = source["encrypted"];
	        this.group_dispatch = source["group_dispatch"];
	        this.disable_history = source["disable_history"];
	        this.public_ip = source["public_ip"];
	        this.shell_path = source["shell_path"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.interfaces = source["interfaces"];
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
	export class Session {
	    hash: string;
	    host: string;
	    port: number;
	    alias: string;
	    user: string;
	    os: string;
	    version?: string;
	    network_interfaces: Record<string, string>;
	    python2: string;
	    python3: string;
	    // Go type: time
	    timestamp: any;
	    group_dispatch: boolean;
	    encrypted: boolean;
	    tag: string;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.alias = source["alias"];
	        this.user = source["user"];
	        this.os = source["os"];
	        this.version = source["version"];
	        this.network_interfaces = source["network_interfaces"];
	        this.python2 = source["python2"];
	        this.python3 = source["python3"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.group_dispatch = source["group_dispatch"];
	        this.encrypted = source["encrypted"];
	        this.tag = source["tag"];
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
	export class TunnelInfo {
	    type: string;
	    address: string;
	
	    static createFrom(source: any = {}) {
	        return new TunnelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.address = source["address"];
	    }
	}

}

export namespace app {
	
	export class ConnectionStatus {
	    connected: boolean;
	    profileName: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.profileName = source["profileName"];
	        this.url = source["url"];
	    }
	}

}

export namespace profile {
	
	export class Profile {
	    name: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	    }
	}

}


export namespace config {
	
	export class Config {
	    server_address: string;
	    client_id: string;
	    client_secret: string;
	    remember_me: boolean;
	    token_expires_at: number;
	    port_preferences: Record<number, number>;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_address = source["server_address"];
	        this.client_id = source["client_id"];
	        this.client_secret = source["client_secret"];
	        this.remember_me = source["remember_me"];
	        this.token_expires_at = source["token_expires_at"];
	        this.port_preferences = source["port_preferences"];
	    }
	}

}

export namespace main {
	
	export class SavedCredentials {
	    server_address: string;
	    client_id: string;
	    client_secret: string;
	    remember_me: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SavedCredentials(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_address = source["server_address"];
	        this.client_id = source["client_id"];
	        this.client_secret = source["client_secret"];
	        this.remember_me = source["remember_me"];
	    }
	}
	export class VersionInfo {
	    version: string;
	    gitCommit: string;
	    buildDate: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.gitCommit = source["gitCommit"];
	        this.buildDate = source["buildDate"];
	    }
	}

}

export namespace models {
	
	export class ServiceInfo {
	    instance_id: number;
	    instance_name: string;
	    agent_name: string;
	    service_type: string;
	    service_port: number;
	    preferred_port: number;
	    description: string;
	    secret_key: string;
	    access_type: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instance_id = source["instance_id"];
	        this.instance_name = source["instance_name"];
	        this.agent_name = source["agent_name"];
	        this.service_type = source["service_type"];
	        this.service_port = source["service_port"];
	        this.preferred_port = source["preferred_port"];
	        this.description = source["description"];
	        this.secret_key = source["secret_key"];
	        this.access_type = source["access_type"];
	        this.status = source["status"];
	    }
	}

}


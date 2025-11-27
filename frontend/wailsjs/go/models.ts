export namespace config {
	
	export class Config {
	    server_address: string;
	    client_id: string;
	    client_secret: string;
	    device_token: string;
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
	        this.device_token = source["device_token"];
	        this.remember_me = source["remember_me"];
	        this.token_expires_at = source["token_expires_at"];
	        this.port_preferences = source["port_preferences"];
	    }
	}

}

export namespace main {
	
	export class DeviceInfo {
	    device_token: string;
	    device_name: string;
	    os: string;
	    arch: string;
	    hostname: string;
	    status: string;
	    last_used_at: string;
	    created_at: string;
	    is_current: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DeviceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.device_token = source["device_token"];
	        this.device_name = source["device_name"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.hostname = source["hostname"];
	        this.status = source["status"];
	        this.last_used_at = source["last_used_at"];
	        this.created_at = source["created_at"];
	        this.is_current = source["is_current"];
	    }
	}
	export class SavedCredentials {
	    server_address: string;
	    client_id: string;
	    client_secret: string;
	    remember_me: boolean;
	    has_token: boolean;
	    is_online: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SavedCredentials(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_address = source["server_address"];
	        this.client_id = source["client_id"];
	        this.client_secret = source["client_secret"];
	        this.remember_me = source["remember_me"];
	        this.has_token = source["has_token"];
	        this.is_online = source["is_online"];
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


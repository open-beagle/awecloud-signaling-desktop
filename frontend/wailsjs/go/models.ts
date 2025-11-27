export namespace config {
	
	export class Config {
	    server_address: string;
	    client_id: string;
	    client_secret: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_address = source["server_address"];
	        this.client_id = source["client_id"];
	        this.client_secret = source["client_secret"];
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
	    description: string;
	    secret_key: string;
	
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
	        this.description = source["description"];
	        this.secret_key = source["secret_key"];
	    }
	}

}


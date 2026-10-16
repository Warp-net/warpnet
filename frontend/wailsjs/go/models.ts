export namespace main {
	
	export class AppMessage {
	    body: number[];
	    message_id: string;
	    node_id: string;
	    path: string;
	    timestamp?: string;
	    version: string;
	    signature: string;
	
	    static createFrom(source: any = {}) {
	        return new AppMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.body = source["body"];
	        this.message_id = source["message_id"];
	        this.node_id = source["node_id"];
	        this.path = source["path"];
	        this.timestamp = source["timestamp"];
	        this.version = source["version"];
	        this.signature = source["signature"];
	    }
	}
	export class ClipboardFile {
	    name: string;
	    type: string;
	    data: number[];
	
	    static createFrom(source: any = {}) {
	        return new ClipboardFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.data = source["data"];
	    }
	}

}


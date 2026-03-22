export namespace main {
	
	export class Status {
	    font: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.font = source["font"];
	        this.active = source["active"];
	    }
	}

}


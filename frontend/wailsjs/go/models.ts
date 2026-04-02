export namespace credential {
	
	export class CredentialInfo {
	    id: string;
	    siteURL: string;
	    email: string;
	    // Go type: time
	    lastUsedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new CredentialInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.siteURL = source["siteURL"];
	        this.email = source["email"];
	        this.lastUsedAt = this.convertValues(source["lastUsedAt"], null);
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

export namespace gui {
	
	export class LoginStatus {
	    loggedIn: boolean;
	    email: string;
	    siteURL: string;
	
	    static createFrom(source: any = {}) {
	        return new LoginStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loggedIn = source["loggedIn"];
	        this.email = source["email"];
	        this.siteURL = source["siteURL"];
	    }
	}
	export class MountStatus {
	    mounted: boolean;
	    mountpoint: string;
	    project: string[];
	    zenMode: boolean;
	    webdavAddr: string;
	
	    static createFrom(source: any = {}) {
	        return new MountStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mounted = source["mounted"];
	        this.mountpoint = source["mountpoint"];
	        this.project = source["project"];
	        this.zenMode = source["zenMode"];
	        this.webdavAddr = source["webdavAddr"];
	    }
	}

}

export namespace model {
	
	export class Project {
	    _id: string;
	    name: string;
	    lastUpdated: string;
	    accessLevel: string;
	    source: string;
	    archived: boolean;
	    trashed: boolean;
	    rootDoc_id: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this._id = source["_id"];
	        this.name = source["name"];
	        this.lastUpdated = source["lastUpdated"];
	        this.accessLevel = source["accessLevel"];
	        this.source = source["source"];
	        this.archived = source["archived"];
	        this.trashed = source["trashed"];
	        this.rootDoc_id = source["rootDoc_id"];
	    }
	}

}


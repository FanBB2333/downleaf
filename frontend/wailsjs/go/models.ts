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
	    project: string;
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


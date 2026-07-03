export namespace main {
	
	export class ProfileSummary {
	    Profile: storage.Profile;
	    Services: storage.Service[];
	
	    static createFrom(source: any = {}) {
	        return new ProfileSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Profile = this.convertValues(source["Profile"], storage.Profile);
	        this.Services = this.convertValues(source["Services"], storage.Service);
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

export namespace storage {
	
	export class Profile {
	    ID: number;
	    Name: string;
	    CreatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.CreatedAt = source["CreatedAt"];
	    }
	}
	export class Service {
	    ID: number;
	    ProfileID: number;
	    Engine: string;
	    ImageTag: string;
	    HostPort: number;
	    Username?: string;
	    PasswordEncrypted?: string;
	    DBName?: string;
	    VolumeName: string;
	
	    static createFrom(source: any = {}) {
	        return new Service(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.ProfileID = source["ProfileID"];
	        this.Engine = source["Engine"];
	        this.ImageTag = source["ImageTag"];
	        this.HostPort = source["HostPort"];
	        this.Username = source["Username"];
	        this.PasswordEncrypted = source["PasswordEncrypted"];
	        this.DBName = source["DBName"];
	        this.VolumeName = source["VolumeName"];
	    }
	}

}


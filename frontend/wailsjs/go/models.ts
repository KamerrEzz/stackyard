export namespace dbengine {
	
	export class QueryResult {
	    Columns: string[];
	    Rows: any[][];
	    RowsAffected: number;
	    Duration: number;
	
	    static createFrom(source: any = {}) {
	        return new QueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Columns = source["Columns"];
	        this.Rows = source["Rows"];
	        this.RowsAffected = source["RowsAffected"];
	        this.Duration = source["Duration"];
	    }
	}

}

export namespace main {
	
	export class ConnectionFormFields {
	    Engine: string;
	    Host: string;
	    Port: number;
	    Username: string;
	    Password: string;
	    Database: string;
	    Params: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionFormFields(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Engine = source["Engine"];
	        this.Host = source["Host"];
	        this.Port = source["Port"];
	        this.Username = source["Username"];
	        this.Password = source["Password"];
	        this.Database = source["Database"];
	        this.Params = source["Params"];
	    }
	}
	export class PortConflictInfo {
	    HasConflict: boolean;
	    Port: number;
	    SuggestedPort: number;
	
	    static createFrom(source: any = {}) {
	        return new PortConflictInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.HasConflict = source["HasConflict"];
	        this.Port = source["Port"];
	        this.SuggestedPort = source["SuggestedPort"];
	    }
	}
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
	export class ServiceRequest {
	    Engine: string;
	    HostPort: number;
	
	    static createFrom(source: any = {}) {
	        return new ServiceRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Engine = source["Engine"];
	        this.HostPort = source["HostPort"];
	    }
	}

}

export namespace storage {
	
	export class Connection {
	    ID: number;
	    Name: string;
	    Engine: string;
	    Host: string;
	    Port: number;
	    Username?: string;
	    PasswordEncrypted?: string;
	    Database?: string;
	    ParamsJSON: string;
	    LastUsedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Engine = source["Engine"];
	        this.Host = source["Host"];
	        this.Port = source["Port"];
	        this.Username = source["Username"];
	        this.PasswordEncrypted = source["PasswordEncrypted"];
	        this.Database = source["Database"];
	        this.ParamsJSON = source["ParamsJSON"];
	        this.LastUsedAt = source["LastUsedAt"];
	    }
	}
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


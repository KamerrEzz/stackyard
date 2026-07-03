export namespace dbengine {
	
	export class ColumnInfo {
	    Name: string;
	    DataType: string;
	    Nullable: boolean;
	    IsPrimaryKey: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ColumnInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.DataType = source["DataType"];
	        this.Nullable = source["Nullable"];
	        this.IsPrimaryKey = source["IsPrimaryKey"];
	    }
	}
	export class ForeignKey {
	    TableName: string;
	    ColumnName: string;
	    ReferencedTable: string;
	    ReferencedColumn: string;
	
	    static createFrom(source: any = {}) {
	        return new ForeignKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TableName = source["TableName"];
	        this.ColumnName = source["ColumnName"];
	        this.ReferencedTable = source["ReferencedTable"];
	        this.ReferencedColumn = source["ReferencedColumn"];
	    }
	}
	export class ResultColumn {
	    Name: string;
	    DatabaseType: string;
	    Nullable?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ResultColumn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.DatabaseType = source["DatabaseType"];
	        this.Nullable = source["Nullable"];
	    }
	}
	export class QueryResult {
	    Columns: ResultColumn[];
	    Rows: any[][];
	    RowsAffected: number;
	    LastInsertID: number;
	    Duration: number;
	
	    static createFrom(source: any = {}) {
	        return new QueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Columns = this.convertValues(source["Columns"], ResultColumn);
	        this.Rows = source["Rows"];
	        this.RowsAffected = source["RowsAffected"];
	        this.LastInsertID = source["LastInsertID"];
	        this.Duration = source["Duration"];
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
	
	export class StatementResult {
	    Statement: string;
	    Result?: QueryResult;
	    Success: boolean;
	    ErrorMessage: string;
	
	    static createFrom(source: any = {}) {
	        return new StatementResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Statement = source["Statement"];
	        this.Result = this.convertValues(source["Result"], QueryResult);
	        this.Success = source["Success"];
	        this.ErrorMessage = source["ErrorMessage"];
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
	export class TableInfo {
	    Name: string;
	    Columns: ColumnInfo[];
	
	    static createFrom(source: any = {}) {
	        return new TableInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Columns = this.convertValues(source["Columns"], ColumnInfo);
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

export namespace importdata {
	
	export class Mismatch {
	    RowIndex: number;
	    Column: string;
	    Reason: string;
	
	    static createFrom(source: any = {}) {
	        return new Mismatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.RowIndex = source["RowIndex"];
	        this.Column = source["Column"];
	        this.Reason = source["Reason"];
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
	    SavedConnectionID: number;
	
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
	        this.SavedConnectionID = source["SavedConnectionID"];
	    }
	}
	export class ImportCommitResult {
	    Mismatches: importdata.Mismatch[];
	    RowsInserted: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportCommitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Mismatches = this.convertValues(source["Mismatches"], importdata.Mismatch);
	        this.RowsInserted = source["RowsInserted"];
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
	export class ImportValidationResult {
	    Mismatches: importdata.Mismatch[];
	    RowCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportValidationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Mismatches = this.convertValues(source["Mismatches"], importdata.Mismatch);
	        this.RowCount = source["RowCount"];
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
	export class RedisSetPage {
	    Members: string[];
	    NextCursor: number;
	
	    static createFrom(source: any = {}) {
	        return new RedisSetPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Members = source["Members"];
	        this.NextCursor = source["NextCursor"];
	    }
	}
	export class ScanKeysResult {
	    Keys: string[];
	    NextCursor: number;
	
	    static createFrom(source: any = {}) {
	        return new ScanKeysResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Keys = source["Keys"];
	        this.NextCursor = source["NextCursor"];
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
	export class SnippetFilter {
	    SearchText: string;
	    ConnectionID: number;
	    Engine: string;
	
	    static createFrom(source: any = {}) {
	        return new SnippetFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SearchText = source["SearchText"];
	        this.ConnectionID = source["ConnectionID"];
	        this.Engine = source["Engine"];
	    }
	}

}

export namespace redis {
	
	export class SortedSetMember {
	    Member: string;
	    Score: number;
	
	    static createFrom(source: any = {}) {
	        return new SortedSetMember(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Member = source["Member"];
	        this.Score = source["Score"];
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
	export class QueryHistoryEntry {
	    ID: number;
	    ConnectionID: number;
	    QueryText: string;
	    ExecutedAt: string;
	    DurationMs: number;
	    Success: boolean;
	    RowsAffected: number;
	    ErrorMessage?: string;
	
	    static createFrom(source: any = {}) {
	        return new QueryHistoryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.ConnectionID = source["ConnectionID"];
	        this.QueryText = source["QueryText"];
	        this.ExecutedAt = source["ExecutedAt"];
	        this.DurationMs = source["DurationMs"];
	        this.Success = source["Success"];
	        this.RowsAffected = source["RowsAffected"];
	        this.ErrorMessage = source["ErrorMessage"];
	    }
	}
	export class QueryHistoryFilter {
	    ConnectionID: number;
	    SearchText: string;
	
	    static createFrom(source: any = {}) {
	        return new QueryHistoryFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ConnectionID = source["ConnectionID"];
	        this.SearchText = source["SearchText"];
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
	export class Snippet {
	    ID: number;
	    ConnectionID?: number;
	    Engine: string;
	    Name: string;
	    Body: string;
	    TagsJSON: string;
	    CreatedAt: string;
	    UpdatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Snippet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.ConnectionID = source["ConnectionID"];
	        this.Engine = source["Engine"];
	        this.Name = source["Name"];
	        this.Body = source["Body"];
	        this.TagsJSON = source["TagsJSON"];
	        this.CreatedAt = source["CreatedAt"];
	        this.UpdatedAt = source["UpdatedAt"];
	    }
	}

}


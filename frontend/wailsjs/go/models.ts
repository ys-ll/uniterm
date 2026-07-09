export namespace database {
	
	export class ColumnDef {
	    name: string;
	    type: string;
	    nullable: boolean;
	    defaultVal: string;
	    defaultType: string;
	    comment: string;
	    collation: string;
	    onUpdate: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ColumnDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.nullable = source["nullable"];
	        this.defaultVal = source["defaultVal"];
	        this.defaultType = source["defaultType"];
	        this.comment = source["comment"];
	        this.collation = source["collation"];
	        this.onUpdate = source["onUpdate"];
	    }
	}
	export class ColumnInfo {
	    name: string;
	    type: string;
	    nullable: boolean;
	    defaultVal: string;
	    defaultType: string;
	    isPrimary: boolean;
	    comment: string;
	    collation: string;
	    onUpdate: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ColumnInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.nullable = source["nullable"];
	        this.defaultVal = source["defaultVal"];
	        this.defaultType = source["defaultType"];
	        this.isPrimary = source["isPrimary"];
	        this.comment = source["comment"];
	        this.collation = source["collation"];
	        this.onUpdate = source["onUpdate"];
	    }
	}
	export class ExecResult {
	    affected: number;
	    lastInsertId: number;
	
	    static createFrom(source: any = {}) {
	        return new ExecResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.affected = source["affected"];
	        this.lastInsertId = source["lastInsertId"];
	    }
	}
	export class IndexDef {
	    name: string;
	    columns: string[];
	    unique: boolean;
	    isPrimary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IndexDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.unique = source["unique"];
	        this.isPrimary = source["isPrimary"];
	    }
	}
	export class IndexInfo {
	    name: string;
	    columns: string[];
	    unique: boolean;
	    isPrimary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IndexInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.unique = source["unique"];
	        this.isPrimary = source["isPrimary"];
	    }
	}
	export class QueryResultColumn {
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new QueryResultColumn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}
	export class QueryResult {
	    columns: QueryResultColumn[];
	    rows: any[];
	
	    static createFrom(source: any = {}) {
	        return new QueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = this.convertValues(source["columns"], QueryResultColumn);
	        this.rows = source["rows"];
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
	
	export class SchemaResult {
	    columns: ColumnInfo[];
	    indexes: IndexInfo[];
	
	    static createFrom(source: any = {}) {
	        return new SchemaResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = this.convertValues(source["columns"], ColumnInfo);
	        this.indexes = this.convertValues(source["indexes"], IndexInfo);
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
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new TableInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}

}

export namespace main {
	
	export class AppInfo {
	    name: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	    }
	}
	export class ModelInfo {
	    id: string;
	    display_name: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.display_name = source["display_name"];
	    }
	}

}

export namespace session {
	
	export class PostLoginExpectStep {
	    expect: string;
	    send: string;
	    enter: boolean;
	    timeoutSecond?: number;
	
	    static createFrom(source: any = {}) {
	        return new PostLoginExpectStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.expect = source["expect"];
	        this.send = source["send"];
	        this.enter = source["enter"];
	        this.timeoutSecond = source["timeoutSecond"];
	    }
	}
	export class ConnectionConfig {
	    id: string;
	    name: string;
	    type: string;
	    host: string;
	    port: number;
	    user: string;
	    authType: string;
	    password?: string;
	    keyPath?: string;
	    groupId?: string;
	    rdpFixedWidth?: number;
	    rdpFixedHeight?: number;
	    rdpSmartSizing: boolean;
	    shellPath?: string;
	    serialPort?: string;
	    serialBaudRate?: number;
	    serialDataBits?: number;
	    serialStopBits?: number;
	    serialParity?: string;
	    dbType?: string;
	    dbName?: string;
	    dbParams?: string;
	    postLoginScript?: string;
	    postLoginExpectSteps?: PostLoginExpectStep[];
	    tunnelSSHConnId?: string;
	    tunnelSSHUser?: string;
	    tunnelSSHPassword?: string;
	    sftpMaxConcurrency?: number;
	    ftpEncryption?: string;
	    ftpPassive: boolean;
	    ftpEncoding?: string;
	    smbDomain?: string;
	    smbShare?: string;
	    webdavUrl?: string;
	    webdavUseSSL: boolean;
	    s3Region?: string;
	    s3Bucket?: string;
	    encoding?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.authType = source["authType"];
	        this.password = source["password"];
	        this.keyPath = source["keyPath"];
	        this.groupId = source["groupId"];
	        this.rdpFixedWidth = source["rdpFixedWidth"];
	        this.rdpFixedHeight = source["rdpFixedHeight"];
	        this.rdpSmartSizing = source["rdpSmartSizing"];
	        this.shellPath = source["shellPath"];
	        this.serialPort = source["serialPort"];
	        this.serialBaudRate = source["serialBaudRate"];
	        this.serialDataBits = source["serialDataBits"];
	        this.serialStopBits = source["serialStopBits"];
	        this.serialParity = source["serialParity"];
	        this.dbType = source["dbType"];
	        this.dbName = source["dbName"];
	        this.dbParams = source["dbParams"];
	        this.postLoginScript = source["postLoginScript"];
	        this.postLoginExpectSteps = this.convertValues(source["postLoginExpectSteps"], PostLoginExpectStep);
	        this.tunnelSSHConnId = source["tunnelSSHConnId"];
	        this.tunnelSSHUser = source["tunnelSSHUser"];
	        this.tunnelSSHPassword = source["tunnelSSHPassword"];
	        this.sftpMaxConcurrency = source["sftpMaxConcurrency"];
	        this.ftpEncryption = source["ftpEncryption"];
	        this.ftpPassive = source["ftpPassive"];
	        this.ftpEncoding = source["ftpEncoding"];
	        this.smbDomain = source["smbDomain"];
	        this.smbShare = source["smbShare"];
	        this.webdavUrl = source["webdavUrl"];
	        this.webdavUseSSL = source["webdavUseSSL"];
	        this.s3Region = source["s3Region"];
	        this.s3Bucket = source["s3Bucket"];
	        this.encoding = source["encoding"];
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
	export class ConnectionGroup {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class ConnectionStoreData {
	    groups: ConnectionGroup[];
	    connections: ConnectionConfig[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectionStoreData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], ConnectionGroup);
	        this.connections = this.convertValues(source["connections"], ConnectionConfig);
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
	export class DiskInfo {
	    name: string;
	    type: string;
	    size: string;
	    mountPoint: string;
	    used: string;
	    total: string;
	    usage: number;
	    media: string;
	    fsType: string;
	    uuid: string;
	    vendor: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new DiskInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.size = source["size"];
	        this.mountPoint = source["mountPoint"];
	        this.used = source["used"];
	        this.total = source["total"];
	        this.usage = source["usage"];
	        this.media = source["media"];
	        this.fsType = source["fsType"];
	        this.uuid = source["uuid"];
	        this.vendor = source["vendor"];
	        this.model = source["model"];
	    }
	}
	export class FieldEntry {
	    field: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new FieldEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.field = source["field"];
	        this.value = source["value"];
	    }
	}
	export class FileItem {
	    name: string;
	    size: number;
	    modTime: string;
	    mode: string;
	    isDir: boolean;
	    isHidden: boolean;
	    owner: string;
	    group: string;
	
	    static createFrom(source: any = {}) {
	        return new FileItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.mode = source["mode"];
	        this.isDir = source["isDir"];
	        this.isHidden = source["isHidden"];
	        this.owner = source["owner"];
	        this.group = source["group"];
	    }
	}
	export class FileListResult {
	    files: FileItem[];
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new FileListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = this.convertValues(source["files"], FileItem);
	        this.dir = source["dir"];
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
	export class NetCardInfo {
	    name: string;
	    state: string;
	    mac: string;
	    speed: string;
	    type: string;
	    bondMaster: string;
	    bondSlaves: string[];
	    ipAddrs: string[];
	
	    static createFrom(source: any = {}) {
	        return new NetCardInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.state = source["state"];
	        this.mac = source["mac"];
	        this.speed = source["speed"];
	        this.type = source["type"];
	        this.bondMaster = source["bondMaster"];
	        this.bondSlaves = source["bondSlaves"];
	        this.ipAddrs = source["ipAddrs"];
	    }
	}
	export class PortInfo {
	    protocol: string;
	    localAddr: string;
	    state: string;
	    process: string;
	
	    static createFrom(source: any = {}) {
	        return new PortInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocol = source["protocol"];
	        this.localAddr = source["localAddr"];
	        this.state = source["state"];
	        this.process = source["process"];
	    }
	}
	
	export class RedisKeyInfo {
	    name: string;
	    type: string;
	    ttl: number;
	
	    static createFrom(source: any = {}) {
	        return new RedisKeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.ttl = source["ttl"];
	    }
	}
	export class ScanResult {
	    keys: RedisKeyInfo[];
	    cursor: number;
	    scanCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.keys = this.convertValues(source["keys"], RedisKeyInfo);
	        this.cursor = source["cursor"];
	        this.scanCount = source["scanCount"];
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
	export class ScoredMember {
	    score: number;
	    member: string;
	
	    static createFrom(source: any = {}) {
	        return new ScoredMember(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.score = source["score"];
	        this.member = source["member"];
	    }
	}
	export class SessionInfo {
	    id: string;
	    type: string;
	    title: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.title = source["title"];
	        this.status = source["status"];
	    }
	}
	export class SocksProxy {
	    kind: string;
	    host: string;
	    port: number;
	    user?: string;
	    pass?: string;
	
	    static createFrom(source: any = {}) {
	        return new SocksProxy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.pass = source["pass"];
	    }
	}
	export class Tunnel {
	    id: string;
	    name: string;
	    mode: string;
	    sshConnId: string;
	    listenHost?: string;
	    listenPort: number;
	    targetHost?: string;
	    targetPort?: number;
	    upstream?: SocksProxy;
	    autoStart?: boolean;
	    groupId?: string;
	    sortOrder?: number;
	
	    static createFrom(source: any = {}) {
	        return new Tunnel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.mode = source["mode"];
	        this.sshConnId = source["sshConnId"];
	        this.listenHost = source["listenHost"];
	        this.listenPort = source["listenPort"];
	        this.targetHost = source["targetHost"];
	        this.targetPort = source["targetPort"];
	        this.upstream = this.convertValues(source["upstream"], SocksProxy);
	        this.autoStart = source["autoStart"];
	        this.groupId = source["groupId"];
	        this.sortOrder = source["sortOrder"];
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
	export class TunnelGroup {
	    id: string;
	    name: string;
	    sortOrder?: number;
	
	    static createFrom(source: any = {}) {
	        return new TunnelGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.sortOrder = source["sortOrder"];
	    }
	}
	export class TunnelState {
	    id: string;
	    status: string;
	    localPort?: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new TunnelState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.status = source["status"];
	        this.localPort = source["localPort"];
	        this.error = source["error"];
	    }
	}
	export class TunnelStoreData {
	    version: number;
	    groups: TunnelGroup[];
	    tunnels: Tunnel[];
	
	    static createFrom(source: any = {}) {
	        return new TunnelStoreData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.groups = this.convertValues(source["groups"], TunnelGroup);
	        this.tunnels = this.convertValues(source["tunnels"], Tunnel);
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

export namespace store {
	
	export class AIConfig {
	    apiKey: string;
	    baseURL: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new AIConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiKey = source["apiKey"];
	        this.baseURL = source["baseURL"];
	        this.model = source["model"];
	    }
	}
	export class AIMessageEntry {
	    id: string;
	    role: string;
	    content: string;
	    tool_call_id?: string;
	    tool_calls?: any[];
	    pendingTools?: any[];
	    _rawApiMsg?: string;
	
	    static createFrom(source: any = {}) {
	        return new AIMessageEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.tool_call_id = source["tool_call_id"];
	        this.tool_calls = source["tool_calls"];
	        this.pendingTools = source["pendingTools"];
	        this._rawApiMsg = source["_rawApiMsg"];
	    }
	}
	export class AIModelConfig {
	    id: string;
	    name: string;
	    apiKey: string;
	    baseURL: string;
	    model: string;
	    protocol: string;
	
	    static createFrom(source: any = {}) {
	        return new AIModelConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.apiKey = source["apiKey"];
	        this.baseURL = source["baseURL"];
	        this.model = source["model"];
	        this.protocol = source["protocol"];
	    }
	}
	export class AISessionEntry {
	    id: string;
	    name: string;
	    createdAt: number;
	    updatedAt: number;
	    messages: AIMessageEntry[];
	
	    static createFrom(source: any = {}) {
	        return new AISessionEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.messages = this.convertValues(source["messages"], AIMessageEntry);
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
	export class AISessionData {
	    sessions: AISessionEntry[];
	    currentSessionId: string;
	
	    static createFrom(source: any = {}) {
	        return new AISessionData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], AISessionEntry);
	        this.currentSessionId = source["currentSessionId"];
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
	
	export class AISettings {
	    models: AIModelConfig[];
	    activeModelId: string;
	
	    static createFrom(source: any = {}) {
	        return new AISettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.models = this.convertValues(source["models"], AIModelConfig);
	        this.activeModelId = source["activeModelId"];
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
	export class TerminalThemeColors {
	    background: string;
	    foreground: string;
	    cursor: string;
	    selection: string;
	    black: string;
	    red: string;
	    green: string;
	    yellow: string;
	    blue: string;
	    magenta: string;
	    cyan: string;
	    white: string;
	    brightBlack: string;
	    brightRed: string;
	    brightGreen: string;
	    brightYellow: string;
	    brightBlue: string;
	    brightMagenta: string;
	    brightCyan: string;
	    brightWhite: string;
	
	    static createFrom(source: any = {}) {
	        return new TerminalThemeColors(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.background = source["background"];
	        this.foreground = source["foreground"];
	        this.cursor = source["cursor"];
	        this.selection = source["selection"];
	        this.black = source["black"];
	        this.red = source["red"];
	        this.green = source["green"];
	        this.yellow = source["yellow"];
	        this.blue = source["blue"];
	        this.magenta = source["magenta"];
	        this.cyan = source["cyan"];
	        this.white = source["white"];
	        this.brightBlack = source["brightBlack"];
	        this.brightRed = source["brightRed"];
	        this.brightGreen = source["brightGreen"];
	        this.brightYellow = source["brightYellow"];
	        this.brightBlue = source["brightBlue"];
	        this.brightMagenta = source["brightMagenta"];
	        this.brightCyan = source["brightCyan"];
	        this.brightWhite = source["brightWhite"];
	    }
	}
	export class CustomTerminalTheme {
	    id: string;
	    name: string;
	    type: string;
	    colors: TerminalThemeColors;
	
	    static createFrom(source: any = {}) {
	        return new CustomTerminalTheme(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.colors = this.convertValues(source["colors"], TerminalThemeColors);
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
	export class SFTPBookmarks {
	    localPaths: string[];
	    remotePaths: string[];
	
	    static createFrom(source: any = {}) {
	        return new SFTPBookmarks(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.localPaths = source["localPaths"];
	        this.remotePaths = source["remotePaths"];
	    }
	}
	export class KeyBinding {
	    ctrl: boolean;
	    shift: boolean;
	    alt: boolean;
	    key: string;
	
	    static createFrom(source: any = {}) {
	        return new KeyBinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ctrl = source["ctrl"];
	        this.shift = source["shift"];
	        this.alt = source["alt"];
	        this.key = source["key"];
	    }
	}
	export class TerminalSettings {
	    theme: string;
	    fontFamily: string;
	    fontSize: number;
	    selectionAction: string;
	    rightClickAction: string;
	    maxHistoryLines: number;
	    smartCompletion?: boolean;
	    highlightEnabled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TerminalSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.fontFamily = source["fontFamily"];
	        this.fontSize = source["fontSize"];
	        this.selectionAction = source["selectionAction"];
	        this.rightClickAction = source["rightClickAction"];
	        this.maxHistoryLines = source["maxHistoryLines"];
	        this.smartCompletion = source["smartCompletion"];
	        this.highlightEnabled = source["highlightEnabled"];
	    }
	}
	export class AppSettings {
	    theme: string;
	    language: string;
	    terminal: TerminalSettings;
	    ai: AISettings;
	    keyboard: Record<string, KeyBinding>;
	    autoCheckUpdate?: boolean;
	    sftpBookmarks: SFTPBookmarks;
	    customTerminalThemes: CustomTerminalTheme[];
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.language = source["language"];
	        this.terminal = this.convertValues(source["terminal"], TerminalSettings);
	        this.ai = this.convertValues(source["ai"], AISettings);
	        this.keyboard = this.convertValues(source["keyboard"], KeyBinding, true);
	        this.autoCheckUpdate = source["autoCheckUpdate"];
	        this.sftpBookmarks = this.convertValues(source["sftpBookmarks"], SFTPBookmarks);
	        this.customTerminalThemes = this.convertValues(source["customTerminalThemes"], CustomTerminalTheme);
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
	
	export class HistoryEntry {
	    id: string;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.command = source["command"];
	    }
	}
	
	export class LocalState {
	    sidebarVisible: boolean;
	    aiSidebarVisible: boolean;
	    windowX: number;
	    windowY: number;
	    windowWidth: number;
	    windowHeight: number;
	    windowMaximised: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LocalState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sidebarVisible = source["sidebarVisible"];
	        this.aiSidebarVisible = source["aiSidebarVisible"];
	        this.windowX = source["windowX"];
	        this.windowY = source["windowY"];
	        this.windowWidth = source["windowWidth"];
	        this.windowHeight = source["windowHeight"];
	        this.windowMaximised = source["windowMaximised"];
	    }
	}
	export class QuickCommand {
	    id: string;
	    name?: string;
	    command: string;
	    groupId?: string;
	    sortOrder: number;
	
	    static createFrom(source: any = {}) {
	        return new QuickCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.command = source["command"];
	        this.groupId = source["groupId"];
	        this.sortOrder = source["sortOrder"];
	    }
	}
	export class QuickCommandGroup {
	    id: string;
	    name: string;
	    sortOrder: number;
	
	    static createFrom(source: any = {}) {
	        return new QuickCommandGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.sortOrder = source["sortOrder"];
	    }
	}
	export class QuickCommandData {
	    version: number;
	    groups: QuickCommandGroup[];
	    commands: QuickCommand[];
	
	    static createFrom(source: any = {}) {
	        return new QuickCommandData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.groups = this.convertValues(source["groups"], QuickCommandGroup);
	        this.commands = this.convertValues(source["commands"], QuickCommand);
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

export namespace sync {
	
	export class ConflictInfo {
	    // Go type: time
	    localTime: any;
	    // Go type: time
	    remoteTime: any;
	
	    static createFrom(source: any = {}) {
	        return new ConflictInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.localTime = this.convertValues(source["localTime"], null);
	        this.remoteTime = this.convertValues(source["remoteTime"], null);
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
	export class SyncConfig {
	    repoUrl: string;
	    branch: string;
	    username: string;
	    autoSync: boolean;
	    // Go type: time
	    lastSyncAt: any;
	    lastSyncStatus: string;
	    lastSyncError: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repoUrl = source["repoUrl"];
	        this.branch = source["branch"];
	        this.username = source["username"];
	        this.autoSync = source["autoSync"];
	        this.lastSyncAt = this.convertValues(source["lastSyncAt"], null);
	        this.lastSyncStatus = source["lastSyncStatus"];
	        this.lastSyncError = source["lastSyncError"];
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
	export class SyncResult {
	    direction: number;
	    message: string;
	    conflict?: ConflictInfo;
	
	    static createFrom(source: any = {}) {
	        return new SyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.direction = source["direction"];
	        this.message = source["message"];
	        this.conflict = this.convertValues(source["conflict"], ConflictInfo);
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

export namespace update {
	
	export class UpdateInfo {
	    hasUpdate: boolean;
	    current: string;
	    latest: string;
	    releaseUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasUpdate = source["hasUpdate"];
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.releaseUrl = source["releaseUrl"];
	    }
	}

}


export namespace main {
	
	export class APIKeyItem {
	    api_key: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new APIKeyItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.api_key = source["api_key"];
	        this.remark = source["remark"];
	    }
	}
	export class AutoSetupClashResult {
	    ok: boolean;
	    error?: string;
	    hint?: string;
	    group?: string;
	    node_count?: number;
	    from?: string;
	    to?: string;
	
	    static createFrom(source: any = {}) {
	        return new AutoSetupClashResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.error = source["error"];
	        this.hint = source["hint"];
	        this.group = source["group"];
	        this.node_count = source["node_count"];
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class CleanupCategory {
	    id: string;
	    name: string;
	    description: string;
	    size_bytes: number;
	    size_human: string;
	    file_count: number;
	    safe: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CleanupCategory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.size_bytes = source["size_bytes"];
	        this.size_human = source["size_human"];
	        this.file_count = source["file_count"];
	        this.safe = source["safe"];
	    }
	}
	export class CleanupResult {
	    category: string;
	    success: boolean;
	    freed_bytes: number;
	    freed_human: string;
	    deleted_dirs: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new CleanupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.success = source["success"];
	        this.freed_bytes = source["freed_bytes"];
	        this.freed_human = source["freed_human"];
	        this.deleted_dirs = source["deleted_dirs"];
	        this.error = source["error"];
	    }
	}
	export class DiagnoseCheck {
	    id: string;
	    title: string;
	    status: string;
	    detail: string;
	    fix_hint: string;
	
	    static createFrom(source: any = {}) {
	        return new DiagnoseCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	        this.fix_hint = source["fix_hint"];
	    }
	}
	export class DiagnoseReport {
	    platform: string;
	    arch: string;
	    ok: number;
	    warn: number;
	    error: number;
	    checks: DiagnoseCheck[];
	
	    static createFrom(source: any = {}) {
	        return new DiagnoseReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platform = source["platform"];
	        this.arch = source["arch"];
	        this.ok = source["ok"];
	        this.warn = source["warn"];
	        this.error = source["error"];
	        this.checks = this.convertValues(source["checks"], DiagnoseCheck);
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
	export class EmailAPIKeyItem {
	    email: string;
	    api_key: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new EmailAPIKeyItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.api_key = source["api_key"];
	        this.remark = source["remark"];
	    }
	}
	export class EmailPasswordItem {
	    email: string;
	    password: string;
	    alt_password?: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new EmailPasswordItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.password = source["password"];
	        this.alt_password = source["alt_password"];
	        this.remark = source["remark"];
	    }
	}
	export class ImportResult {
	    email: string;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class JWTItem {
	    jwt: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new JWTItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jwt = source["jwt"];
	        this.remark = source["remark"];
	    }
	}
	export class JailbreakRuntime {
	    enabled: boolean;
	    preset_id: string;
	    source: string;
	    active_text: string;
	    active_length: number;
	    file_path?: string;
	    file_status?: services.JailbreakFileStatus;
	    stats: services.JailbreakStatsSnapshot;
	    warn_anthropic: boolean;
	
	    static createFrom(source: any = {}) {
	        return new JailbreakRuntime(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.preset_id = source["preset_id"];
	        this.source = source["source"];
	        this.active_text = source["active_text"];
	        this.active_length = source["active_length"];
	        this.file_path = source["file_path"];
	        this.file_status = this.convertValues(source["file_status"], services.JailbreakFileStatus);
	        this.stats = this.convertValues(source["stats"], services.JailbreakStatsSnapshot);
	        this.warn_anthropic = source["warn_anthropic"];
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
	export class ManualPinStatus {
	    enabled: boolean;
	    account_id?: string;
	    email?: string;
	    nickname?: string;
	
	    static createFrom(source: any = {}) {
	        return new ManualPinStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.account_id = source["account_id"];
	        this.email = source["email"];
	        this.nickname = source["nickname"];
	    }
	}
	export class PerformanceTip {
	    id: string;
	    title: string;
	    description: string;
	    impact: string;
	    auto_fix: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PerformanceTip(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.impact = source["impact"];
	        this.auto_fix = source["auto_fix"];
	    }
	}
	export class PrereqStepResult {
	    step: string;
	    title: string;
	    ok: boolean;
	    skipped: boolean;
	    error?: string;
	    hint?: string;
	
	    static createFrom(source: any = {}) {
	        return new PrereqStepResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.step = source["step"];
	        this.title = source["title"];
	        this.ok = source["ok"];
	        this.skipped = source["skipped"];
	        this.error = source["error"];
	        this.hint = source["hint"];
	    }
	}
	export class ProviderKeyItem {
	    provider: string;
	    base_url: string;
	    token: string;
	    remark: string;
	    nickname: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderKeyItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.base_url = source["base_url"];
	        this.token = source["token"];
	        this.remark = source["remark"];
	        this.nickname = source["nickname"];
	    }
	}
	export class RotationPoolStatus {
	    enabled: boolean;
	    member_count: number;
	    interval_min: number;
	    quota_refresh_min: number;
	    next_switch_at?: string;
	    last_switched_to?: string;
	    last_switched_at?: string;
	    last_quota_refresh_at?: string;
	    last_error?: string;
	    total_switches: number;
	    total_quota_refreshes: number;
	    paused_by_pin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RotationPoolStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.member_count = source["member_count"];
	        this.interval_min = source["interval_min"];
	        this.quota_refresh_min = source["quota_refresh_min"];
	        this.next_switch_at = source["next_switch_at"];
	        this.last_switched_to = source["last_switched_to"];
	        this.last_switched_at = source["last_switched_at"];
	        this.last_quota_refresh_at = source["last_quota_refresh_at"];
	        this.last_error = source["last_error"];
	        this.total_switches = source["total_switches"];
	        this.total_quota_refreshes = source["total_quota_refreshes"];
	        this.paused_by_pin = source["paused_by_pin"];
	    }
	}
	export class TokenItem {
	    token: string;
	    remark: string;
	
	    static createFrom(source: any = {}) {
	        return new TokenItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.remark = source["remark"];
	    }
	}
	export class UpstreamProxyStatus {
	    source: string;
	    url: string;
	    last_applied_at: string;
	
	    static createFrom(source: any = {}) {
	        return new UpstreamProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.url = source["url"];
	        this.last_applied_at = source["last_applied_at"];
	    }
	}
	export class WindsurfDiskUsage {
	    categories: CleanupCategory[];
	    total_bytes: number;
	    total_human: string;
	
	    static createFrom(source: any = {}) {
	        return new WindsurfDiskUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.categories = this.convertValues(source["categories"], CleanupCategory);
	        this.total_bytes = source["total_bytes"];
	        this.total_human = source["total_human"];
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

export namespace models {
	
	export class Account {
	    id: string;
	    email: string;
	    password?: string;
	    nickname: string;
	    token?: string;
	    refresh_token?: string;
	    windsurf_api_key?: string;
	    plan_name: string;
	    used_quota: number;
	    total_quota: number;
	    daily_remaining: string;
	    weekly_remaining: string;
	    daily_reset_at: string;
	    weekly_reset_at: string;
	    subscription_expires_at: string;
	    token_expires_at: string;
	    status: string;
	    tags: string;
	    remark: string;
	    last_login_at: string;
	    last_quota_update: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.password = source["password"];
	        this.nickname = source["nickname"];
	        this.token = source["token"];
	        this.refresh_token = source["refresh_token"];
	        this.windsurf_api_key = source["windsurf_api_key"];
	        this.plan_name = source["plan_name"];
	        this.used_quota = source["used_quota"];
	        this.total_quota = source["total_quota"];
	        this.daily_remaining = source["daily_remaining"];
	        this.weekly_remaining = source["weekly_remaining"];
	        this.daily_reset_at = source["daily_reset_at"];
	        this.weekly_reset_at = source["weekly_reset_at"];
	        this.subscription_expires_at = source["subscription_expires_at"];
	        this.token_expires_at = source["token_expires_at"];
	        this.status = source["status"];
	        this.tags = source["tags"];
	        this.remark = source["remark"];
	        this.last_login_at = source["last_login_at"];
	        this.last_quota_update = source["last_quota_update"];
	        this.created_at = source["created_at"];
	    }
	}
	export class ProviderAccount {
	    id: string;
	    provider: string;
	    base_url: string;
	    auth_token: string;
	    nickname?: string;
	    remark?: string;
	    status: string;
	    created_at: string;
	    last_used_at?: string;
	    used_quota?: number;
	    total_quota?: number;
	
	    static createFrom(source: any = {}) {
	        return new ProviderAccount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.base_url = source["base_url"];
	        this.auth_token = source["auth_token"];
	        this.nickname = source["nickname"];
	        this.remark = source["remark"];
	        this.status = source["status"];
	        this.created_at = source["created_at"];
	        this.last_used_at = source["last_used_at"];
	        this.used_quota = source["used_quota"];
	        this.total_quota = source["total_quota"];
	    }
	}
	export class Settings {
	    concurrent_limit: number;
	    auto_refresh_tokens: boolean;
	    auto_refresh_quotas: boolean;
	    quota_refresh_policy: string;
	    quota_custom_interval_minutes: number;
	    auto_switch_plan_filter: string;
	    auto_switch_on_quota_exhausted: boolean;
	    manual_pin_enabled: boolean;
	    manual_pin_account_id: string;
	    rotation_pool_enabled: boolean;
	    rotation_pool_account_ids: string[];
	    rotation_pool_interval_min: number;
	    rotation_pool_quota_refresh_min: number;
	    quota_hot_poll_seconds: number;
	    minimize_to_tray: boolean;
	    desktop_notifications: boolean;
	    silent_start: boolean;
	    mitm_route_mode: string;
	    mitm_debug_dump: boolean;
	    mitm_full_capture: boolean;
	    static_cache_intercept: boolean;
	    mitm_jailbreak_enabled: boolean;
	    mitm_jailbreak_override: string;
	    mitm_jailbreak_preset_id: string;
	    mitm_jailbreak_override_source: string;
	    mitm_jailbreak_override_file: string;
	    forge_enabled: boolean;
	    fake_credits: number;
	    fake_credits_premium: number;
	    fake_credits_other: number;
	    fake_credits_used: number;
	    fake_subscription_type: string;
	    fake_billing_extend_years: number;
	    debug_log: boolean;
	    import_concurrency: number;
	    openai_relay_enabled: boolean;
	    openai_relay_port: number;
	    openai_relay_secret: string;
	    clash_rotate_enabled: boolean;
	    clash_controller_url: string;
	    clash_secret: string;
	    clash_group: string;
	    clash_nodes: string;
	    clash_interval_minutes: number;
	    clash_rotate_on_rate_limit: boolean;
	    clash_latency_test_url: string;
	    clash_latency_max_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.concurrent_limit = source["concurrent_limit"];
	        this.auto_refresh_tokens = source["auto_refresh_tokens"];
	        this.auto_refresh_quotas = source["auto_refresh_quotas"];
	        this.quota_refresh_policy = source["quota_refresh_policy"];
	        this.quota_custom_interval_minutes = source["quota_custom_interval_minutes"];
	        this.auto_switch_plan_filter = source["auto_switch_plan_filter"];
	        this.auto_switch_on_quota_exhausted = source["auto_switch_on_quota_exhausted"];
	        this.manual_pin_enabled = source["manual_pin_enabled"];
	        this.manual_pin_account_id = source["manual_pin_account_id"];
	        this.rotation_pool_enabled = source["rotation_pool_enabled"];
	        this.rotation_pool_account_ids = source["rotation_pool_account_ids"];
	        this.rotation_pool_interval_min = source["rotation_pool_interval_min"];
	        this.rotation_pool_quota_refresh_min = source["rotation_pool_quota_refresh_min"];
	        this.quota_hot_poll_seconds = source["quota_hot_poll_seconds"];
	        this.minimize_to_tray = source["minimize_to_tray"];
	        this.desktop_notifications = source["desktop_notifications"];
	        this.silent_start = source["silent_start"];
	        this.mitm_route_mode = source["mitm_route_mode"];
	        this.mitm_debug_dump = source["mitm_debug_dump"];
	        this.mitm_full_capture = source["mitm_full_capture"];
	        this.static_cache_intercept = source["static_cache_intercept"];
	        this.mitm_jailbreak_enabled = source["mitm_jailbreak_enabled"];
	        this.mitm_jailbreak_override = source["mitm_jailbreak_override"];
	        this.mitm_jailbreak_preset_id = source["mitm_jailbreak_preset_id"];
	        this.mitm_jailbreak_override_source = source["mitm_jailbreak_override_source"];
	        this.mitm_jailbreak_override_file = source["mitm_jailbreak_override_file"];
	        this.forge_enabled = source["forge_enabled"];
	        this.fake_credits = source["fake_credits"];
	        this.fake_credits_premium = source["fake_credits_premium"];
	        this.fake_credits_other = source["fake_credits_other"];
	        this.fake_credits_used = source["fake_credits_used"];
	        this.fake_subscription_type = source["fake_subscription_type"];
	        this.fake_billing_extend_years = source["fake_billing_extend_years"];
	        this.debug_log = source["debug_log"];
	        this.import_concurrency = source["import_concurrency"];
	        this.openai_relay_enabled = source["openai_relay_enabled"];
	        this.openai_relay_port = source["openai_relay_port"];
	        this.openai_relay_secret = source["openai_relay_secret"];
	        this.clash_rotate_enabled = source["clash_rotate_enabled"];
	        this.clash_controller_url = source["clash_controller_url"];
	        this.clash_secret = source["clash_secret"];
	        this.clash_group = source["clash_group"];
	        this.clash_nodes = source["clash_nodes"];
	        this.clash_interval_minutes = source["clash_interval_minutes"];
	        this.clash_rotate_on_rate_limit = source["clash_rotate_on_rate_limit"];
	        this.clash_latency_test_url = source["clash_latency_test_url"];
	        this.clash_latency_max_ms = source["clash_latency_max_ms"];
	    }
	}

}

export namespace services {
	
	export class AutoDetectClashGroupResult {
	    ok: boolean;
	    error?: string;
	    group: string;
	    node_count: number;
	    candidates: string[];
	    all_groups: string[];
	
	    static createFrom(source: any = {}) {
	        return new AutoDetectClashGroupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.error = source["error"];
	        this.group = source["group"];
	        this.node_count = source["node_count"];
	        this.candidates = source["candidates"];
	        this.all_groups = source["all_groups"];
	    }
	}
	export class ClashProbeResult {
	    ok: boolean;
	    error?: string;
	    groups: string[];
	
	    static createFrom(source: any = {}) {
	        return new ClashProbeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.error = source["error"];
	        this.groups = source["groups"];
	    }
	}
	export class JailbreakFileStatus {
	    path: string;
	    exists: boolean;
	    size: number;
	    charset: string;
	    excerpt: string;
	    truncated: boolean;
	    is_dir: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new JailbreakFileStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.exists = source["exists"];
	        this.size = source["size"];
	        this.charset = source["charset"];
	        this.excerpt = source["excerpt"];
	        this.truncated = source["truncated"];
	        this.is_dir = source["is_dir"];
	        this.error = source["error"];
	    }
	}
	export class JailbreakPreset {
	    id: string;
	    name: string;
	    description: string;
	    risk: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new JailbreakPreset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.risk = source["risk"];
	        this.text = source["text"];
	    }
	}
	export class JailbreakStatsSnapshot {
	    total_injects: number;
	    today_injects: number;
	    last_inject_at?: string;
	    since_last_inject_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new JailbreakStatsSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_injects = source["total_injects"];
	        this.today_injects = source["today_injects"];
	        this.last_inject_at = source["last_inject_at"];
	        this.since_last_inject_ms = source["since_last_inject_ms"];
	    }
	}
	export class MitmProxyEvent {
	    at: string;
	    message: string;
	    tone: string;
	
	    static createFrom(source: any = {}) {
	        return new MitmProxyEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = source["at"];
	        this.message = source["message"];
	        this.tone = source["tone"];
	    }
	}
	export class SessionBindingInfo {
	    conv_id: string;
	    conv_id_short: string;
	    pool_key_short: string;
	    pool_key_hash: string;
	    bound_at: string;
	    last_seen_at: string;
	    request_count: number;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionBindingInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conv_id = source["conv_id"];
	        this.conv_id_short = source["conv_id_short"];
	        this.pool_key_short = source["pool_key_short"];
	        this.pool_key_hash = source["pool_key_hash"];
	        this.bound_at = source["bound_at"];
	        this.last_seen_at = source["last_seen_at"];
	        this.request_count = source["request_count"];
	        this.title = source["title"];
	    }
	}
	export class PoolKeyInfo {
	    key_short: string;
	    key_hash: string;
	    plan: string;
	    healthy: boolean;
	    disabled: boolean;
	    runtime_exhausted: boolean;
	    cooldown_until: string;
	    has_jwt: boolean;
	    request_count: number;
	    success_count: number;
	    total_exhausted: number;
	    is_current: boolean;
	    bound_session_count: number;
	    email?: string;
	    nickname?: string;
	
	    static createFrom(source: any = {}) {
	        return new PoolKeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key_short = source["key_short"];
	        this.key_hash = source["key_hash"];
	        this.plan = source["plan"];
	        this.healthy = source["healthy"];
	        this.disabled = source["disabled"];
	        this.runtime_exhausted = source["runtime_exhausted"];
	        this.cooldown_until = source["cooldown_until"];
	        this.has_jwt = source["has_jwt"];
	        this.request_count = source["request_count"];
	        this.success_count = source["success_count"];
	        this.total_exhausted = source["total_exhausted"];
	        this.is_current = source["is_current"];
	        this.bound_session_count = source["bound_session_count"];
	        this.email = source["email"];
	        this.nickname = source["nickname"];
	    }
	}
	export class MitmProxyStatus {
	    running: boolean;
	    port: number;
	    hosts_mapped: boolean;
	    ca_installed: boolean;
	    current_key: string;
	    pool_status: PoolKeyInfo[];
	    total_requests: number;
	    active_sessions: SessionBindingInfo[];
	    session_count: number;
	    last_error_kind: string;
	    last_error_summary: string;
	    last_error_at: string;
	    last_error_key: string;
	    recent_events: MitmProxyEvent[];
	
	    static createFrom(source: any = {}) {
	        return new MitmProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.hosts_mapped = source["hosts_mapped"];
	        this.ca_installed = source["ca_installed"];
	        this.current_key = source["current_key"];
	        this.pool_status = this.convertValues(source["pool_status"], PoolKeyInfo);
	        this.total_requests = source["total_requests"];
	        this.active_sessions = this.convertValues(source["active_sessions"], SessionBindingInfo);
	        this.session_count = source["session_count"];
	        this.last_error_kind = source["last_error_kind"];
	        this.last_error_summary = source["last_error_summary"];
	        this.last_error_at = source["last_error_at"];
	        this.last_error_key = source["last_error_key"];
	        this.recent_events = this.convertValues(source["recent_events"], MitmProxyEvent);
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
	export class OpenAIRelayStatus {
	    running: boolean;
	    port: number;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new OpenAIRelayStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.url = source["url"];
	    }
	}
	
	
	export class UsageRecord {
	    id: string;
	    at: string;
	    model: string;
	    request_model: string;
	    prompt_tokens: number;
	    completion_tokens: number;
	    total_tokens: number;
	    duration_ms: number;
	    api_key_short: string;
	    status: string;
	    error_detail?: string;
	    format: string;
	
	    static createFrom(source: any = {}) {
	        return new UsageRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = source["at"];
	        this.model = source["model"];
	        this.request_model = source["request_model"];
	        this.prompt_tokens = source["prompt_tokens"];
	        this.completion_tokens = source["completion_tokens"];
	        this.total_tokens = source["total_tokens"];
	        this.duration_ms = source["duration_ms"];
	        this.api_key_short = source["api_key_short"];
	        this.status = source["status"];
	        this.error_detail = source["error_detail"];
	        this.format = source["format"];
	    }
	}
	export class UsageSummary {
	    total_requests: number;
	    total_prompt_tokens: number;
	    total_completion_tokens: number;
	    total_tokens: number;
	    by_model: Record<string, number>;
	    by_model_tokens: Record<string, number>;
	    by_date: Record<string, number>;
	    by_date_tokens: Record<string, number>;
	    error_count: number;
	    estimated_cost_usd: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_requests = source["total_requests"];
	        this.total_prompt_tokens = source["total_prompt_tokens"];
	        this.total_completion_tokens = source["total_completion_tokens"];
	        this.total_tokens = source["total_tokens"];
	        this.by_model = source["by_model"];
	        this.by_model_tokens = source["by_model_tokens"];
	        this.by_date = source["by_date"];
	        this.by_date_tokens = source["by_date_tokens"];
	        this.error_count = source["error_count"];
	        this.estimated_cost_usd = source["estimated_cost_usd"];
	    }
	}

}


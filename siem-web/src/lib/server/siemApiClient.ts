import type { AlertSeverity } from '../severity';

export interface LogEntry {
	Timestamp: string;
	Labels: Record<string, string>;
	Line: string;
}

export interface EventsStatsResponse {
	event_count_24h: number;
	heat_grid: { source: string; hours: string[] }[];
	hourly_totals: { hour_start: string; count: number }[];
}

export interface NavSummaryResponse {
	events_per_min: number;
	open_alert_count: number;
}

export interface AlertResponse {
	id: number;
	rule_id: number;
	group_key: string;
	severity: AlertSeverity;
	title: string;
	body: string;
	event_count: number;
	state: string;
	first_seen_at: string;
	last_seen_at: string;
	acked_by?: number;
	acked_at?: string;
}

export interface AlertSample {
	id: number;
	ts: string;
	line: string;
}

export interface RuleResponse {
	id: number;
	name: string;
	shape: string;
	logql: string;
	window_sec: number;
	threshold?: number;
	group_by: string[];
	severity: AlertSeverity;
	destinations: string[];
	cooldown_sec: number;
	interval_sec: number;
	enabled: boolean;
	last_run_at?: string;
}

export interface CreateRuleRequest {
	name: string;
	shape: string;
	logql: string;
	window_sec: number;
	threshold?: number;
	group_by: string[];
	severity: AlertSeverity;
	destinations: string[];
	cooldown_sec: number;
	interval_sec: number;
	enabled: boolean;
}

export interface VolumeBucket {
	bucket_start: string;
	count: number;
}

export interface SearchResponse {
	logql: string;
	count: number;
	entries: LogEntry[];
	volume: VolumeBucket[];
}

export interface SourceResponse {
	id: number;
	name: string;
	address: string;
	transport: string;
	parser: string;
	claimed: boolean;
	heartbeat_sec: number;
	last_seen_at?: string;
	status: 'healthy' | 'silent';
	events_per_min: number;
}

export interface IngestHealthResponse {
	received_events_per_source: Record<string, number>;
	loki_sent_events_total: number;
	degraded: boolean;
}

export interface EstablishSessionRequest {
	subject: string;
	email: string;
	display_name: string;
	groups: string[];
}

export interface EstablishSessionResponse {
	user_id: number;
	role: string;
	display_name: string;
}

export interface RoleMappingResponse {
	id: number;
	group_claim: string;
	role: string;
	priority: number;
}

export interface AuthSettingsResponse {
	oidc_issuer: string;
	oidc_client_id: string;
	oidc_groups_scope: string;
	role_mappings: RoleMappingResponse[] | null;
}

export interface UpdateRoleMappingsRequest {
	role_mappings: { group_claim: string; role: string; priority: number }[];
}

export interface NotificationSettingsResponse {
	ntfy_configured: boolean;
	min_severity: string;
}

export class SiemApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.name = 'SiemApiError';
		this.status = status;
	}
}

export class SiemApiClient {
	private baseUrl: string;
	private fetchFn: typeof fetch;

	constructor(config: { baseUrl: string }, fetchFn: typeof fetch = fetch) {
		this.baseUrl = config.baseUrl;
		this.fetchFn = fetchFn;
	}

	async establishSession(req: EstablishSessionRequest): Promise<EstablishSessionResponse> {
		return this.request<EstablishSessionResponse>('/auth/session', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(req)
		});
	}

	async getEventsStats(sessionToken: string): Promise<EventsStatsResponse> {
		return this.request<EventsStatsResponse>('/events/stats', this.authInit(sessionToken));
	}

	async getNavSummary(sessionToken: string): Promise<NavSummaryResponse> {
		return this.request<NavSummaryResponse>('/nav/summary', this.authInit(sessionToken));
	}

	async getAlerts(sessionToken: string, state?: string): Promise<AlertResponse[]> {
		const path = state ? `/alerts?state=${encodeURIComponent(state)}` : '/alerts';
		return this.request<AlertResponse[]>(path, this.authInit(sessionToken));
	}

	async ackAlert(sessionToken: string, id: number): Promise<void> {
		return this.requestNoContent(`/alerts/${id}/ack`, {
			method: 'POST',
			...this.authInit(sessionToken)
		});
	}

	async muteAlert(sessionToken: string, id: number): Promise<void> {
		return this.requestNoContent(`/alerts/${id}/mute`, {
			method: 'POST',
			...this.authInit(sessionToken)
		});
	}

	async getAlertSamples(sessionToken: string, id: number): Promise<AlertSample[]> {
		return this.request<AlertSample[]>(`/alerts/${id}/samples`, this.authInit(sessionToken));
	}

	async getRules(sessionToken: string): Promise<RuleResponse[]> {
		return this.request<RuleResponse[]>('/rules', this.authInit(sessionToken));
	}

	async createRule(sessionToken: string, req: CreateRuleRequest): Promise<RuleResponse> {
		return this.request<RuleResponse>('/rules', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', ...this.authInit(sessionToken).headers },
			body: JSON.stringify(req)
		});
	}

	async getSources(sessionToken: string): Promise<SourceResponse[]> {
		return this.request<SourceResponse[]>('/sources', this.authInit(sessionToken));
	}

	async getIngestHealth(sessionToken: string): Promise<IngestHealthResponse> {
		return this.request<IngestHealthResponse>(
			'/sources/ingest-health',
			this.authInit(sessionToken)
		);
	}

	async claimSource(sessionToken: string, id: number): Promise<void> {
		return this.requestNoContent(`/sources/${id}/claim`, {
			method: 'POST',
			...this.authInit(sessionToken)
		});
	}

	async search(sessionToken: string, params: Record<string, string>): Promise<SearchResponse> {
		const qs = new URLSearchParams(params).toString();
		const path = qs ? `/events/search?${qs}` : '/events/search';
		return this.request<SearchResponse>(path, this.authInit(sessionToken));
	}

	async getAuthSettings(sessionToken: string): Promise<AuthSettingsResponse> {
		return this.request<AuthSettingsResponse>('/settings/auth', this.authInit(sessionToken));
	}

	async updateRoleMappings(sessionToken: string, req: UpdateRoleMappingsRequest): Promise<void> {
		return this.requestNoContent('/settings/auth', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json', ...this.authInit(sessionToken).headers },
			body: JSON.stringify(req)
		});
	}

	async getNotificationSettings(sessionToken: string): Promise<NotificationSettingsResponse> {
		return this.request<NotificationSettingsResponse>(
			'/settings/notifications',
			this.authInit(sessionToken)
		);
	}

	async updateNotificationSettings(sessionToken: string, minSeverity: string): Promise<void> {
		return this.requestNoContent('/settings/notifications', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json', ...this.authInit(sessionToken).headers },
			body: JSON.stringify({ min_severity: minSeverity })
		});
	}

	async testNotification(sessionToken: string): Promise<void> {
		await this.request('/settings/notifications/test', {
			method: 'POST',
			...this.authInit(sessionToken)
		});
	}

	private authInit(sessionToken: string): RequestInit {
		return { headers: { Authorization: `Bearer ${sessionToken}` } };
	}

	private async request<T>(path: string, init: RequestInit): Promise<T> {
		const res = await this.fetchFn(`${this.baseUrl}${path}`, init);
		if (!res.ok) {
			throw new SiemApiError(res.status, await res.text());
		}
		return res.json() as Promise<T>;
	}

	private async requestNoContent(path: string, init: RequestInit): Promise<void> {
		const res = await this.fetchFn(`${this.baseUrl}${path}`, init);
		if (!res.ok) {
			throw new SiemApiError(res.status, await res.text());
		}
	}
}

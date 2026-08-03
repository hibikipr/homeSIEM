export interface LogEntry {
	Timestamp: string;
	Labels: Record<string, string>;
	Line: string;
}

export interface EventsStatsResponse {
	event_count_24h: number;
	heat_grid: { source: string; hours: string[] }[];
}

export interface AlertResponse {
	id: number;
	rule_id: number;
	group_key: string;
	severity: string;
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
	severity: string;
	destinations: string[];
	cooldown_sec: number;
	interval_sec: number;
	enabled: boolean;
	last_run_at?: string;
}

export interface SearchResponse {
	logql: string;
	count: number;
	entries: LogEntry[];
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

	async search(sessionToken: string, params: Record<string, string>): Promise<SearchResponse> {
		const qs = new URLSearchParams(params).toString();
		const path = qs ? `/events/search?${qs}` : '/events/search';
		return this.request<SearchResponse>(path, this.authInit(sessionToken));
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

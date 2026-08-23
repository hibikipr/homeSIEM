<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type {
		AbsenceContextSource,
		AlertResponse,
		AlertSample,
		RuleResponse,
		SourceResponse
	} from '$lib/server/siemApiClient';
	import type { AlertStats } from '$lib/alerts';
	import { extractMessage } from '$lib/logline';
	import { formatMinuteLabel, formatSecondsAsMinutes } from '$lib/minutePresets';

	let {
		alert,
		samples,
		stats,
		rule,
		sourceDisplayNames,
		liveSources
	}: {
		alert: AlertResponse;
		samples: AlertSample[];
		stats: AlertStats;
		rule: RuleResponse | undefined;
		// Current Sources display_name, keyed by raw source name - see
		// +page.server.ts's fetch for why this is resolved live rather
		// than trusting alert.title/body, which are static text baked in
		// at raise time and never rewritten by a later rename.
		sourceDisplayNames: Record<string, string>;
		// The full live Sources rows behind sourceDisplayNames, same fetch,
		// keyed the same way - the fallback data source for an
		// absence-shaped alert whose stored context has no per-source
		// detail at all (see fallbackSources below).
		liveSources: Record<string, SourceResponse>;
	} = $props();

	// alert.group_key IS the raw source name for source-quiet/first-seen
	// alerts (see rules.AbsenceEvaluator/FirstSeenEvaluator - GroupKey:
	// src.Name / value respectively), but for a threshold alert it can be
	// any grouped field value at all (a port, a username, whatever
	// group_by named) - only show a resolved name when group_key actually
	// matches a known source, never claim an arbitrary group_key IS a
	// source.
	let resolvedSourceName = $derived(sourceDisplayNames[alert.group_key]);

	// The threshold-shaped stat tiles (matched events, ports, source IP,
	// reputation) are meaningless for an absence-shaped alert - source-quiet
	// fires on the *absence* of matching events, so those are always
	// 0/empty/unknown for this shape, not a sign anything's missing. Show
	// the per-source last-seen/heartbeat data AbsenceEvaluator's Context
	// actually carries instead (see siem-api's rules.sourceContext).
	let isAbsence = $derived(rule?.shape === 'absence');
	// What actually fired, straight from the stored alert - the authoritative
	// source when present.
	let historicalSources = $derived<AbsenceContextSource[]>(
		isAbsence && Array.isArray(alert.context?.sources)
			? (alert.context.sources as AbsenceContextSource[])
			: []
	);
	// Falls back to resolving alert.group_key against the live Sources list
	// when the stored context has nothing - either an already-open alert
	// raised before AbsenceEvaluator started attaching Context at all (that
	// row will never get backfilled unless it happens to touch/reopen again
	// under the exact same source or, for a correlated "multi:" alert, the
	// exact same combination - see store.TouchAlert's doc), or a source a
	// stored context did name that's since been deleted from Sources
	// (filtered out here, same "don't claim a resolved name for something
	// that isn't really there" posture as resolvedSourceName below). This is
	// current status, not history - a live-fallback note in the template
	// says so rather than presenting it as what was true when the alert fired.
	let fallbackSources = $derived<AbsenceContextSource[]>(
		isAbsence && historicalSources.length === 0
			? (alert.group_key.startsWith('multi:')
					? alert.group_key.slice('multi:'.length).split(',')
					: [alert.group_key]
				)
					.map((name) => liveSources[name])
					.filter((s): s is SourceResponse => Boolean(s))
					.map((s) => ({
						name: s.name,
						display_name: s.display_name || s.name,
						last_seen_at: s.last_seen_at ?? null,
						heartbeat_sec: s.heartbeat_sec
					}))
			: []
	);
	let usingLiveFallback = $derived(historicalSources.length === 0 && fallbackSources.length > 0);
	let displaySources = $derived(historicalSources.length > 0 ? historicalSources : fallbackSources);
	// One snapshot per render, not a live ticker - matches the rest of this
	// pane (samples list, stats) being a static view of server-loaded data
	// rather than something that updates itself between navigations.
	const nowMs = Date.now();

	// Floors to whole minutes before handing off to formatMinuteLabel -
	// elapsed wall-clock time is essentially never an exact minute multiple,
	// and formatSecondsAsMinutes' raw-seconds fallback for that case (e.g.
	// "11263s") is exactly the ugly display it exists to avoid for clean
	// interval settings, not something we want for a live-computed elapsed
	// duration.
	function elapsedLabel(sinceIso: string, nowMs: number): string {
		const elapsedSec = Math.floor((nowMs - new Date(sinceIso).getTime()) / 1000);
		if (elapsedSec < 60) return 'just now';
		return formatMinuteLabel(Math.floor(elapsedSec / 60)) + ' ago';
	}

	// A source's row here can be showing a stale snapshot from whenever this
	// alert was last touched/reopened (see store.TouchAlert/ReopenAlert) -
	// recomputing status from last_seen_at + heartbeat_sec against the
	// current time, rather than trusting the alert was raised because the
	// source is *still* stale right now, is what surfaces the "this alert is
	// open but the source already recovered" case rather than hiding it.
	function absenceStatus(src: AbsenceContextSource, nowMs: number): string {
		if (!src.last_seen_at) return 'never seen';
		const elapsedSec = (nowMs - new Date(src.last_seen_at).getTime()) / 1000;
		if (elapsedSec < src.heartbeat_sec) return 'recovered';
		return `overdue by ${formatMinuteLabel(Math.floor((elapsedSec - src.heartbeat_sec) / 60))}`;
	}

	let acking = $state(false);
	let muting = $state(false);
	let error = $state<string | null>(null);

	async function acknowledge() {
		acking = true;
		error = null;
		try {
			const response = await fetch(`/api/alerts/${alert.id}/ack`, { method: 'POST' });
			if (!response.ok) {
				error = 'Failed to acknowledge alert.';
				return;
			}
			await invalidateAll();
		} finally {
			acking = false;
		}
	}

	async function mute() {
		muting = true;
		error = null;
		try {
			const response = await fetch(`/api/alerts/${alert.id}/mute`, { method: 'POST' });
			if (!response.ok) {
				error = 'Failed to mute rule.';
				return;
			}
			await invalidateAll();
		} finally {
			muting = false;
		}
	}
</script>

<div class="detail">
	<div class="header">
		<div class="title-block">
			<div class="eyebrow-row">
				<span class="eyebrow severity-{alert.severity}">{alert.severity}</span>
				{#if alert.state === 'open'}
					<span class="tag">unacknowledged</span>
				{/if}
			</div>
			<h1>{alert.title}</h1>
			<p class="body">{alert.body}</p>
			{#if resolvedSourceName && resolvedSourceName !== alert.group_key}
				<p class="renamed-note">
					This alert's source ({alert.group_key}) is now named <strong>{resolvedSourceName}</strong> in
					Sources - the text above is unchanged from when the alert was raised.
				</p>
			{/if}
		</div>
		<div class="actions">
			<button class="primary" onclick={acknowledge} disabled={acking || alert.state !== 'open'}>
				Acknowledge
			</button>
			<button
				class="ghost"
				disabled
				title="Not implemented — SOAR-style automated response is out of scope for v1"
			>
				Block at gateway
			</button>
			<button class="ghost" onclick={mute} disabled={muting}>Mute rule 1h</button>
			{#if error}
				<span class="error">{error}</span>
			{/if}
		</div>
	</div>

	{#if isAbsence}
		<div class="absence-sources">
			<span class="label">Sources affected</span>
			{#if usingLiveFallback}
				<p class="renamed-note">
					This alert predates per-source detail tracking - showing each source's current status
					instead of what was true when it fired.
				</p>
			{/if}
			<div class="absence-table">
				<div class="absence-row absence-head">
					<span>Source</span>
					<span>Last seen</span>
					<span>Heartbeat</span>
					<span>Status</span>
				</div>
				{#each displaySources as src (src.name)}
					<div class="absence-row">
						<span>{src.display_name || src.name}</span>
						<span>{src.last_seen_at ? elapsedLabel(src.last_seen_at, nowMs) : 'never'}</span>
						<span>{formatSecondsAsMinutes(src.heartbeat_sec)}</span>
						<span class="status" class:recovered={absenceStatus(src, nowMs) === 'recovered'}>
							{absenceStatus(src, nowMs)}
						</span>
					</div>
				{:else}
					<div class="absence-row empty-row">No source detail recorded for this alert.</div>
				{/each}
			</div>
		</div>
	{:else}
		<div class="stats">
			<div class="stat">
				<span class="label">Matched events</span>
				<span class="value">{stats.matchedEvents}</span>
			</div>
			<div class="stat">
				<span class="label">Distinct ports</span>
				<span class="value">{stats.distinctPorts.length}</span>
			</div>
			<div class="stat">
				<span class="label">Source IP</span>
				<span class="value">{stats.sourceIps[0] ?? '—'}</span>
			</div>
			<div class="stat">
				<span class="label">Reputation</span>
				<span class="value">{stats.reputation}</span>
			</div>
		</div>

		{#if stats.distinctPorts.length > 0}
			<div class="ports">
				<span class="label">Ports touched, in order</span>
				<div class="chips">
					{#each stats.distinctPorts as port (port)}
						<span class="chip">{port}</span>
					{/each}
				</div>
			</div>
		{/if}

		<div class="matched-events">
			<span class="label">Matched events</span>
			<div class="log-block">
				{#each samples as sample (sample.id)}
					<div class="log-line">{extractMessage(sample.line)}</div>
				{:else}
					<div class="log-line empty">No samples recorded yet.</div>
				{/each}
			</div>
		</div>
	{/if}

	<div class="rule-panel">
		<span class="label">Rule that fired</span>
		{#if rule}
			<div class="rule-name">{rule.name}</div>
			<div class="rule-meta">
				<span class="enabled" class:off={!rule.enabled}
					>{rule.enabled ? 'enabled' : 'disabled'}</span
				>
				<span class="destinations">{rule.destinations.join(', ')}</span>
			</div>
			<div class="logql-block">{rule.logql}</div>
		{:else}
			<div class="rule-name">rule #{alert.rule_id}</div>
		{/if}
	</div>
</div>

<style>
	.detail {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
	}
	.header {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
	}
	.title-block {
		flex: 1 1 380px;
	}
	.eyebrow-row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-info-text);
	}
	.eyebrow.severity-warning {
		color: var(--color-severity-warning);
	}
	.eyebrow.severity-critical {
		color: var(--color-severity-critical);
	}
	.tag {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
		border-radius: var(--radius-sm);
		padding: 0 var(--space-2);
	}
	h1 {
		font-size: 26px;
		font-weight: 500;
		margin: var(--space-2) 0 0;
	}
	.body {
		max-width: 68ch;
		color: var(--color-muted);
		margin-top: var(--space-2);
	}
	.renamed-note {
		max-width: 68ch;
		color: var(--color-muted-2);
		font-size: var(--text-label);
		margin-top: var(--space-1);
	}
	.actions {
		display: flex;
		gap: var(--space-3);
		align-items: flex-start;
	}
	.primary {
		background: transparent;
		border: 1px solid var(--color-accent);
		color: var(--color-text);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-4);
		font-size: var(--text-table);
	}
	.ghost {
		background: none;
		border: 1px solid var(--color-line-2);
		color: var(--color-accent-light);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-4);
		font-size: var(--text-table);
	}
	.ghost:disabled,
	.primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.error {
		align-self: center;
		font-size: var(--text-table);
		color: var(--color-severity-critical);
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: var(--space-4);
	}
	.stat {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	.label {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.value {
		font-size: 20px;
		font-weight: 500;
	}
	.absence-table {
		margin-top: var(--space-2);
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		overflow: hidden;
	}
	.absence-row {
		display: grid;
		grid-template-columns: 1.2fr 1fr 1fr 1fr;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-table);
		border-bottom: 1px solid var(--color-line-2);
	}
	.absence-row:last-child {
		border-bottom: none;
	}
	.absence-row.absence-head {
		color: var(--color-muted-2);
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
	}
	.absence-row.empty-row {
		display: block;
		color: var(--color-muted-2);
	}
	.status.recovered {
		color: var(--color-severity-healthy);
	}
	.chips {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-2);
		flex-wrap: wrap;
	}
	.chip {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-2);
	}
	.log-block {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-3);
		margin-top: var(--space-2);
		font-family: var(--font-mono);
		font-size: var(--text-log-row);
		line-height: var(--line-height-log);
	}
	.log-line {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.log-line.empty {
		color: var(--color-muted-2);
	}
	.rule-panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
	}
	.rule-name {
		font-size: 13.5px;
		font-weight: 500;
		margin-top: var(--space-2);
	}
	.rule-meta {
		display: flex;
		gap: var(--space-3);
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-1);
	}
	.enabled {
		text-transform: uppercase;
		font-size: var(--text-eyebrow);
		color: var(--color-severity-healthy);
	}
	.enabled.off {
		color: var(--color-muted-2);
	}
	.logql-block {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
		background: var(--color-surface-3);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		margin-top: var(--space-2);
		overflow-x: auto;
		white-space: nowrap;
	}
</style>

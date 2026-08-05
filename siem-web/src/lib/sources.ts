import type { SourceResponse } from './server/siemApiClient';

export function splitClaimedUnclaimed(sources: SourceResponse[]): {
	claimed: SourceResponse[];
	unclaimed: SourceResponse[];
} {
	return {
		claimed: sources.filter((s) => s.claimed),
		unclaimed: sources.filter((s) => !s.claimed)
	};
}

export function formatEventsPerMin(eventsPerMin: number): string {
	if (eventsPerMin < 1) return eventsPerMin.toFixed(1);
	return Math.round(eventsPerMin).toString();
}

export function formatLastSeen(lastSeenAt: string | undefined): string {
	if (!lastSeenAt) return 'never';
	const minutes = Math.floor((Date.now() - new Date(lastSeenAt).getTime()) / 60_000);
	if (minutes < 1) return 'just now';
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	return `${Math.floor(hours / 24)}d ago`;
}

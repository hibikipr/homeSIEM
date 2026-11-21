import { invalidate } from '$app/navigation';
import { SvelteMap } from 'svelte/reactivity';
import { DEP_ALERTS } from '#lib/invalidation.ts';
import type { AlertResponse } from '#lib/server/siemApiClient.ts';

// Client-side optimistic overlay for ack/mute. Components (AlertDetail,
// TriageCard) put the expected state here the moment the user clicks; the
// inbox list and Wall triage lane read it so every view reflects the action
// immediately. The entry is removed once the alerts load has re-run (success)
// or the request failed (rollback).
export const MUTE_DURATION_MS = 60 * 60 * 1000;

type Override = { state: 'acked' | 'muted'; muted_until?: string };
export const alertOverrides = new SvelteMap<number, Override>();

function mutedUntilIso(): string {
	return new Date(Date.now() + MUTE_DURATION_MS).toISOString();
}

export function applyOverride(alert: AlertResponse): AlertResponse {
	const o = alertOverrides.get(alert.id);
	return o ? { ...alert, ...o } : alert;
}

// Returns true when the server accepted the action. On any failure the
// optimistic state is rolled back and false is returned so the caller can
// show its error message.
export async function setAlertState(id: number, kind: 'ack' | 'mute'): Promise<boolean> {
	alertOverrides.set(
		id,
		kind === 'ack' ? { state: 'acked' } : { state: 'muted', muted_until: mutedUntilIso() }
	);
	try {
		const response = await fetch(`/api/alerts/${id}/${kind}`, { method: 'POST' });
		if (!response.ok) {
			alertOverrides.delete(id);
			return false;
		}
	} catch {
		alertOverrides.delete(id);
		return false;
	}
	try {
		await invalidate(DEP_ALERTS);
	} finally {
		alertOverrides.delete(id);
	}
	return true;
}

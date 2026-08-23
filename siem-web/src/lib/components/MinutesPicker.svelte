<script lang="ts">
	import { formatMinuteLabel } from '$lib/minutePresets';

	const CUSTOM = 'custom';

	let {
		seconds = $bindable(),
		presetsMinutes,
		label,
		description
	}: {
		// Bound to the parent's own state, always in seconds - siem-api's
		// rule fields are all *_sec, this component only changes what's
		// *displayed*, never the unit the value is actually stored/sent in.
		seconds: number;
		presetsMinutes: number[];
		label: string;
		description?: string;
	} = $props();

	// Initialized once from `seconds` (not $derived) - an existing rule's
	// value that doesn't match any preset (e.g. one set before this UI
	// existed, or via the API directly) still correctly falls through to
	// "Custom" and shows its real value on mount. But it has to be its own
	// state after that: selecting "Custom" alone doesn't change `seconds`
	// (nothing's been typed yet), so if this stayed purely derived from
	// `seconds`, it would immediately recompute back to whatever preset
	// `seconds` still equals, snapping the dropdown back before the input
	// could ever appear.
	let selectedPreset = $state(
		presetsMinutes.includes(seconds / 60) ? String(seconds / 60) : CUSTOM
	);

	function onPresetChange(event: Event) {
		const value = (event.target as HTMLSelectElement).value;
		selectedPreset = value;
		if (value !== CUSTOM) {
			seconds = Number(value) * 60;
		}
		// Selecting "Custom" alone doesn't change `seconds` - it just reveals
		// the input below, pre-filled with the current value, until the user
		// actually types something.
	}

	function onCustomMinutesInput(event: Event) {
		const minutes = Number((event.target as HTMLInputElement).value);
		if (!Number.isNaN(minutes) && minutes > 0) {
			seconds = minutes * 60;
		}
	}
</script>

<div class="picker">
	<label>
		{label}
		<select value={selectedPreset} onchange={onPresetChange}>
			{#each presetsMinutes as m (m)}
				<option value={String(m)}>{formatMinuteLabel(m)}</option>
			{/each}
			<option value={CUSTOM}>Custom…</option>
		</select>
	</label>
	{#if selectedPreset === CUSTOM}
		<label class="custom">
			Minutes
			<input type="number" min="1" step="1" value={seconds / 60} oninput={onCustomMinutesInput} />
		</label>
	{/if}
	{#if description}
		<p class="hint">{description}</p>
	{/if}
</div>

<style>
	.picker {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	label.custom {
		margin-top: var(--space-1);
	}
	select,
	input {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-2);
		font-size: var(--text-table);
		font-family: inherit;
	}
	.hint {
		margin: 0;
		font-size: 11px;
		color: var(--color-muted-2);
		line-height: 1.4;
	}
</style>

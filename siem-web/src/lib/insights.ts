import type { InsightEvidence } from './server/siemApiClient';

// Which program(s) an insight's evidence actually points at - the thing a
// reviewer needs first ("what do I go look at?") but that was previously
// only visible by expanding a row to reach the evidence table. Order of
// first appearance is preserved (evidence is already sorted by the model's
// own emphasis), duplicates collapsed, and an evidence item with no program
// label (possible for lines that never matched siem-ingest's program
// extraction) surfaces as "(unknown)" rather than silently vanishing - a
// gap in attribution is itself something worth seeing, not something to hide.
export function uniquePrograms(evidence: InsightEvidence[]): string[] {
	const seen = new Set<string>();
	const out: string[] = [];
	for (const ev of evidence) {
		const program = ev.program || '(unknown)';
		if (!seen.has(program)) {
			seen.add(program);
			out.push(program);
		}
	}
	return out;
}

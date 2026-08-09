import { describe, it, expect } from 'vitest';
import { RULE_TEMPLATES, parseGroupBy } from './ruleTemplates';

describe('RULE_TEMPLATES', () => {
	it('has five templates covering all three rule shapes', () => {
		expect(RULE_TEMPLATES).toHaveLength(5);
		const shapes = new Set(RULE_TEMPLATES.map((t) => t.shape));
		expect(shapes).toEqual(new Set(['threshold', 'absence', 'first_seen']));
	});

	it('every template has a non-empty label and name', () => {
		for (const template of RULE_TEMPLATES) {
			expect(template.label.length).toBeGreaterThan(0);
			expect(template.name.length).toBeGreaterThan(0);
		}
	});

	it('the absence template does not rely on logql', () => {
		const source_quiet = RULE_TEMPLATES.find((t) => t.shape === 'absence');
		expect(source_quiet?.logql).toBe('');
	});
});

describe('RULE_TEMPLATES backend-semantics invariants', () => {
	it.each(RULE_TEMPLATES)(
		'$name ($shape): logql/threshold/windowSec match what its evaluator reads',
		(template) => {
			if (template.shape !== 'absence') {
				expect(template.logql.length).toBeGreaterThan(0);
			}
			if (template.shape === 'threshold') {
				expect(template.threshold).toBeGreaterThanOrEqual(1);
			}
			expect(template.windowSec).toBeGreaterThanOrEqual(1);
		}
	);
});

describe('parseGroupBy', () => {
	it('splits a comma-separated list and trims whitespace', () => {
		expect(parseGroupBy('a, b,c')).toEqual(['a', 'b', 'c']);
	});

	it('returns an empty array for a blank string', () => {
		expect(parseGroupBy('')).toEqual([]);
		expect(parseGroupBy('   ')).toEqual([]);
	});

	it('filters out empty entries from trailing or doubled commas', () => {
		expect(parseGroupBy('a,,b,')).toEqual(['a', 'b']);
	});

	it('returns a single-element array for one field name', () => {
		expect(parseGroupBy('source')).toEqual(['source']);
	});
});

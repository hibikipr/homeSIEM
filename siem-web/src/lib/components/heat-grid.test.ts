import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { resolve } from 'path';

describe('HeatGrid.svelte CSS', () => {
	it('should have truncation CSS rules for .label', () => {
		// Read the component file
		const componentPath = resolve(__dirname, './HeatGrid.svelte');
		const content = readFileSync(componentPath, 'utf-8');

		// Extract the style block
		const styleMatch = content.match(/<style>([\s\S]*?)<\/style>/);
		expect(styleMatch).toBeDefined();
		const styleBlock = styleMatch![1];

		// Check that .label rule exists
		expect(styleBlock).toContain('.label {');

		// Check for truncation properties within .label block
		const labelRuleMatch = styleBlock.match(/\.label\s*\{([^}]+)\}/);
		expect(labelRuleMatch).toBeDefined();
		const labelRule = labelRuleMatch![1];

		// Verify all truncation properties are present
		expect(labelRule).toContain('white-space: nowrap;');
		expect(labelRule).toContain('overflow: hidden;');
		expect(labelRule).toContain('text-overflow: ellipsis;');

		// Also verify the existing properties are still there
		expect(labelRule).toContain('width: 96px;');
		expect(labelRule).toContain('font-family: var(--font-mono);');
		expect(labelRule).toContain('font-size: var(--text-label);');
		expect(labelRule).toContain('color: var(--color-muted);');
		expect(labelRule).toContain('flex-shrink: 0;');
	});

	it('should prevent label text from wrapping to multiple lines', () => {
		// This test verifies the CSS strategy: when white-space: nowrap is applied,
		// the browser will not break long text into multiple lines. Combined with
		// overflow: hidden and text-overflow: ellipsis, this ensures every .label
		// stays a single line tall, preventing row height variation that caused
		// the tooltip overlap bug.

		const componentPath = resolve(__dirname, './HeatGrid.svelte');
		const content = readFileSync(componentPath, 'utf-8');
		const styleMatch = content.match(/<style>([\s\S]*?)<\/style>/);
		const styleBlock = styleMatch![1];

		// The fix works by applying CSS that browsers interpret as:
		// 1. white-space: nowrap - prevents text from breaking to new lines
		// 2. overflow: hidden - hides content that doesn't fit in the 96px width
		// 3. text-overflow: ellipsis - adds "..." to indicate truncation
		//
		// This ensures:
		// - Every label row is exactly the same height (single line)
		// - Long source names don't grow the row unpredictably
		// - The tooltip at fixed top-right position doesn't overlap wrapped labels
		// - Full source name is still available via the row's aria-label on hover

		expect(styleBlock).toMatch(/\.label\s*\{[^}]*white-space:\s*nowrap;/);
		expect(styleBlock).toMatch(/\.label\s*\{[^}]*overflow:\s*hidden;/);
		expect(styleBlock).toMatch(/\.label\s*\{[^}]*text-overflow:\s*ellipsis;/);
	});
});

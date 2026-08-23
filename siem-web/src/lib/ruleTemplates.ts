import type { AlertSeverity } from './severity';

export type RuleShape = 'threshold' | 'absence' | 'first_seen' | 'insight';

export type RuleTemplate = {
	label: string;
	name: string;
	shape: RuleShape;
	logql: string;
	windowSec: number;
	threshold: number;
	groupBy: string;
	severity: AlertSeverity;
};

export const RULE_TEMPLATES: RuleTemplate[] = [
	{
		label: 'Repeated critical events from one source',
		name: 'critical-burst',
		shape: 'threshold',
		logql: '{job="siem", severity="critical"}',
		windowSec: 300,
		threshold: 5,
		groupBy: 'source',
		severity: 'critical'
	},
	{
		label: 'VPN connection',
		name: 'vpn-connect',
		shape: 'threshold',
		logql: '{job="siem"} |= "Connected to VPN"',
		windowSec: 60,
		threshold: 1,
		groupBy: '',
		severity: 'info'
	},
	{
		label: 'Admin accessed UniFi OS',
		name: 'admin-access',
		shape: 'threshold',
		logql: '{job="siem"} |= "Admin Accessed UniFi OS"',
		windowSec: 60,
		threshold: 1,
		groupBy: '',
		severity: 'warning'
	},
	{
		label: 'Source went quiet',
		name: 'source-quiet',
		shape: 'absence',
		logql: '',
		// This is the correlation window (see RuleForm's label for this
		// shape), not a detection window - AbsenceEvaluator decides
		// staleness from each source's own heartbeat, never this value.
		// 4 hours: a real production case had three unrelated sources go
		// quiet ~4 hours apart overnight, from what was almost certainly
		// one shared cause - narrow enough that two things going quiet
		// days apart still get their own separate alerts.
		windowSec: 4 * 60 * 60,
		threshold: 1,
		groupBy: '',
		severity: 'warning'
	},
	{
		label: 'New source seen',
		name: 'new-source',
		shape: 'first_seen',
		logql: '{job="siem"}',
		windowSec: 86400,
		threshold: 1,
		groupBy: 'source',
		severity: 'info'
	},
	{
		label: 'Insight found by Ollama',
		name: 'insight-alert',
		shape: 'insight',
		logql: '',
		windowSec: 60,
		threshold: 1,
		groupBy: '',
		// warning, not info: Insights already runs on its own 30-minute
		// cadence and covers plenty of low-stakes operational chatter (see
		// insights/prompt.go) - defaulting to "notify on literally
		// anything Ollama writes down" would make this rule noisy from the
		// moment it's turned on. Still fully editable like every other
		// template.
		severity: 'warning'
	}
];

export function parseGroupBy(input: string): string[] {
	return input
		.split(',')
		.map((s) => s.trim())
		.filter((s) => s.length > 0);
}

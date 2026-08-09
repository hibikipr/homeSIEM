import type { AlertSeverity } from './severity';

export type RuleShape = 'threshold' | 'absence' | 'first_seen';

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
		windowSec: 60,
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
	}
];

export function parseGroupBy(input: string): string[] {
	return input
		.split(',')
		.map((s) => s.trim())
		.filter((s) => s.length > 0);
}

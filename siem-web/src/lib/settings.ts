const ROLE_CAPABILITY_LABELS: Record<string, string> = {
	admin: 'read/write/manage',
	analyst: 'read/search/triage',
	viewer: 'read only'
};

export function roleCapabilityLabel(role: string): string {
	return ROLE_CAPABILITY_LABELS[role] ?? 'unknown';
}

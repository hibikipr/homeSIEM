import { API_URL } from '$app/env/private';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ locals, fetch }) => {
	const token = locals.sessionToken as string;
	const upstream = await fetch(`${API_URL}/alerts/stream`, {
		headers: { Authorization: `Bearer ${token}` }
	});

	return new Response(upstream.body, {
		status: upstream.status,
		headers: {
			'Content-Type': 'text/event-stream',
			'Cache-Control': 'no-cache'
		}
	});
};

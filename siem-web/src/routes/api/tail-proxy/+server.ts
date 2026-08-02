import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ locals, fetch }) => {
	const token = locals.sessionToken as string;

	const upstream = await fetch(`${env.API_URL}/events/tail`, {
		headers: { Authorization: `Bearer ${token}` }
	});

	return new Response(upstream.body, {
		status: upstream.status,
		headers: {
			'Content-Type': 'text/event-stream',
			'Cache-Control': 'no-cache',
			Connection: 'keep-alive'
		}
	});
};

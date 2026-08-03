import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = ({ locals, url }) => {
	return {
		user: locals.user,
		activeRoute: url.pathname
	};
};

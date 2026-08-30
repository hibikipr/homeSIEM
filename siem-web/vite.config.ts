import { defineConfig } from 'vitest/config';
import adapter from '@sveltejs/adapter-node';
import { sveltekit } from '@sveltejs/kit/vite';

export default defineConfig({
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},

			// Node adapter: siem-web deploys as a standalone Node server in Docker,
			// not to a platform adapter-auto could detect (Vercel/Netlify/Cloudflare/etc).
			adapter: adapter()
		})
	],
	test: {
		expect: { requireAssertions: true },
		projects: [
			{
				extends: './vite.config.ts',
				test: {
					name: 'server',
					environment: 'node',
					include: ['src/**/*.{test,spec}.{js,ts}'],
					exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
				}
			},
			{
				extends: './vite.config.ts',
				// Without this, Vite resolves svelte's "server" export
				// condition even under jsdom (Vitest transforms as SSR by
				// default), so @testing-library/svelte's mount() hits
				// svelte's server-only runtime and throws
				// lifecycle_function_unavailable instead of actually
				// mounting into the DOM.
				resolve: { conditions: ['browser'] },
				test: {
					name: 'client',
					// jsdom, not node: @testing-library/svelte mounts real
					// components, which need a DOM to render into.
					environment: 'jsdom',
					include: ['src/**/*.svelte.{test,spec}.{js,ts}']
				}
			}
		]
	}
});

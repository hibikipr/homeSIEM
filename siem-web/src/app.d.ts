// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	namespace App {
		// interface Error {}
		interface Locals {
			user?: { userId: number; email: string; displayName: string; groups: string[]; role: string };
			sessionToken?: string;
		}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

declare module '@phosphor-icons/web/regular';

export {};

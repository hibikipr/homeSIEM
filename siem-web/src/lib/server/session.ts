import { SignJWT, jwtVerify } from 'jose';

export interface SessionClaims {
	sub: string;
	userId: number;
	email: string;
	displayName: string;
	groups: string[];
	role: string;
	picture: string;
}

export const SESSION_COOKIE_NAME = 'siem_session';

export const SESSION_COOKIE_OPTIONS = {
	path: '/',
	httpOnly: true,
	secure: true,
	sameSite: 'lax' as const,
	maxAge: 60 * 60 * 12
};

export async function mintSessionToken(claims: SessionClaims, secret: Uint8Array): Promise<string> {
	return new SignJWT({
		user_id: claims.userId,
		email: claims.email,
		display_name: claims.displayName,
		groups: claims.groups,
		role: claims.role,
		picture: claims.picture
	})
		.setProtectedHeader({ alg: 'HS256' })
		.setSubject(claims.sub)
		.setIssuedAt()
		.setExpirationTime('12h')
		.sign(secret);
}

export async function verifySessionToken(
	token: string,
	secret: Uint8Array
): Promise<SessionClaims> {
	const { payload } = await jwtVerify(token, secret, { algorithms: ['HS256'] });

	if (typeof payload.sub !== 'string') {
		throw new Error('session token missing sub claim');
	}

	return {
		sub: payload.sub,
		userId: payload.user_id as number,
		email: payload.email as string,
		displayName: payload.display_name as string,
		groups: (payload.groups as string[]) ?? [],
		role: payload.role as string,
		picture: (payload.picture as string) ?? ''
	};
}

import { type SoniqApiClientOptions, requestJSON } from './http';
import { type User } from './users';

export type AuthUserResponse = {
  user: User;
};

export type SignInInput = {
  email: string;
  password: string;
};

export type SignUpInput = {
  email: string;
  display_name: string;
  password: string;
};

export async function signUp(input: SignUpInput, options: SoniqApiClientOptions = {}): Promise<AuthUserResponse> {
  return requestJSON<AuthUserResponse>('/auth/signup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  }, options);
}

export async function signIn(input: SignInInput, options: SoniqApiClientOptions = {}): Promise<AuthUserResponse> {
  return requestJSON<AuthUserResponse>('/auth/signin', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  }, options);
}

export async function signOut(options: SoniqApiClientOptions = {}): Promise<void> {
  await requestJSON<null>('/auth/signout', { method: 'POST' }, options);
}

import { type SoniqApiClientOptions, requestJSON } from './http';

export type User = {
  id: string;
  email: string;
  display_name: string;
  created_at: string;
  updated_at: string;
};

export async function getMe(options: SoniqApiClientOptions = {}): Promise<User> {
  return requestJSON<User>('/me', { method: 'GET' }, options);
}

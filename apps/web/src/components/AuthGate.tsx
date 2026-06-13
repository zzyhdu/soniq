import { type FormEvent, useState } from 'react';

import { type SignInInput, type SignUpInput } from '@soniq/api-client';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

export type AuthGateMode = 'signin' | 'signup';

export type AuthGateProps = {
  isSubmitting: boolean;
  error: string | null;
  onSignIn: (input: SignInInput) => Promise<unknown>;
  onSignUp: (input: SignUpInput) => Promise<unknown>;
};

export function AuthGate({ isSubmitting, error, onSignIn, onSignUp }: AuthGateProps) {
  const [mode, setMode] = useState<AuthGateMode>('signin');
  const [email, setEmail] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [password, setPassword] = useState('');

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (mode === 'signup') {
      await onSignUp({
        email,
        display_name: displayName,
        password,
      });
      return;
    }
    await onSignIn({ email, password });
  }

  return (
    <main className="bg-muted/30 flex min-h-screen items-center justify-center px-4 py-8">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>{mode === 'signup' ? 'Sign up' : 'Sign in'}</CardTitle>
          <CardDescription>Soniq</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={handleSubmit}>
            <div className="space-y-2">
              <Label htmlFor="auth-email">Email</Label>
              <Input
                id="auth-email"
                type="email"
                autoComplete="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                disabled={isSubmitting}
                required
              />
            </div>

            {mode === 'signup' && (
              <div className="space-y-2">
                <Label htmlFor="auth-display-name">Display name</Label>
                <Input
                  id="auth-display-name"
                  autoComplete="name"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                  disabled={isSubmitting}
                  required
                />
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="auth-password">Password</Label>
              <Input
                id="auth-password"
                type="password"
                autoComplete={mode === 'signup' ? 'new-password' : 'current-password'}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                disabled={isSubmitting}
                minLength={8}
                maxLength={1024}
                required
              />
            </div>

            {error !== null && (
              <p className="text-destructive text-sm" role="alert">
                {error}
              </p>
            )}

            <Button className="w-full" type="submit" disabled={isSubmitting}>
              {mode === 'signup' ? 'Sign up' : 'Sign in'}
            </Button>
            <Button
              className="w-full"
              type="button"
              variant="ghost"
              disabled={isSubmitting}
              onClick={() => setMode(mode === 'signup' ? 'signin' : 'signup')}
            >
              {mode === 'signup' ? 'Sign in' : 'Sign up'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}

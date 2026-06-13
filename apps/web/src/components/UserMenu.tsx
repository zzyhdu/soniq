import { type User } from '@soniq/api-client';
import { LogOut } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';

export type UserMenuProps = {
  user: User | undefined;
  isLoading: boolean;
  error: string | null;
  onLogout?: () => void;
  isLoggingOut?: boolean;
};

export function UserMenu({ user, isLoading, error, onLogout, isLoggingOut = false }: UserMenuProps) {
  if (isLoading) {
    return <span className="text-muted-foreground text-sm">Loading user...</span>;
  }

  if (error !== null) {
    return <span className="text-destructive text-sm" role="alert">{error}</span>;
  }

  if (user === undefined) {
    return null;
  }

  return (
    <div className="flex min-w-0 items-center gap-3 text-sm">
      <div className="min-w-0 text-right">
        <div className="truncate font-medium">{user.display_name}</div>
        <div className="text-muted-foreground truncate">{user.email}</div>
      </div>
      <Badge variant="outline">user</Badge>
      {onLogout !== undefined && (
        <Button variant="ghost" size="sm" onClick={onLogout} disabled={isLoggingOut}>
          <LogOut className="size-4" aria-hidden="true" />
          Sign out
        </Button>
      )}
    </div>
  );
}

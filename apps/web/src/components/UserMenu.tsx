import { type User } from '@soniq/api-client';

import { Badge } from '@/components/ui/badge';

export type UserMenuProps = {
  user: User | undefined;
  isLoading: boolean;
  error: string | null;
};

export function UserMenu({ user, isLoading, error }: UserMenuProps) {
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
      <Badge variant="outline">dev</Badge>
    </div>
  );
}

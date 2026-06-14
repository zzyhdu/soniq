import { type User } from '@soniq/api-client';
import { LogOut, UserCircle } from 'lucide-react';

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
    <div className="flex min-w-0 items-center gap-2 text-sm">
      <div className="hidden min-w-0 text-right xl:block">
        <div className="truncate text-[13px] font-medium text-[#191c1e]">{user.display_name}</div>
        <div className="truncate text-[11px] text-[#45474c]">{user.email}</div>
      </div>
      <UserCircle className="size-6 shrink-0 text-[#45474c]" aria-hidden="true" />
      {onLogout !== undefined && (
        <Button
          variant="ghost"
          size="sm"
          className="h-8 rounded px-2 text-[12px] text-[#45474c] hover:bg-[#f2f4f6] hover:text-[#191c1e]"
          onClick={onLogout}
          disabled={isLoggingOut}
        >
          <LogOut className="size-4" aria-hidden="true" />
          Sign out
        </Button>
      )}
    </div>
  );
}

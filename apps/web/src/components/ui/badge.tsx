import * as React from 'react';

import { cn } from '@/lib/utils';

export type BadgeProps = React.HTMLAttributes<HTMLDivElement> & {
  variant?: 'default' | 'secondary' | 'outline' | 'destructive';
};

const variantClasses = {
  default: 'border-transparent bg-primary text-primary-foreground',
  secondary: 'border-transparent bg-secondary text-secondary-foreground',
  outline: 'text-foreground',
  destructive: 'border-transparent bg-destructive text-destructive-foreground',
};

export function Badge({ className, variant = 'default', ...props }: BadgeProps) {
  return (
    <div
      className={cn(
        'focus:ring-ring inline-flex items-center rounded-md border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:ring-2 focus:ring-offset-2 focus:outline-none',
        variantClasses[variant],
        className,
      )}
      {...props}
    />
  );
}

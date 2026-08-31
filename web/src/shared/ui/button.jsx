import { Button as ButtonPrimitive } from '@base-ui/react/button';
import { cva } from 'class-variance-authority';

import { cn } from '../lib/utils.js';

const buttonVariants = cva('inline-flex items-center justify-center gap-[8px]', {
  variants: {
    variant: {
      default: '',
      secondary: 'secondary',
      destructive: 'danger',
    },
    size: {
      default: '',
      sm: 'small',
      icon: 'icon-button',
    },
  },
  defaultVariants: {
    variant: 'default',
    size: 'default',
  },
});

export function Button({ className, variant, size, ...props }) {
  return <ButtonPrimitive className={cn(buttonVariants({ variant, size }), className)} {...props} />;
}

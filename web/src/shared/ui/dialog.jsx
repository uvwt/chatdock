import { Dialog as DialogPrimitive } from '@base-ui/react/dialog';

import { cn } from '../lib/utils.js';

export function Dialog(props) {
  return <DialogPrimitive.Root {...props} />;
}

export function DialogPortal(props) {
  return <DialogPrimitive.Portal {...props} />;
}

export function DialogBackdrop({ className, ...props }) {
  return <DialogPrimitive.Backdrop className={cn(className)} {...props} />;
}

export function DialogViewport({ className, ...props }) {
  return (
    <DialogPrimitive.Viewport
      className={cn('fixed inset-[0] z-[1401] flex items-center justify-center p-[22px]', className)}
      {...props}
    />
  );
}

export function DialogPopup({ className, ...props }) {
  return <DialogPrimitive.Popup className={cn('outline-none', className)} {...props} />;
}

export function DialogTitle({ className, ...props }) {
  return <DialogPrimitive.Title className={cn('m-0', className)} {...props} />;
}

export function DialogDescription(props) {
  return <DialogPrimitive.Description {...props} />;
}

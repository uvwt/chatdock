import { Menu as MenuPrimitive } from '@base-ui/react/menu';

import { cn } from '../lib/utils.js';

export const Menu = MenuPrimitive.Root;
export const MenuTrigger = MenuPrimitive.Trigger;
export const MenuPortal = MenuPrimitive.Portal;

export function MenuPositioner({ className, ...props }) {
  return <MenuPrimitive.Positioner className={cn('z-[3000]', className)} {...props} />;
}

export function MenuPopup({ className, ...props }) {
  return <MenuPrimitive.Popup className={cn('outline-none', className)} {...props} />;
}

export function MenuItem({ className, danger = false, ...props }) {
  return (
    <MenuPrimitive.Item
      nativeButton
      render={<button type="button" />}
      className={cn(danger && 'danger', className)}
      {...props}
    />
  );
}

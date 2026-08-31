import { Tooltip as TooltipPrimitive } from '@base-ui/react/tooltip';

import { cn } from '../lib/utils.js';

export function Tooltip({ align = 'center', children, className, content, side = 'top', sideOffset = 6 }) {
  if (!content) return children;

  return (
    <TooltipPrimitive.Root disableHoverablePopup>
      <TooltipPrimitive.Trigger delay={450} closeDelay={100} render={children} />
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Positioner
          align={align}
          side={side}
          sideOffset={sideOffset}
          collisionPadding={8}
          className="pointer-events-none z-[3500]"
        >
          <TooltipPrimitive.Popup
            className={cn(
              'max-w-[240px] select-none rounded-[8px] border border-border bg-popover px-[8px] py-[5px] text-[11px] font-medium leading-[1.3] text-popover-foreground shadow-md',
              className,
            )}
          >
            {content}
          </TooltipPrimitive.Popup>
        </TooltipPrimitive.Positioner>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}

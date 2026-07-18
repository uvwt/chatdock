import React from 'react';

const iconPaths = {
  menu: ['M4 7h16', 'M4 12h16', 'M4 17h16'],
  sparkles: ['M12 3l1.35 3.65L17 8l-3.65 1.35L12 13l-1.35-3.65L7 8l3.65-1.35L12 3z', 'M18.5 13.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8.8-2.2z'],
  settings: ['M4 7h9', 'M17 7h3', 'M4 17h3', 'M11 17h9', 'M13 4v6', 'M7 14v6'],
  sun: ['M12 3v2', 'M12 19v2', 'M3 12h2', 'M19 12h2', 'M5.64 5.64l1.42 1.42', 'M16.94 16.94l1.42 1.42', 'M18.36 5.64l-1.42 1.42', 'M7.06 16.94l-1.42 1.42', 'M12 8.25a3.75 3.75 0 1 0 0 7.5 3.75 3.75 0 0 0 0-7.5z'],
  moon: ['M19.5 15.1A7.5 7.5 0 0 1 8.9 4.5 7.5 7.5 0 1 0 19.5 15.1z'],
  plus: ['M12 5v14', 'M5 12h14'],
  clock: ['M12 3.5a8.5 8.5 0 1 0 0 17 8.5 8.5 0 0 0 0-17z', 'M12 7.5V12l3 2'],
  tasks: ['M9 6h11', 'M9 12h11', 'M9 18h11', 'M4 6h.01', 'M4 12h.01', 'M4 18h.01'],
};

export function UiIcon({ name, className = '' }) {
  const paths = iconPaths[name];
  if (!paths) return null;
  return <svg className={['ui-icon', className].filter(Boolean).join(' ')} aria-hidden="true" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
    {paths.map((path, index) => <path key={index} d={path} />)}
  </svg>;
}

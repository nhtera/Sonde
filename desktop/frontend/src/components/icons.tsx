// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

// Icons of the design (stroke icons on a 24 or 16 grid).

import type { SVGProps } from "react";

type P = { size?: number } & SVGProps<SVGSVGElement>;

const svg = (size: number, box: number, props: SVGProps<SVGSVGElement>, children: React.ReactNode) => (
  <svg width={size} height={size} viewBox={`0 0 ${box} ${box}`} fill="none" stroke="currentColor" strokeWidth={box === 24 ? 1.7 : 1.6} strokeLinecap="round" strokeLinejoin="round" aria-hidden {...props}>
    {children}
  </svg>
);

export const FilesIcon = ({ size = 17, ...p }: P) =>
  svg(size, 24, p, <path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2h8.5A1.5 1.5 0 0 1 21 8.5v9a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5z" />);
export const HistoryIcon = ({ size = 17, ...p }: P) => svg(size, 24, p, <><circle cx="12" cy="12" r="8.5" /><path d="M12 7.5V12l3 2" /></>);
export const TestRunIcon = ({ size = 17, ...p }: P) => svg(size, 24, p, <path d="M4 7l2 2 3-3.5M4 16l2 2 3-3.5M13 8h7M13 17h7" />);
export const EnvIcon = ({ size = 17, ...p }: P) => svg(size, 24, p, <><path d="M12 3l9 4.5-9 4.5-9-4.5z" /><path d="M3 12l9 4.5 9-4.5M3 16.5L12 21l9-4.5" /></>);
export const ContractIcon = ({ size = 17, ...p }: P) => svg(size, 24, p, <><path d="M12 3l7.5 3v5.5c0 4.5-3.2 8-7.5 9.5-4.3-1.5-7.5-5-7.5-9.5V6z" /><path d="M9 12l2 2 4-4.5" /></>);
export const AgentsIcon = ({ size = 17, ...p }: P) => svg(size, 24, p, <><rect x="4.5" y="8" width="15" height="11" rx="3" /><path d="M12 4.5V8M9.5 13v1M14.5 13v1" /></>);
export const SettingsIcon = ({ size = 17, ...p }: P) => svg(size, 24, p, <><path d="M4 7h9M17 7h3M4 17h3M11 17h9" /><circle cx="15" cy="7" r="2" /><circle cx="9" cy="17" r="2" /></>);
export const MoonIcon = ({ size = 16, ...p }: P) => svg(size, 24, p, <path d="M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z" />);
export const SunIcon = ({ size = 16, ...p }: P) => svg(size, 24, p, <><circle cx="12" cy="12" r="4" /><path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M5.6 18.4L7 17M17 7l1.4-1.4" /></>);
export const SearchIcon = ({ size = 13, ...p }: P) => svg(size, 16, p, <><circle cx="7" cy="7" r="4.5" /><path d="M10.5 10.5L14 14" /></>);
export const PlusIcon = ({ size = 14, ...p }: P) => svg(size, 16, p, <path d="M8 3v10M3 8h10" />);
export const ImportIcon = ({ size = 14, ...p }: P) => svg(size, 16, p, <path d="M8 2.5v8M4.5 7L8 10.5 11.5 7M3 13.5h10" />);
export const DocIcon = ({ size = 16, ...p }: P) => svg(size, 16, p, <><rect x="2.5" y="2.5" width="11" height="11" rx="2" /><path d="M5.5 6h5M5.5 9h3" /></>);
export const BranchIcon = ({ size = 11, ...p }: P) => svg(size, 16, p, <><circle cx="4" cy="3.5" r="1.6" /><circle cx="4" cy="12.5" r="1.6" /><circle cx="12" cy="5.5" r="1.6" /><path d="M4 5v6M12 7c0 3-8 2-8 4" /></>);
export const ChevronDown = ({ size = 9, ...p }: P) => (
  <svg width={size} height={size} viewBox="0 0 10 10" fill="none" stroke="currentColor" strokeWidth={1.4} aria-hidden {...p}>
    <path d="M2.5 4l2.5 2.5L7.5 4" />
  </svg>
);
export const PlayIcon = ({ size = 10, ...p }: P) => (
  <svg width={size} height={size} viewBox="0 0 10 10" aria-hidden {...p}>
    <path d="M2.5 1.5l6 3.5-6 3.5z" fill="currentColor" />
  </svg>
);
export const LockIcon = ({ size = 12, ...p }: P) => svg(size, 16, p, <><rect x="3.5" y="7" width="9" height="6.5" rx="1.5" /><path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" /></>);
export const SondeMark = ({ size = 16 }: { size?: number }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="var(--accent)" strokeWidth={2} strokeLinecap="round" aria-hidden>
    <circle cx="6" cy="18" r="2" fill="var(--accent)" stroke="none" />
    <path d="M6 12a6 6 0 0 1 6 6" />
    <path d="M6 7a11 11 0 0 1 11 11" />
  </svg>
);
export const PinIcon = ({ size = 10, ...p }: P) => svg(size, 16, p, <path d="M10 2.5l3.5 3.5M11.75 4.25L9 7l-3-.5-1.5 1.5 4.5 4.5L10.5 11 10 8l2.75-2.75M6.75 9.25L3 13" />);
export const CloseIcon = ({ size = 10, ...p }: P) => svg(size, 16, p, <path d="M4 4l8 8M12 4l-8 8" />);
export const CopyIcon = ({ size = 13, ...p }: P) =>
  svg(size, 16, { strokeWidth: 1.5, ...p }, <><rect x="5" y="5" width="8.5" height="8.5" rx="1.5" /><path d="M11 5V3.5A1.5 1.5 0 0 0 9.5 2h-6A1.5 1.5 0 0 0 2 3.5v6A1.5 1.5 0 0 0 3.5 11H5" /></>);
export const RunToIcon = ({ size = 11, ...p }: P) => svg(size, 12, { strokeWidth: 1.5, ...p }, <path d="M2 6h6M6 3l3 3-3 3M10.5 2v8" />);
export const DownloadIcon = ({ size = 12, ...p }: P) => svg(size, 16, { strokeWidth: 1.5, ...p }, <path d="M8 2.5v8M4.5 7L8 10.5 11.5 7M3 13.5h10" />);

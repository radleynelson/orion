import { useId } from 'react';

interface Props {
  size?: number;
  className?: string;
}

export default function OrionMark({ size = 28, className = '' }: Props) {
  const gradientId = useId();

  return (
    <span className={`orion-mark${className ? ` ${className}` : ''}`} style={{ width: size, height: size }} aria-label="Orion">
      <svg width={size} height={size} viewBox="0 0 44 44" aria-hidden="true">
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stopColor="#2B2D32" />
            <stop offset="100%" stopColor="#1A1A1C" />
          </linearGradient>
        </defs>
        <rect width="44" height="44" rx="13" fill={`url(#${gradientId})`} />
        <rect x="0.5" y="0.5" width="43" height="43" rx="12.5" fill="none" stroke="rgba(255,255,255,0.08)" />
        <path d="M10 30 Q22 14 34 18" stroke="rgba(124,169,247,0.32)" strokeWidth="0.9" fill="none" strokeLinecap="round" />
        <circle cx="14" cy="26" r="2.4" fill="#7CA9F7" />
        <circle cx="22" cy="22" r="3" fill="#EAEAEC" />
        <circle cx="30" cy="20" r="2.4" fill="#7CA9F7" />
      </svg>
    </span>
  );
}

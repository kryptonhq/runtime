// Inline ⌬ glyph logo, matching the favicon. Sized for the sidebar header.
export function Logo({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 32 32"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-label="Krypton"
    >
      <circle cx="16" cy="16" r="14" fill="#6366f1" />
      <text
        x="16"
        y="22"
        fontFamily="ui-monospace, monospace"
        fontSize="20"
        fontWeight="700"
        textAnchor="middle"
        fill="#eef2ff"
      >
        ⌬
      </text>
    </svg>
  );
}

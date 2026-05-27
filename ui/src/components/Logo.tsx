// Krypton mark — sidebar header chip.
// SVG is imported as a URL (Vite handles this natively) and rendered
// via <img>, so we don't carry the markup inline. Single source of
// truth lives at ui/src/assets/logo.svg, mirroring the favicon.
import logoUrl from "../assets/logo.svg";

export function Logo({ className }: { className?: string }) {
  return <img src={logoUrl} alt="Krypton" className={className} />;
}

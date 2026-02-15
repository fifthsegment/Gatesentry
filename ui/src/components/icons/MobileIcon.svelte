<script lang="ts">
  export let size: number = 128;
</script>

<svg
  xmlns="http://www.w3.org/2000/svg"
  viewBox="0 0 128 128"
  width={size}
  height={size}
  aria-hidden="true"
>
  <!-- Phone body -->
  <rect x="32" y="8" width="64" height="112" rx="12" ry="12" fill="#37474F" />
  <!-- Phone inner body -->
  <rect x="34" y="10" width="60" height="108" rx="11" ry="11" fill="#263238" />
  <!-- Screen -->
  <rect x="38" y="24" width="52" height="80" rx="4" ry="4" fill="#0f62fe" />
  <!-- Screen gradient -->
  <rect
    x="38"
    y="24"
    width="52"
    height="80"
    rx="4"
    ry="4"
    fill="url(#screenGradMobile)"
  />
  <!-- Status bar area -->
  <rect
    x="38"
    y="24"
    width="52"
    height="12"
    rx="4"
    ry="4"
    fill="rgba(0,0,0,0.15)"
  />
  <!-- Status bar icons -->
  <rect
    x="72"
    y="28"
    width="6"
    height="3"
    rx="1"
    fill="rgba(255,255,255,0.6)"
  />
  <rect
    x="80"
    y="28"
    width="6"
    height="3"
    rx="1"
    fill="rgba(255,255,255,0.6)"
  />
  <!-- App icons row 1 -->
  <rect x="44" y="44" width="12" height="12" rx="3" fill="#42be65" />
  <rect x="60" y="44" width="12" height="12" rx="3" fill="#ff832b" />
  <rect x="76" y="44" width="12" height="12" rx="3" fill="#d4bbff" />
  <!-- App icons row 2 -->
  <rect x="44" y="62" width="12" height="12" rx="3" fill="#08bdba" />
  <rect x="60" y="62" width="12" height="12" rx="3" fill="#ee5396" />
  <rect x="76" y="62" width="12" height="12" rx="3" fill="#4589ff" />
  <!-- App icons row 3 -->
  <rect x="44" y="80" width="12" height="12" rx="3" fill="#f1c21b" />
  <rect x="60" y="80" width="12" height="12" rx="3" fill="#a56eff" />
  <rect x="76" y="80" width="12" height="12" rx="3" fill="#24a148" />
  <!-- Screen shine -->
  <rect
    x="38"
    y="24"
    width="52"
    height="24"
    rx="4"
    ry="4"
    fill="rgba(255,255,255,0.06)"
  />
  <!-- Notch / speaker -->
  <rect x="52" y="14" width="24" height="4" rx="2" fill="#37474F" />
  <!-- Front camera -->
  <circle cx="70" cy="16" r="2" fill="#455A64" />
  <circle cx="70" cy="16" r="1" fill="#1a1a2e" />
  <!-- Home indicator bar -->
  <rect x="50" y="108" width="28" height="4" rx="2" fill="#455A64" />
  <defs>
    <linearGradient id="screenGradMobile" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#0f62fe" />
      <stop offset="50%" stop-color="#4589ff" />
      <stop offset="100%" stop-color="#0043ce" />
    </linearGradient>
  </defs>
</svg>

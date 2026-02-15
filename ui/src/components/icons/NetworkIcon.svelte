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
  <!-- Central hub / router body -->
  <rect x="28" y="44" width="72" height="40" rx="6" ry="6" fill="#37474F" />
  <!-- Router front panel -->
  <rect x="32" y="48" width="64" height="32" rx="3" ry="3" fill="#263238" />
  <!-- Status LEDs -->
  <circle cx="44" cy="58" r="3" fill="#42be65" />
  <circle cx="56" cy="58" r="3" fill="#42be65" />
  <circle cx="68" cy="58" r="3" fill="#0f62fe" />
  <circle cx="80" cy="58" r="3" fill="#0f62fe" />
  <!-- LED glow -->
  <circle cx="44" cy="58" r="5" fill="#42be65" opacity="0.2" />
  <circle cx="56" cy="58" r="5" fill="#42be65" opacity="0.2" />
  <circle cx="68" cy="58" r="5" fill="#0f62fe" opacity="0.15" />
  <circle cx="80" cy="58" r="5" fill="#0f62fe" opacity="0.15" />
  <!-- Ventilation lines -->
  <rect x="40" y="68" width="48" height="2" rx="1" fill="#37474F" />
  <rect x="40" y="72" width="48" height="2" rx="1" fill="#37474F" />

  <!-- Top connection lines (to devices) -->
  <!-- Left line -->
  <line
    x1="42"
    y1="44"
    x2="28"
    y2="20"
    stroke="#546E7A"
    stroke-width="2.5"
    stroke-linecap="round"
  />
  <!-- Center line -->
  <line
    x1="64"
    y1="44"
    x2="64"
    y2="16"
    stroke="#546E7A"
    stroke-width="2.5"
    stroke-linecap="round"
  />
  <!-- Right line -->
  <line
    x1="86"
    y1="44"
    x2="100"
    y2="20"
    stroke="#546E7A"
    stroke-width="2.5"
    stroke-linecap="round"
  />

  <!-- Top device nodes -->
  <circle cx="28" cy="17" r="7" fill="#455A64" />
  <circle cx="28" cy="17" r="4" fill="#0f62fe" />
  <circle cx="64" cy="13" r="7" fill="#455A64" />
  <circle cx="64" cy="13" r="4" fill="#0f62fe" />
  <circle cx="100" cy="17" r="7" fill="#455A64" />
  <circle cx="100" cy="17" r="4" fill="#0f62fe" />

  <!-- Bottom connection lines (to internet/globe) -->
  <line
    x1="64"
    y1="84"
    x2="64"
    y2="100"
    stroke="#546E7A"
    stroke-width="2.5"
    stroke-linecap="round"
  />

  <!-- Globe / internet node -->
  <circle cx="64" cy="108" r="10" fill="#455A64" />
  <circle cx="64" cy="108" r="7" fill="url(#globeGradNet)" />
  <!-- Globe lines -->
  <ellipse
    cx="64"
    cy="108"
    rx="7"
    ry="3.5"
    fill="none"
    stroke="rgba(255,255,255,0.3)"
    stroke-width="0.8"
  />
  <line
    x1="64"
    y1="101"
    x2="64"
    y2="115"
    stroke="rgba(255,255,255,0.3)"
    stroke-width="0.8"
  />
  <line
    x1="57"
    y1="108"
    x2="71"
    y2="108"
    stroke="rgba(255,255,255,0.3)"
    stroke-width="0.8"
  />

  <!-- Shield overlay on router (protection symbol) -->
  <path
    d="M58 50 L64 47 L70 50 L70 57 C70 60 67 63 64 64 C61 63 58 60 58 57 Z"
    fill="none"
    stroke="rgba(255,255,255,0.25)"
    stroke-width="1.2"
  />
  <path
    d="M62 55 L64 57 L68 52"
    fill="none"
    stroke="rgba(255,255,255,0.35)"
    stroke-width="1.2"
    stroke-linecap="round"
    stroke-linejoin="round"
  />

  <defs>
    <linearGradient id="globeGradNet" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#0f62fe" />
      <stop offset="100%" stop-color="#0043ce" />
    </linearGradient>
  </defs>
</svg>

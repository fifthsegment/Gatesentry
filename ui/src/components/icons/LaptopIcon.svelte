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
  <!-- Laptop screen body -->
  <rect x="20" y="16" width="88" height="64" rx="6" ry="6" fill="#37474F" />
  <!-- Screen bezel -->
  <rect x="26" y="22" width="76" height="52" rx="2" ry="2" fill="#0f62fe" />
  <!-- Screen gradient overlay -->
  <rect
    x="26"
    y="22"
    width="76"
    height="52"
    rx="2"
    ry="2"
    fill="url(#screenGradLaptop)"
  />
  <!-- Screen content lines -->
  <rect
    x="34"
    y="34"
    width="40"
    height="3"
    rx="1.5"
    fill="rgba(255,255,255,0.7)"
  />
  <rect
    x="34"
    y="41"
    width="52"
    height="3"
    rx="1.5"
    fill="rgba(255,255,255,0.5)"
  />
  <rect
    x="34"
    y="48"
    width="36"
    height="3"
    rx="1.5"
    fill="rgba(255,255,255,0.4)"
  />
  <rect
    x="34"
    y="55"
    width="44"
    height="3"
    rx="1.5"
    fill="rgba(255,255,255,0.3)"
  />
  <!-- Screen shine -->
  <rect
    x="26"
    y="22"
    width="76"
    height="16"
    rx="2"
    ry="2"
    fill="rgba(255,255,255,0.08)"
  />
  <!-- Camera dot -->
  <circle cx="64" cy="19" r="1.5" fill="#546E7A" />
  <!-- Laptop base -->
  <path
    d="M10 82 L20 80 L108 80 L118 82 L118 88 C118 92 114 96 110 96 L18 96 C14 96 10 92 10 88 Z"
    fill="#455A64"
  />
  <!-- Base top surface -->
  <rect x="20" y="80" width="88" height="4" rx="1" fill="#546E7A" />
  <!-- Trackpad -->
  <rect x="50" y="85" width="28" height="6" rx="2" fill="#37474F" />
  <!-- Power indicator -->
  <circle cx="64" cy="77" r="1" fill="#42be65" />
  <defs>
    <linearGradient id="screenGradLaptop" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#0f62fe" />
      <stop offset="50%" stop-color="#4589ff" />
      <stop offset="100%" stop-color="#0043ce" />
    </linearGradient>
  </defs>
</svg>

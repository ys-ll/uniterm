import { ref, computed, watch, type Ref } from 'vue'
import type { ConnectionConfig } from '../../types/session'

const RDP_ADAPTIVE_RESOLUTION = 'adaptive'
export const RDP_CUSTOM_RESOLUTION = 'custom'

// Default preset list (mstsc-style), plus a "custom" entry that reveals
// free width/height inputs — the RDP protocol accepts any desktop size, so
// presets cannot cover every monitor (e.g. 1360×768).
const rdpResolutions = [
  { label: '800 × 600 (SVGA)', w: 800, h: 600 },
  { label: '1024 × 768 (XGA)', w: 1024, h: 768 },
  { label: '1280 × 720 (HD)', w: 1280, h: 720 },
  { label: '1280 × 800 (WXGA)', w: 1280, h: 800 },
  { label: '1280 × 1024 (SXGA)', w: 1280, h: 1024 },
  { label: '1366 × 768 (HD)', w: 1366, h: 768 },
  { label: '1440 × 900 (WXGA+)', w: 1440, h: 900 },
  { label: '1600 × 900 (HD+)', w: 1600, h: 900 },
  { label: '1600 × 1200 (UXGA)', w: 1600, h: 1200 },
  { label: '1680 × 1050 (WSXGA+)', w: 1680, h: 1050 },
  { label: '1920 × 1080 (Full HD)', w: 1920, h: 1080 },
  { label: '1920 × 1200 (WUXGA)', w: 1920, h: 1200 },
  { label: '2560 × 1440 (QHD)', w: 2560, h: 1440 },
  { label: '2560 × 1600 (WQXGA)', w: 2560, h: 1600 },
  { label: '3840 × 2160 (4K UHD)', w: 3840, h: 2160 },
]

function rdpResolutionKey(w?: number, h?: number): string {
  if (w === undefined || h === undefined || w === -1 || h === -1) return 'fullscreen'
  const match = rdpResolutions.find(r => r.w === w && r.h === h)
  return match ? `${match.w}x${match.h}` : RDP_CUSTOM_RESOLUTION
}

export interface UseRdpResolutionReturn {
  rdpResolution: Ref<string>
  rdpCustomWidth: Ref<number>
  rdpCustomHeight: Ref<number>
  rdpResolutionOptions: Ref<{ value: string; label: string }[]>
  /** Sync the picker state from a hydrated edit config. */
  syncFromConfig: (config: ConnectionConfig) => void
}

/**
 * RDP desktop-size picker state, kept in sync with `form.rdpFixedWidth/Height`.
 * Extracted from ConnectionForm so the parent only wires the picker to the
 * BasicFields subcomponent.
 */
export function useRdpResolution(
  form: ConnectionConfig,
  t: (key: string) => string
): UseRdpResolutionReturn {
  // computed (not a plain const) so the labels re-evaluate when the UI locale
  // changes; a const would freeze the language from component-mount time.
  const rdpResolutionOptions = computed(() => [
    { value: 'adaptive', label: t('rdp.adaptive') },
    { value: 'fullscreen', label: t('rdp.fullscreen') },
    ...rdpResolutions.map(r => ({ value: `${r.w}x${r.h}`, label: r.label })),
    { value: RDP_CUSTOM_RESOLUTION, label: t('rdp.customResolution') },
  ])

  const rdpResolution = ref('fullscreen')
  const rdpCustomWidth = ref(1360)
  const rdpCustomHeight = ref(768)

  // Keep the form's fixed size fields in sync with the resolution picker.
  watch(rdpResolution, (val) => {
    if (val === RDP_ADAPTIVE_RESOLUTION || val === 'fullscreen') {
      // Adaptive (-2) seeds the desktop from the window size (backend),
      // fullscreen (-1) follows the primary monitor — both are sentinels.
      const sentinel = val === RDP_ADAPTIVE_RESOLUTION ? -2 : -1
      form.rdpFixedWidth = sentinel
      form.rdpFixedHeight = sentinel
    } else if (val === RDP_CUSTOM_RESOLUTION) {
      form.rdpFixedWidth = rdpCustomWidth.value
      form.rdpFixedHeight = rdpCustomHeight.value
    } else {
      const [w, h] = val.split('x').map(Number)
      form.rdpFixedWidth = w
      form.rdpFixedHeight = h
    }
  })

  // A change to the custom width/height inputs applies immediately while
  // "custom" is selected.
  watch([rdpCustomWidth, rdpCustomHeight], ([w, h]) => {
    if (rdpResolution.value === RDP_CUSTOM_RESOLUTION && w && h) {
      form.rdpFixedWidth = w
      form.rdpFixedHeight = h
    }
  })

  function syncFromConfig(config: ConnectionConfig) {
    // Sync resolution picker to the config's fixed size. Sizes outside the
    // preset list fall back to the custom inputs so they stay visible and
    // editable.
    rdpResolution.value =
      config.rdpFixedWidth === -2
        ? RDP_ADAPTIVE_RESOLUTION
        : config.rdpFixedWidth === -1
          ? 'fullscreen'
          : rdpResolutionKey(config.rdpFixedWidth, config.rdpFixedHeight)
    if (rdpResolution.value === RDP_CUSTOM_RESOLUTION) {
      rdpCustomWidth.value = config.rdpFixedWidth ?? 1360
      rdpCustomHeight.value = config.rdpFixedHeight ?? 768
    }
  }

  return { rdpResolution, rdpCustomWidth, rdpCustomHeight, rdpResolutionOptions, syncFromConfig }
}
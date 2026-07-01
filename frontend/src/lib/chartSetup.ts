/**
 * lib/chartSetup.ts
 *
 * Registers ONLY the Chart.js components required by TelemetryPulse.
 * Tree-shakeable — we do not import the full Chart.js bundle.
 *
 * Import this module once (in App.tsx) before any Chart instance is created.
 */
import {
  Chart,
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  CategoryScale,
  Filler,
  Tooltip,
  Legend,
} from 'chart.js'

Chart.register(
  LineController,
  LineElement,
  PointElement,
  LinearScale,
  CategoryScale,
  Filler,
  Tooltip,
  Legend,
)

/** Apply global Chart.js defaults matching TelemetryPulse's dark theme. */
Chart.defaults.color = '#3d5166'
Chart.defaults.font.family = "'JetBrains Mono', 'Fira Code', monospace"
Chart.defaults.font.size = 11

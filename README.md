# TelemetryPulse

![Build Status](https://img.shields.io/badge/build-passing-brightgreen?style=flat-square)
![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go)
![React](https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react)
![TypeScript](https://img.shields.io/badge/TypeScript-5.0-3178C6?style=flat-square&logo=typescript)
![Redis](https://img.shields.io/badge/Redis-PubSub-DC382D?style=flat-square&logo=redis)

## 🚀 Elevator Pitch

TelemetryPulse is an industrial-grade, real-time Synthetic Network Performance Monitoring & Observability Engine. Designed to ingest, calculate, and visualize network latencies at high velocities, it solves the problem of detecting sub-second micro-anomalies in distributed systems without overwhelming the browser DOM or backing data stores.

## ✨ Key Features

* **O(1) Anomaly Detection:** Implements a sliding window statistical algorithm (Z-Score analysis) that computes mean and standard deviation over an N-sized rolling window in constant time.
* **Demand-Driven Architecture:** The backend actively monitors WebSocket connection counts, pausing all network probes and Redis `PUBLISH` events when the dashboard is idle, yielding exact zero-cost overhead when not actively observed.
* **Main-Thread Decoupled UI:** Frontend ingestion uses a `requestAnimationFrame` loop combined with a mutable delta queue. This entirely decouples network ingestion velocity (e.g., 500ms intervals) from React's render lifecycle, maintaining a smooth 60 FPS without main-thread thrashing.
* **Canvas-Based Rendering:** Bypasses SVG overhead entirely. All telemetry is imperatively drawn to an HTML5 Canvas via Chart.js, avoiding costly DOM manipulations per data point.
* **Minimalist UI/UX:** An unapologetic, uncompromising "all-black and chrome" industrial aesthetic built entirely with Tailwind CSS utility classes.

## 🏗 System Architecture

The pipeline is highly decoupled and scales horizontally across endpoints:

1. **Synthetic Probes (Go):** Independent Goroutines simulate network traffic (ICMP/TCP latency distributions with log-normal baselines).
2. **Statistical Engine (Go):** Computes live sliding-window math to detect Z-Score anomalies (spikes, packet loss).
3. **Message Broker (Redis):** Employs a strict Pub/Sub architecture to fan out telemetry payloads rapidly.
4. **WebSocket Hub (Go):** Subscribes to Redis and synchronizes state into a single aggregated snapshot frame broadcasted to all connected clients.
5. **Observability UI (React):** Connects to the WebSocket, queues incoming JSON packets, and renders the data onto a virtualized log and Canvas chart at up to 60 FPS.

## 🛠 Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes.

### Prerequisites

You must have the following installed on your machine:
* **Go** (v1.21+)
* **Node.js** (v18+)
* **Redis** (running locally on port `6379`)

### Installation & Local Development

**1. Clone the repository:**
```bash
git clone https://github.com/yourusername/TelemetryPulse.git
cd TelemetryPulse
```

**2. Start the Backend:**
```bash
cd backend
go mod tidy
# The backend defaults to localhost:6379 for Redis and port 8080 for the API
go run cmd/telemetrypulse/main.go
```

**3. Start the Frontend:**
Open a new terminal window:
```bash
cd frontend
# Create the local environment file
cp .env.example .env.local

npm install
npm run dev
```

**4. View the Dashboard:**
Open your browser and navigate to `http://localhost:5173`. The backend will instantly detect the connection, resume network probing, and begin streaming live telemetry to the dashboard.

## ⚡ Performance Metrics

TelemetryPulse is rigorously optimized for real-time observability:
* **Ingestion Velocity:** Easily handles 2Hz (500ms) multi-endpoint payload broadcasts.
* **Render Frame Rate:** Bound safely to 60 FPS through `requestAnimationFrame` debouncing, even during high-density anomaly bursts.
* **Resource Optimization:** Redis quotas and CPU cycles are strictly preserved through the automatic demand-driven suspension architecture.

## ☁️ Deployment

TelemetryPulse is 12-factor app compliant and fully container-ready, making it trivial to deploy on modern cloud platforms:
* **Backend:** Deployable on Render or Railway using the `PORT` and `REDIS_URL` environment variables.
* **Frontend:** Deployable as a static Vite site on Vercel or Netlify via `VITE_WS_URL` and `VITE_API_URL` configuration.
* **Datastore:** Seamlessly connects to managed Redis instances like Upstash.

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

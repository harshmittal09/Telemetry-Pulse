/**
 * hooks/useTelemetryStream.ts
 *
 * PHASE 3 — Memory-Safe, Thread-Decoupled Telemetry Ingestion
 * ============================================================
 *
 * Architecture
 * ────────────
 *
 *  WebSocket (500 ms network velocity)
 *       │
 *       │  onmessage callback
 *       ▼
 *  deltaQueueRef (useRef<WSMessage[]>)   ← MUTABLE, lives OUTSIDE React
 *       │                                  No re-renders triggered here.
 *       │  requestAnimationFrame loop    ← Browser schedules at ≤60 FPS
 *       ▼                                  (≈16.67 ms/frame)
 *  useState({ endpoints, log })          ← React state, drives DOM
 *
 * Why this design?
 * ─────────────────
 * Problem: The WebSocket fires at 500 ms. If we called setState() inside
 * onmessage, React would re-render 2× per second — fine for a single
 * endpoint, but with 60 FPS animations and a growing virtualized log the
 * main thread would thrash. Bursts (multiple messages in quick succession
 * after reconnect) would cause cascading re-renders.
 *
 * Solution: The onmessage callback writes into a mutable array reference
 * (deltaQueueRef) — this is a plain JS mutation, zero React involvement.
 * A separate requestAnimationFrame loop runs at up to 60 FPS, drains the
 * queue in a single O(n) sweep, and calls setState() ONCE per frame if
 * any new data arrived. This decouples:
 *   • Network velocity (500 ms) — unbounded, controlled by backend.
 *   • UI render velocity (≤60 FPS) — controlled by the browser scheduler.
 *
 * The rAF loop is aligned with vsync, so layout/paint never races with
 * state updates — the browser decides the optimal moment to flush.
 *
 * Key invariants:
 *   1. deltaQueueRef is NEVER read by React (no dependency array).
 *   2. setState() is called AT MOST once per animation frame.
 *   3. WebSocket reconnects with exponential back-off (max 30 s).
 *   4. All timers and the WebSocket are cleaned up on unmount.
 *   5. Max log length is capped at MAX_LOG_ENTRIES to bound memory.
 */

import { useEffect, useRef, useState, useCallback } from 'react';
import type {
  WSMessage,
  TelemetryPayload,
  EndpointStateMap,
  LogEntry,
  ConnectionStatus,
  SystemMetrics,
  RDSMetrics,
} from '../types/telemetry';

// ─── Constants ──────────────────────────────────────────────────────────────

/** Maximum number of log entries retained in memory. */
const MAX_LOG_ENTRIES = 2000;

/** WebSocket URL — Vite proxy rewrites /ws → ws://localhost:8080/ws. */
const WS_URL = import.meta.env.VITE_WS_URL || `ws://${window.location.host}/ws`;

/** Exponential back-off: initial delay, multiplier, cap. */
const RECONNECT_BASE_MS  = 500;
const RECONNECT_MAX_MS   = 30_000;
const RECONNECT_EXPONENT = 1.8;

// ─── Types ───────────────────────────────────────────────────────────────────

export interface TelemetryStreamState {
  /** Latest per-endpoint snapshot, keyed by endpoint_id. */
  endpoints: EndpointStateMap;
  /** Append-only event log (capped at MAX_LOG_ENTRIES). */
  log: LogEntry[];
  /** Current WebSocket lifecycle state. */
  status: ConnectionStatus;
  /** Total frames received since mount (useful for debug overlays). */
  frameCount: number;
  /** Latest Go runtime / system memory metrics. Null until first frame arrives. */
  systemMetrics: SystemMetrics | null;
  /** Latest AWS RDS backup metrics. Null when RDS probe is not configured. */
  rdsMetrics: RDSMetrics | null;
}

export interface TelemetryStreamControls {
  /** Manually close and reconnect the WebSocket. */
  reconnect: () => void;
}

// ─── Hook ────────────────────────────────────────────────────────────────────

/**
 * useTelemetryStream opens a WebSocket connection to the backend and returns
 * live telemetry state updated at ≤60 FPS via a requestAnimationFrame loop.
 *
 * The hook is self-contained: it manages the WebSocket lifecycle, exponential
 * back-off reconnection, the mutable delta queue, and the rAF render loop.
 */
export function useTelemetryStream(): TelemetryStreamState & TelemetryStreamControls {

  // ── React state — these drive the DOM ─────────────────────────────────────
  const [state, setState] = useState<TelemetryStreamState>({
    endpoints:     new Map<string, TelemetryPayload>(),
    log:           [],
    status:        'connecting',
    frameCount:    0,
    systemMetrics: null,
    rdsMetrics:    null,
  });

  // ── Mutable refs — these live OUTSIDE React's rendering cycle ─────────────

  /**
   * deltaQueueRef is the in-memory delta queue.
   *
   * CRITICAL: This is a plain mutable JS array, NOT React state.
   * The onmessage callback pushes into it without triggering any re-render.
   * The rAF loop drains it and calls setState() once per frame if non-empty.
   */
  const deltaQueueRef = useRef<WSMessage[]>([]);

  /** Monotonically increasing log sequence counter (not part of state). */
  const seqRef = useRef<number>(0);

  /** Reference to the active WebSocket instance. */
  const wsRef = useRef<WebSocket | null>(null);

  /** rAF handle — stored so we can cancel on unmount. */
  const rafHandleRef = useRef<number>(0);

  /** Back-off attempt counter. Reset to 0 on successful connection. */
  const reconnectAttemptsRef = useRef<number>(0);

  /** Reconnect timeout handle. */
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  /** Whether the hook has been unmounted (prevents setState after unmount). */
  const unmountedRef = useRef<boolean>(false);

  // ── requestAnimationFrame render loop ──────────────────────────────────────

  /**
   * startRafLoop begins the vsync-aligned poll loop.
   *
   * Each frame:
   *  1. Atomically drain deltaQueueRef (splice the array to avoid race with
   *     concurrent pushes from onmessage — JS is single-threaded so this is
   *     safe, but we still do it atomically for clarity).
   *  2. If the queue was empty, skip setState (no-op frame).
   *  3. Otherwise, merge all new payloads into the endpoint map and prepend
   *     new log entries, then call setState() ONCE.
   */
  const startRafLoop = useCallback(() => {
    const loop = () => {
      if (unmountedRef.current) return;

      // Atomically drain the delta queue.
      const batch = deltaQueueRef.current.splice(0);

      if (batch.length > 0) {
        setState(prev => {
          // Clone the Map so React can detect the reference change.
          const nextEndpoints = new Map(prev.endpoints);
          const newLogEntries: LogEntry[] = [];
          // Track the latest system/rds snapshots from this batch.
          let nextSystem = prev.systemMetrics;
          let nextRds    = prev.rdsMetrics;

          for (const msg of batch) {
            for (const payload of msg.payloads) {
              // Update latest snapshot for this endpoint.
              nextEndpoints.set(payload.endpoint_id, payload);
              // Create a log entry with a stable, monotonically increasing key.
              newLogEntries.push({ seq: seqRef.current++, payload });
            }
            // Hoist system/rds from the latest frame that carries them.
            if (msg.system) nextSystem = msg.system;
            if (msg.rds)    nextRds    = msg.rds;
          }

          // Prepend new entries and cap total length.
          const nextLog = [...newLogEntries, ...prev.log].slice(0, MAX_LOG_ENTRIES);

          return {
            ...prev,
            endpoints:     nextEndpoints,
            log:           nextLog,
            frameCount:    prev.frameCount + 1,
            systemMetrics: nextSystem,
            rdsMetrics:    nextRds,
          };
        });
      }

      // Schedule the next frame unconditionally (loop runs until unmount).
      rafHandleRef.current = requestAnimationFrame(loop);
    };

    rafHandleRef.current = requestAnimationFrame(loop);
  }, []);

  // ── WebSocket lifecycle ────────────────────────────────────────────────────

  const connect = useCallback(() => {
    // Tear down any existing connection before creating a new one.
    if (wsRef.current) {
      wsRef.current.onmessage = null;
      wsRef.current.onerror   = null;
      wsRef.current.onclose   = null;
      wsRef.current.close();
      wsRef.current = null;
    }

    if (unmountedRef.current) return;

    if (!unmountedRef.current) {
      setState(prev => ({ ...prev, status: 'connecting' }));
    }

    const ws = new WebSocket(WS_URL);
    wsRef.current = ws;

    ws.onopen = () => {
      if (unmountedRef.current) return;
      reconnectAttemptsRef.current = 0; // reset back-off on success
      setState(prev => ({ ...prev, status: 'connected' }));
    };

    /**
     * onmessage — the ingestion hot path.
     *
     * CRITICAL: We do NOT call setState here. We push the parsed message
     * into the mutable deltaQueueRef and let the rAF loop flush it on the
     * next vsync boundary. This keeps the main thread unblocked between
     * 500 ms network ticks.
     */
    ws.onmessage = (event: MessageEvent<string>) => {
      try {
        const msg = JSON.parse(event.data) as WSMessage;
        // Push into the delta queue — zero React involvement.
        deltaQueueRef.current.push(msg);
      } catch {
        // Malformed JSON — silently discard.
      }
    };

    ws.onerror = () => {
      // onerror is always followed by onclose; handle reconnect there.
    };

    ws.onclose = () => {
      if (unmountedRef.current) return;

      setState(prev => ({ ...prev, status: 'disconnected' }));

      // Exponential back-off reconnect.
      const delay = Math.min(
        RECONNECT_BASE_MS * Math.pow(RECONNECT_EXPONENT, reconnectAttemptsRef.current),
        RECONNECT_MAX_MS,
      );
      reconnectAttemptsRef.current++;

      reconnectTimerRef.current = setTimeout(() => {
        if (!unmountedRef.current) connect();
      }, delay);
    };
  }, []);

  // ── Mount / unmount ────────────────────────────────────────────────────────

  useEffect(() => {
    unmountedRef.current = false;

    // Start the rAF render loop.
    startRafLoop();

    // Open WebSocket.
    connect();

    return () => {
      // Signal rAF loop to stop.
      unmountedRef.current = true;

      // Cancel rAF.
      cancelAnimationFrame(rafHandleRef.current);

      // Cancel pending reconnect timer.
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);

      // Close WebSocket cleanly.
      if (wsRef.current) {
        wsRef.current.onmessage = null;
        wsRef.current.onerror   = null;
        wsRef.current.onclose   = null;
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [connect, startRafLoop]);

  // ── Public reconnect control ───────────────────────────────────────────────

  const reconnect = useCallback(() => {
    reconnectAttemptsRef.current = 0;
    if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
    connect();
  }, [connect]);

  return { ...state, reconnect };
}

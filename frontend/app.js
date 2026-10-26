// Loads the last 15 minutes from /api/history, then stays live with readings
// pushed over /ws. The table is redrawn on every reading (only a few rows, so
// that's cheap) and the clock bits in the status bar update twice a second.

const MAX_READINGS = 90; // the server keeps the same amount
const BAR_WIDTH = 3;
const BAR_GAP = 1;
const CHART_HEIGHT = 32;
const FRESH_MS = 1200; // how long a value that just changed stays highlighted
const SVG_NS = 'http://www.w3.org/2000/svg';

const sites = new Map(); // url -> readings, oldest first
const freshUntil = new Map(); // url -> when its highlight ends
let settings = { interval_ms: 10000, spike_sigma: 2.5, spike_min_ms: 10 };
let lastReading = null; // newest reading from any site
let clockSkew = 0; // browser clock minus server clock, measured from live readings
let hover = null; // { url, ts } of the bar under the pointer
let connection = 'connecting';
let retryDelay = 1000;

const el = {
  intro: document.getElementById('intro'),
  summary: document.getElementById('summary'),
  lastReading: document.getElementById('last-reading'),
  next: document.getElementById('next'),
  nextCheck: document.getElementById('next-check'),
  progress: document.getElementById('progress'),
  table: document.getElementById('table'),
  rows: document.getElementById('rows'),
  hoverInfo: document.getElementById('hover-info'),
  events: document.getElementById('events'),
  how: document.getElementById('how'),
};

// --- data ---

async function loadHistory() {
  const res = await fetch('/api/history', { cache: 'no-store' });
  if (!res.ok) {
    throw new Error('GET /api/history returned ' + res.status);
  }
  const data = await res.json();

  settings = {
    interval_ms: data.interval_ms || settings.interval_ms,
    spike_sigma: data.spike_sigma || settings.spike_sigma,
    spike_min_ms: data.spike_min_ms ?? settings.spike_min_ms,
  };

  sites.clear();
  for (const target of data.targets || []) {
    const readings = (target.readings || []).slice(-MAX_READINGS);
    sites.set(target.url, readings);
    const last = readings[readings.length - 1];
    if (last && (!lastReading || last.timestamp > lastReading.timestamp)) {
      lastReading = last;
    }
  }
}

function addReading(reading) {
  if (!sites.has(reading.url)) {
    sites.set(reading.url, []);
  }
  const list = sites.get(reading.url);

  // a reading can arrive both in the history response and on the socket
  const last = list[list.length - 1];
  if (last && reading.timestamp <= last.timestamp) {
    return;
  }

  list.push(reading);
  if (list.length > MAX_READINGS) {
    list.shift();
  }

  if (!lastReading || reading.timestamp >= lastReading.timestamp) {
    lastReading = reading;
  }
  // the reading was taken at timestamp and finished latency_ms later, which is
  // roughly when it reached us, so this tells us how far off our clock is
  clockSkew = Date.now() - (reading.timestamp + reading.latency_ms);

  freshUntil.set(reading.url, Date.now() + FRESH_MS);
  setTimeout(renderRows, FRESH_MS + 50);
}

function serverNow() {
  return Date.now() - clockSkew;
}

// --- websocket ---

function connect() {
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const socket = new WebSocket(protocol + '//' + location.host + '/ws');
  setConnection('connecting');

  socket.onopen = () => {
    retryDelay = 1000;
    setConnection('open');
  };

  socket.onmessage = (event) => {
    let reading;
    try {
      reading = JSON.parse(event.data);
    } catch {
      return;
    }
    addReading(reading);
    render();
  };

  socket.onclose = () => {
    setConnection('retrying');
    setTimeout(() => {
      connect();
      refresh(); // fill in whatever we missed while disconnected
    }, retryDelay);
    retryDelay = Math.min(retryDelay * 2, 30000);
  };
}

function setConnection(state) {
  connection = state;
  renderClock();
}

async function refresh() {
  try {
    await loadHistory();
  } catch (err) {
    console.error(err);
  }
  render();
}

// --- formatting ---

function siteName(url) {
  try {
    const u = new URL(url);
    const path = u.pathname === '/' ? '' : u.pathname;
    return u.host.replace(/^www\./, '') + path;
  } catch {
    return url;
  }
}

function clock(ts) {
  return new Date(ts).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

function plural(n, word) {
  return n + ' ' + word + (n === 1 ? '' : 's');
}

function duration(ms) {
  const s = Math.max(1, Math.round(ms / 1000));
  if (s < 60) {
    return plural(s, 'second');
  }
  const min = Math.floor(s / 60);
  const rest = s % 60;
  return rest ? min + ' min ' + rest + ' s' : min + ' min';
}

function ago(ms) {
  const s = Math.floor(ms / 1000);
  if (s < 10) {
    return 'just now';
  }
  if (s < 60) {
    return s + ' s ago';
  }
  return Math.floor(s / 60) + ' min ago';
}

function every() {
  const s = Math.round(settings.interval_ms / 1000);
  return s === 1 ? 'every second' : 'every ' + s + ' seconds';
}

// --- rendering ---

function render() {
  renderIntro();
  renderSummary();
  renderRows();
  renderEvents();
  renderClock();
}

function renderIntro() {
  if (sites.size > 0) {
    el.intro.textContent = 'Checks ' + plural(sites.size, 'website') + ' ' + every() +
      ' and flags any response that is unusually slow for that site.';
  }
  el.how.textContent = 'A Go worker requests each site ' + every() + ' and times how long the first ' +
    'response takes. Each site keeps its own baseline from its last 100 good readings. A reading counts ' +
    'as a spike when it is more than ' + settings.spike_sigma + ' standard deviations above that baseline ' +
    'and at least ' + settings.spike_min_ms + ' ms slower. Readings reach this page through Redis and a WebSocket.';
}

function renderSummary() {
  if (sites.size === 0) {
    el.summary.textContent = 'No sites are being checked yet.';
    return;
  }

  let down = 0;
  let waiting = 0;
  let spikes = 0;
  for (const list of sites.values()) {
    const last = list[list.length - 1];
    if (!last) {
      waiting++;
    } else if (!last.ok) {
      down++;
    }
    spikes += list.filter((r) => r.anomaly).length;
  }

  let text;
  if (waiting === sites.size) {
    text = 'Waiting for the first readings.';
  } else if (down === 0) {
    text = sites.size === 1 ? 'The site is up right now.' : 'All ' + sites.size + ' sites are up right now.';
  } else {
    text = down + ' of ' + plural(sites.size, 'site') + (down === 1 ? ' is' : ' are') + ' not responding.';
  }
  if (spikes > 0) {
    text += ' ' + plural(spikes, 'slow response') + ' in the last 15 minutes.';
  }
  el.summary.textContent = text;
}

function renderClock() {
  el.next.hidden = true;

  if (connection === 'retrying') {
    el.lastReading.textContent = 'Connection lost, trying again';
    return;
  }
  if (connection === 'connecting') {
    el.lastReading.textContent = 'Connecting';
    return;
  }
  if (!lastReading) {
    el.lastReading.textContent = 'Connected, waiting for the first reading';
    return;
  }

  const since = serverNow() - lastReading.timestamp;
  if (since > settings.interval_ms * 3) {
    el.lastReading.textContent = 'No new readings since ' + clock(lastReading.timestamp);
    return;
  }

  const secs = Math.floor(since / 1000);
  el.lastReading.textContent = secs < 1 ? 'Last reading just now' : 'Last reading ' + secs + ' s ago';

  const left = lastReading.timestamp + settings.interval_ms - serverNow();
  const done = Math.min(1, Math.max(0, 1 - left / settings.interval_ms));
  el.next.hidden = false;
  el.progress.style.width = Math.round(done * 100) + '%';
  el.nextCheck.textContent = left > 0 ? 'Next check in ' + Math.ceil(left / 1000) + ' s' : 'Checking now';
}

function renderRows() {
  const rows = document.createDocumentFragment();
  const now = Date.now();

  for (const [url, list] of sites) {
    const tr = document.createElement('tr');
    tr.append(
      textCell(siteName(url), 'site'),
      statusCell(list[list.length - 1]),
      nowCell(url, list, now),
      textCell(normal(list), 'num muted'),
      chartCell(url, list),
      textCell(uptime(list), 'num'),
    );
    rows.append(tr);
  }

  el.rows.replaceChildren(rows);
  renderHoverInfo();
}

function textCell(text, className) {
  const td = document.createElement('td');
  td.className = className || '';
  td.textContent = text;
  return td;
}

function statusCell(last) {
  let label = 'Up';
  let kind = 'up';
  if (!last) {
    label = 'Waiting';
    kind = 'waiting';
  } else if (!last.ok) {
    label = 'Down';
    kind = 'down';
  } else if (last.anomaly) {
    label = 'Slow';
    kind = 'slow';
  }

  const td = document.createElement('td');
  const pill = document.createElement('span');
  pill.className = 'pill ' + kind;
  pill.textContent = label;
  td.append(pill);
  return td;
}

function nowCell(url, list, now) {
  const last = list[list.length - 1];
  if (!last) {
    return textCell('-', 'muted');
  }

  const box = document.createElement('span');
  box.className = (freshUntil.get(url) || 0) > now ? 'now fresh' : 'now';
  const value = document.createElement('span');
  const change = document.createElement('span');
  change.className = 'change';

  if (!last.ok) {
    value.className = 'error';
    value.textContent = last.error || 'No response';
  } else {
    value.textContent = Math.round(last.latency_ms) + ' ms';
    const prev = list[list.length - 2];
    if (prev && !prev.ok) {
      change.textContent = 'back up';
    } else if (prev) {
      const diff = Math.round(last.latency_ms) - Math.round(prev.latency_ms);
      change.textContent = (diff < 0 ? '\u2212' : '+') + Math.abs(diff) + ' ms';
    }
  }

  box.append(value, change);
  const td = document.createElement('td');
  td.append(box);
  return td;
}

function normal(list) {
  for (let i = list.length - 1; i >= 0; i--) {
    if (list[i].ok && list[i].baseline_ms > 0) {
      return Math.round(list[i].baseline_ms) + ' ms';
    }
  }
  return '-';
}

function uptime(list) {
  if (list.length === 0) {
    return '-';
  }
  const pct = (list.filter((r) => r.ok).length / list.length) * 100;
  return (Number.isInteger(pct) ? pct : pct.toFixed(1)) + '%';
}

function chartCell(url, list) {
  const width = MAX_READINGS * (BAR_WIDTH + BAR_GAP);
  const svg = document.createElementNS(SVG_NS, 'svg');
  svg.setAttribute('class', 'bars');
  svg.setAttribute('width', width);
  svg.setAttribute('height', CHART_HEIGHT);
  svg.setAttribute('viewBox', '0 0 ' + width + ' ' + CHART_HEIGHT);
  svg.setAttribute('role', 'img');
  svg.setAttribute('data-url', url);

  const latencies = list.filter((r) => r.ok).map((r) => r.latency_ms);
  const min = Math.min(...latencies);
  const range = Math.max(1, Math.max(...latencies) - min);

  // newest reading on the right, so a short history fills in from the right
  const offset = MAX_READINGS - list.length;
  let spikes = 0;
  let failures = 0;

  list.forEach((r, i) => {
    let h = CHART_HEIGHT;
    let kind = 'down';
    if (r.ok) {
      h = 3 + Math.round(((r.latency_ms - min) / range) * (CHART_HEIGHT - 5));
      kind = r.anomaly ? 'spike' : 'ok';
    }
    if (kind === 'spike') spikes++;
    if (kind === 'down') failures++;
    if (hover && hover.url === url && hover.ts === r.timestamp) {
      kind += ' hover';
    }

    const bar = document.createElementNS(SVG_NS, 'rect');
    bar.setAttribute('x', (offset + i) * (BAR_WIDTH + BAR_GAP));
    bar.setAttribute('y', CHART_HEIGHT - h);
    bar.setAttribute('width', BAR_WIDTH);
    bar.setAttribute('height', h);
    bar.setAttribute('class', kind);
    svg.append(bar);
  });

  svg.setAttribute('aria-label', 'Last 15 minutes for ' + siteName(url) + ': ' +
    plural(list.length, 'reading') + ', ' + plural(spikes, 'spike') + ', ' + plural(failures, 'failed check'));

  const td = document.createElement('td');
  td.append(svg);
  return td;
}

// works out which reading is under the pointer from its x position, so short
// bars are as easy to point at as tall ones
function onPointerMove(event) {
  const svg = event.target.closest && event.target.closest('svg.bars');
  if (!svg) {
    return;
  }
  const url = svg.getAttribute('data-url');
  const list = sites.get(url) || [];
  const box = svg.getBoundingClientRect();
  if (box.width === 0) {
    return;
  }
  const slot = Math.floor(((event.clientX - box.left) / box.width) * MAX_READINGS);
  const reading = list[slot - (MAX_READINGS - list.length)];
  const next = reading ? { url, ts: reading.timestamp } : null;

  if (next?.url !== hover?.url || next?.ts !== hover?.ts) {
    hover = next;
    renderRows();
  }
}

function renderHoverInfo() {
  const list = hover ? sites.get(hover.url) || [] : [];
  const r = list.find((x) => x.timestamp === hover?.ts);
  if (!r) {
    el.hoverInfo.className = 'muted num';
    el.hoverInfo.textContent = 'Point at a bar to see that reading.';
    return;
  }

  const at = siteName(hover.url) + ' at ' + clock(r.timestamp) + ': ';
  el.hoverInfo.className = 'num';
  if (!r.ok) {
    el.hoverInfo.textContent = at + 'no response (' + (r.error || 'unknown') + ')';
    return;
  }
  let text = at + Math.round(r.latency_ms) + ' ms';
  if (r.baseline_ms > 0) {
    text += ', normal is about ' + Math.round(r.baseline_ms) + ' ms';
  }
  if (r.anomaly) {
    text += ', flagged as a spike';
  }
  el.hoverInfo.textContent = text;
}

function renderEvents() {
  const events = [];

  for (const [url, list] of sites) {
    const name = siteName(url);
    let downSince = null;

    for (const r of list) {
      if (!r.ok) {
        downSince = downSince || r;
        continue;
      }
      if (downSince) {
        events.push({
          at: downSince.timestamp,
          text: name + ' stopped responding (' + (downSince.error || 'no response') + '). Back after ' +
            duration(r.timestamp - downSince.timestamp) + '.',
        });
        downSince = null;
      }
      if (r.anomaly) {
        events.push({
          at: r.timestamp,
          text: name + ' took ' + Math.round(r.latency_ms) + ' ms, it normally answers in about ' +
            Math.round(r.baseline_ms) + ' ms.',
        });
      }
    }

    if (downSince) {
      events.push({
        at: downSince.timestamp,
        text: name + ' stopped responding (' + (downSince.error || 'no response') + ') and is still down.',
      });
    }
  }

  events.sort((a, b) => b.at - a.at);

  const items = document.createDocumentFragment();
  const now = serverNow();
  for (const e of events.slice(0, 8)) {
    const li = document.createElement('li');
    const time = document.createElement('time');
    time.dateTime = new Date(e.at).toISOString();
    time.textContent = clock(e.at);
    const text = document.createElement('span');
    text.textContent = e.text;
    const when = document.createElement('span');
    when.className = 'ago';
    when.textContent = ago(now - e.at);
    li.append(time, text, when);
    items.append(li);
  }

  if (events.length === 0) {
    const li = document.createElement('li');
    li.className = 'muted';
    li.textContent = 'Nothing unusual in the last 15 minutes.';
    items.append(li);
  }

  el.events.replaceChildren(items);
}

el.rows.addEventListener('mousemove', onPointerMove);
el.table.addEventListener('mouseleave', () => {
  if (hover) {
    hover = null;
    renderRows();
  }
});

// countdown and "x s ago" texts
setInterval(() => {
  renderClock();
  renderEvents();
}, 500);

connect();
refresh();

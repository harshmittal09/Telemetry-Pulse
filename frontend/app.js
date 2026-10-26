// Loads the last 15 minutes from /api/history, then keeps the page live with
// readings pushed over /ws. Everything is redrawn on each reading, there are
// only a handful of rows so that's cheap.

const MAX_READINGS = 90; // the server keeps the same amount
const BAR_WIDTH = 3;
const BAR_GAP = 1;
const CHART_HEIGHT = 32;
const SVG_NS = 'http://www.w3.org/2000/svg';

const sites = new Map(); // url -> readings, oldest first
let intervalMs = 10000;
let lastReadingAt = null;
let connection = 'connecting';
let retryDelay = 1000;

const el = {
  intro: document.getElementById('intro'),
  summary: document.getElementById('summary'),
  connection: document.getElementById('connection'),
  rows: document.getElementById('rows'),
  events: document.getElementById('events'),
  interval: document.getElementById('interval'),
};

// --- data ---

async function loadHistory() {
  const res = await fetch('/api/history', { cache: 'no-store' });
  if (!res.ok) {
    throw new Error('GET /api/history returned ' + res.status);
  }
  const data = await res.json();

  intervalMs = data.interval_ms || intervalMs;
  sites.clear();
  for (const target of data.targets || []) {
    const readings = (target.readings || []).slice(-MAX_READINGS);
    sites.set(target.url, readings);
    const last = readings[readings.length - 1];
    if (last && (!lastReadingAt || last.timestamp > lastReadingAt)) {
      lastReadingAt = last.timestamp;
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
  lastReadingAt = reading.timestamp;
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
  renderConnection();
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

function duration(ms) {
  const s = Math.max(1, Math.round(ms / 1000));
  if (s < 60) {
    return plural(s, 'second');
  }
  const min = Math.floor(s / 60);
  const rest = s % 60;
  return rest ? min + ' min ' + rest + ' s' : min + ' min';
}

function plural(n, word) {
  return n + ' ' + word + (n === 1 ? '' : 's');
}

// --- rendering ---

function render() {
  renderIntro();
  renderSummary();
  renderRows();
  renderEvents();
  renderConnection();
}

function renderIntro() {
  const seconds = Math.round(intervalMs / 1000);
  const every = seconds === 1 ? 'every second' : 'every ' + seconds + ' seconds';
  el.interval.textContent = seconds === 1 ? 'second' : seconds + ' seconds';
  if (sites.size > 0) {
    el.intro.textContent = 'Checks ' + plural(sites.size, 'website') + ' ' + every +
      ' and flags any response that is unusually slow for that site.';
  }
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

function renderConnection() {
  if (connection === 'retrying') {
    el.connection.textContent = 'Connection lost, trying again';
    return;
  }
  if (connection === 'connecting') {
    el.connection.textContent = 'Connecting';
    return;
  }
  if (!lastReadingAt) {
    el.connection.textContent = 'Connected, waiting for readings';
    return;
  }
  const stale = Date.now() - lastReadingAt > intervalMs * 3;
  el.connection.textContent = stale
    ? 'Connected, but no new readings since ' + clock(lastReadingAt)
    : 'Connected, last reading at ' + clock(lastReadingAt);
}

function renderRows() {
  const rows = document.createDocumentFragment();

  for (const [url, list] of sites) {
    const last = list[list.length - 1];
    const tr = document.createElement('tr');

    tr.append(
      textCell(siteName(url), 'site'),
      statusCell(last),
      nowCell(last),
      textCell(normal(list), 'num muted'),
      chartCell(url, list),
      textCell(uptime(list), 'num'),
    );
    rows.append(tr);
  }

  el.rows.replaceChildren(rows);
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

function nowCell(last) {
  if (!last) {
    return textCell('-', 'muted');
  }
  if (!last.ok) {
    return textCell(last.error || 'No response', 'error');
  }
  return textCell(Math.round(last.latency_ms) + ' ms', 'num');
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
  const ok = list.filter((r) => r.ok).length;
  const pct = (ok / list.length) * 100;
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

  const latencies = list.filter((r) => r.ok).map((r) => r.latency_ms);
  const min = Math.min(...latencies);
  const range = Math.max(1, Math.max(...latencies) - min);

  // newest reading on the right, so a short history fills in from the right
  const offset = MAX_READINGS - list.length;
  const paths = { ok: '', spike: '', down: '' };
  let spikes = 0;
  let failures = 0;

  list.forEach((r, i) => {
    const x = (offset + i) * (BAR_WIDTH + BAR_GAP);
    if (!r.ok) {
      paths.down += 'M' + x + ' 0h' + BAR_WIDTH + 'v' + CHART_HEIGHT + 'h-' + BAR_WIDTH + 'Z';
      failures++;
      return;
    }
    const h = 3 + Math.round(((r.latency_ms - min) / range) * (CHART_HEIGHT - 5));
    const bar = 'M' + x + ' ' + (CHART_HEIGHT - h) + 'h' + BAR_WIDTH + 'v' + h + 'h-' + BAR_WIDTH + 'Z';
    if (r.anomaly) {
      paths.spike += bar;
      spikes++;
    } else {
      paths.ok += bar;
    }
  });

  svg.setAttribute('aria-label', 'Last 15 minutes for ' + siteName(url) + ': ' +
    plural(list.length, 'reading') + ', ' + plural(spikes, 'spike') + ', ' + plural(failures, 'failed check'));

  for (const kind of ['down', 'ok', 'spike']) {
    if (paths[kind]) {
      const path = document.createElementNS(SVG_NS, 'path');
      path.setAttribute('class', kind);
      path.setAttribute('d', paths[kind]);
      svg.append(path);
    }
  }

  const td = document.createElement('td');
  td.append(svg);
  return td;
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
  for (const e of events.slice(0, 10)) {
    const li = document.createElement('li');
    const time = document.createElement('time');
    time.dateTime = new Date(e.at).toISOString();
    time.textContent = clock(e.at);
    const text = document.createElement('span');
    text.textContent = e.text;
    li.append(time, text);
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

// keeps "last reading at" honest if the server goes quiet
setInterval(renderConnection, 5000);

connect();
refresh();

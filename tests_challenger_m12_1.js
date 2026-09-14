/**
 * Challenger M12-1: Empirical Test Harness
 * Target: web/static/index.html (Milestone M12 - Cross-LLM FinOps Tab 1)
 *
 * Tests:
 *  1. DOM ID Integrity & Absence of Duplicate IDs
 *  2. Initial Page Load & Tab 1 Default State
 *  3. Tab Switching Transitions: cross -> agy -> cross
 *  4. Tab Switching Transitions: cross -> codex -> cross
 *  5. Tab Switching Transitions: cross -> claude -> cross
 *  6. Sub-view Isolation while on Tab 1
 *  7. Sort Button Transitions: tokens -> cost -> activity -> tokens
 *  8. Time Range Transitions: today -> 24h -> 7d -> 30d -> all
 *  9. Combined Time Range & Sort Persistence
 * 10. Zero Mock Data Contract & Honest Empty State Rendering
 * 11. Division-by-Zero Resilience (All 0s)
 * 12. Rapid Fuzzing / Stress Test (100 transitions)
 */

const fs = require('fs');
const path = require('path');
const vm = require('vm');

const htmlPath = path.resolve(__dirname, 'web/static/index.html');
const html = fs.readFileSync(htmlPath, 'utf8');

let totalTests = 0;
let passedTests = 0;
let failedTests = 0;

function assert(condition, message) {
  totalTests++;
  if (condition) {
    passedTests++;
    console.log(`  ✅ PASS: ${message}`);
  } else {
    failedTests++;
    console.error(`  ❌ FAIL: ${message}`);
  }
}

console.log('================================================================');
console.log('   CHALLENGER M12-1: EMPIRICAL DOM STATE TRANSITION TEST SUITE');
console.log('================================================================\n');

// -------------------------------------------------------------
// SUITE 1: DOM ID Integrity & Element Layout
// -------------------------------------------------------------
console.log('--- SUITE 1: Static DOM ID Integrity & Element Layout ---');

const idRegex = /\bid=["']([^"']+)["']/g;
const idsFound = {};
const duplicateIds = [];
let idMatch;
while ((idMatch = idRegex.exec(html)) !== null) {
  const id = idMatch[1];
  if (idsFound[id]) {
    duplicateIds.push(id);
    idsFound[id]++;
  } else {
    idsFound[id] = 1;
  }
}
assert(duplicateIds.length === 0, `Zero duplicate IDs across entire HTML (found: ${duplicateIds.join(', ') || 'none'})`);

const requiredIds = [
  'btn-provider-cross',
  'btn-provider-agy',
  'btn-provider-codex',
  'btn-provider-claude',
  'cross-toolbar-label',
  'view-section-cross-llm',
  'cross-num-grand-tokens',
  'cross-sub-prompt',
  'cross-sub-output',
  'cross-sub-cache',
  'cross-sub-think',
  'cross-num-est-cost',
  'cross-sub-cost-savings',
  'cross-txt-cost-sub',
  'cross-top-consumer-name',
  'cross-top-consumer-pct',
  'cross-top-consumer-tokens',
  'cross-most-active-name',
  'cross-most-active-calls',
  'cross-most-active-pct',
  'cross-num-cache-pct',
  'cross-sub-cached-tokens',
  'echart-cross-stacked-bar',
  'echart-cross-donut',
  'btn-sort-tokens',
  'btn-sort-cost',
  'btn-sort-activity',
  'leaderboard-table',
  'leaderboard-table-body',
  'range-today',
  'range-24h',
  'range-7d',
  'range-30d',
  'range-all'
];

requiredIds.forEach(id => {
  assert(idsFound[id] === 1, `Required element #${id} exists uniquely in DOM`);
});

// Verify Tab 1 button position: it must be the FIRST child in .provider-tabs
const providerTabsMatch = html.match(/<div class="provider-tabs">([\s\S]*?)<\/div>/);
assert(providerTabsMatch !== null, '.provider-tabs container found');
if (providerTabsMatch) {
  const firstButtonMatch = providerTabsMatch[1].trim().match(/<button[\s\S]*?id=["']([^"']+)["']/);
  assert(firstButtonMatch && firstButtonMatch[1] === 'btn-provider-cross',
    `#btn-provider-cross is the FIRST button inside .provider-tabs (found: ${firstButtonMatch ? firstButtonMatch[1] : 'none'})`);
}

// -------------------------------------------------------------
// DOM Emulator & Execution Sandbox
// -------------------------------------------------------------
class MockClassList {
  constructor(el) {
    this.el = el;
    this.classes = new Set();
  }
  add(...names) {
    names.forEach(n => { if (n) this.classes.add(n); });
    this.el.className = Array.from(this.classes).join(' ');
  }
  remove(...names) {
    names.forEach(n => this.classes.delete(n));
    this.el.className = Array.from(this.classes).join(' ');
  }
  toggle(name, force) {
    if (force !== undefined) {
      if (force) this.add(name);
      else this.remove(name);
      return force;
    }
    if (this.classes.has(name)) {
      this.remove(name);
      return false;
    } else {
      this.add(name);
      return true;
    }
  }
  contains(name) {
    return this.classes.has(name);
  }
}

class MockElement {
  constructor(tagName = 'div', id = '', className = '') {
    this.tagName = tagName.toUpperCase();
    this.id = id;
    this.style = { display: '' };
    this.dataset = {};
    this.children = [];
    this.parentNode = null;
    this.clientWidth = 500;
    this.clientHeight = 320;
    this._textContent = '';
    this._innerHTML = '';
    this.classList = new MockClassList(this);
    if (className) {
      this.classList.add(...className.split(/\s+/).filter(Boolean));
    }
  }

  appendChild(child) {
    this.children.push(child);
    child.parentNode = this;
    return child;
  }
  insertBefore(newChild, refChild) {
    const idx = this.children.indexOf(refChild);
    if (idx >= 0) this.children.splice(idx, 0, newChild);
    else this.children.push(newChild);
    newChild.parentNode = this;
    return newChild;
  }
  removeChild(child) {
    const idx = this.children.indexOf(child);
    if (idx >= 0) this.children.splice(idx, 1);
    child.parentNode = null;
    return child;
  }

  get textContent() { return this._textContent; }
  set textContent(val) { this._textContent = String(val); }

  get innerHTML() { return this._innerHTML; }
  set innerHTML(val) { this._innerHTML = String(val); }

  setAttribute(name, val) {
    if (name.startsWith('data-')) {
      const key = name.slice(5).replace(/-([a-z])/g, (_, c) => c.toUpperCase());
      this.dataset[key] = val;
    }
  }

  getAttribute(name) {
    if (name.startsWith('data-')) {
      const key = name.slice(5).replace(/-([a-z])/g, (_, c) => c.toUpperCase());
      return this.dataset[key];
    }
    return null;
  }

  scrollIntoView() {}
}

class MockDocument {
  constructor() {
    this.elementsById = new Map();
    this.allElements = [];
  }

  createElement(tag) {
    const el = new MockElement(tag);
    if (tag.toLowerCase() === 'canvas') {
      el.getContext = () => new Proxy({
        measureText: () => ({ width: 50 }),
        createLinearGradient: () => ({ addColorStop: () => {} }),
        createRadialGradient: () => ({ addColorStop: () => {} })
      }, {
        get: (target, prop) => {
          if (prop in target) return target[prop];
          return () => {};
        },
        set: () => true
      });
    }
    this.allElements.push(el);
    return el;
  }

  register(el) {
    if (el.id) this.elementsById.set(el.id, el);
    this.allElements.push(el);
    return el;
  }

  getElementById(id) {
    return this.elementsById.get(id) || null;
  }

  querySelector(selector) {
    const list = this.querySelectorAll(selector);
    return list.length > 0 ? list[0] : null;
  }

  querySelectorAll(selector) {
    if (selector.startsWith('.')) {
      const cls = selector.slice(1);
      return this.allElements.filter(e => e.classList.contains(cls));
    }
    if (selector.startsWith('#')) {
      const id = selector.slice(1);
      const el = this.getElementById(id);
      return el ? [el] : [];
    }
    return this.allElements.filter(e => e.tagName.toLowerCase() === selector.toLowerCase());
  }

  addEventListener() {}
}

function buildEmulatedDOM() {
  const doc = new MockDocument();

  // Parse HTML elements with id and class
  const tagRegex = /<([a-zA-Z0-9\-]+)([^>]*)>/g;
  let tagMatch;
  while ((tagMatch = tagRegex.exec(html)) !== null) {
    const tagName = tagMatch[1];
    const attrsStr = tagMatch[2];
    const idM = attrsStr.match(/\bid=["']([^"']+)["']/);
    const classM = attrsStr.match(/\bclass=["']([^"']+)["']/);

    const id = idM ? idM[1] : '';
    const cls = classM ? classM[1] : '';

    if (id || cls) {
      const el = new MockElement(tagName, id, cls);
      doc.register(el);
    }
  }

  return doc;
}

// -------------------------------------------------------------
// Script Extraction & Sandbox Execution
// -------------------------------------------------------------
const scriptContent = html.match(/<script>([\s\S]*?)<\/script>/)[1];

function createSandbox(doc) {
  const storage = {};
  const mockLocalStorage = {
    getItem: k => (k in storage ? storage[k] : null),
    setItem: (k, v) => { storage[k] = String(v); },
    removeItem: k => { delete storage[k]; },
    clear: () => { for (const k in storage) delete storage[k]; }
  };

  const fetchLog = [];
  const mockFetch = async (url) => {
    fetchLog.push(url);
    if (url.includes('/api/projects/leaderboard')) {
      return {
        ok: true,
        json: async () => ({
          kpis: {
            grand_total_tokens: 12500000,
            grand_total_cost_usd: 128.50,
            grand_total_savings_usd: 42.00,
            grand_total_activity: 350,
            top_consumer_project: 'TokenMonitor',
            top_consumer_percent: 65.4,
            top_consumer_tokens: 8175000,
            top_active_project: 'TokenMonitor',
            top_active_count: 210,
            overall_cache_hit_percent: 31.8
          },
          projects: [
            {
              rank: 1,
              project_id: 'tokenmonitor',
              project_name: 'TokenMonitor (GoLangDev)',
              workspace: 'E:/GoogleDrive/WorkSpace/Code/ProjectGolang/GoLangDev/TokenMonitor',
              status: 'RUNNING',
              total_tokens: 8175000,
              prompt_tokens: 5000000,
              output_tokens: 2000000,
              cached_tokens: 1000000,
              thinking_tokens: 175000,
              estimated_cost_usd: 84.20,
              total_calls: 150,
              agent_tasks: 60,
              total_activity: 210,
              cache_hit_percent: 33.3,
              google_breakdown: { tokens: 7000000, percentage: 85.6 },
              openai_breakdown: { tokens: 1000000, percentage: 12.2 },
              claude_breakdown: { tokens: 175000, percentage: 2.2 }
            },
            {
              rank: 2,
              project_id: 'mcredit',
              project_name: 'MCREDIT (ProjectR)',
              workspace: 'E:/GoogleDrive/WorkSpace/Code/ProjectR',
              status: 'COMPLETED',
              total_tokens: 3000000,
              prompt_tokens: 2000000,
              output_tokens: 800000,
              cached_tokens: 200000,
              thinking_tokens: 0,
              estimated_cost_usd: 31.50,
              total_calls: 80,
              agent_tasks: 20,
              total_activity: 100,
              cache_hit_percent: 25.0,
              google_breakdown: { tokens: 3000000, percentage: 100.0 },
              openai_breakdown: { tokens: 0, percentage: 0.0 },
              claude_breakdown: { tokens: 0, percentage: 0.0 }
            },
            {
              rank: 3,
              project_id: 'tieuchuanhardeninglinux',
              project_name: 'TieuChuanHardeningLinux',
              workspace: 'E:/GoogleDrive/WorkSpace/Code/Security',
              status: 'STANDBY',
              total_tokens: 1325000,
              prompt_tokens: 900000,
              output_tokens: 350000,
              cached_tokens: 75000,
              thinking_tokens: 0,
              estimated_cost_usd: 12.80,
              total_calls: 30,
              agent_tasks: 10,
              total_activity: 40,
              cache_hit_percent: 18.2,
              google_breakdown: { tokens: 1325000, percentage: 100.0 },
              openai_breakdown: { tokens: 0, percentage: 0.0 },
              claude_breakdown: { tokens: 0, percentage: 0.0 }
            }
          ]
        })
      };
    }
    if (url.includes('/api/account')) {
      return {
        ok: true,
        json: async () => ({
          plan_name: 'NES GAME',
          email: 'ethanpham671986@gmail.com',
          quota_bandwidth: '20x Bandwidth',
          account_type: 'Google Consumer Account (Individual)',
          installation_uuid: 'mock-uuid-1234',
          registered_at: '2026-09-06',
          subscription_expiry: '2026-10-06',
          days_remaining: 22,
          token_expires_in: 'Valid'
        })
      };
    }
    return {
      ok: true,
      json: async () => ({})
    };
  };

  const chartOps = {
    barSetOption: [],
    donutSetOption: [],
    barResize: 0,
    donutResize: 0
  };

  const makeProxy = () => new Proxy(() => {}, {
    get: (target, prop) => {
      if (prop === 'then') return undefined;
      if (prop === 'length') return 0;
      if (prop === 'edges' || prop === 'nodes' || prop === 'links' || prop === 'data') return [];
      if (prop === 'getOption') return () => ({ series: [{ data: [], links: [] }] });
      if (prop === Symbol.toPrimitive) return (hint) => (hint === 'number' ? 0 : '');
      return makeProxy();
    },
    apply: () => makeProxy()
  });

  const mockEcharts = {
    init: (dom) => {
      const isBar = dom.id === 'echart-cross-stacked-bar';
      return {
        setOption: (opt) => {
          if (isBar) chartOps.barSetOption.push(opt);
          else chartOps.donutSetOption.push(opt);
        },
        clear: () => {},
        resize: () => {
          if (isBar) chartOps.barResize++;
          else chartOps.donutResize++;
        },
        on: () => {},
        off: () => {},
        getZr: () => ({ on: () => {}, off: () => {}, painter: { getViewPortWidth: () => 800, getViewPortHeight: () => 600 } }),
        getOption: () => ({ series: [{ data: [], links: [] }] }),
        getModel: () => makeProxy(),
        convertToPixel: () => [0, 0]
      };
    },
    graphic: {
      LinearGradient: class { constructor() {} },
      clipRectByRect: () => ({ x: 0, y: 0, width: 100, height: 100 })
    }
  };

  const sandbox = {
    window: {
      addEventListener: () => {},
      onresize: null
    },
    document: doc,
    localStorage: mockLocalStorage,
    fetch: mockFetch,
    echarts: mockEcharts,
    Chart: class { constructor() {} },
    requestAnimationFrame: (cb) => {
      // Defer execution and protect against recursive animation loops
      setImmediate(() => {
        try { cb(Date.now()); } catch(e) {}
      });
      return 1;
    },
    cancelAnimationFrame: () => {},
    setInterval: () => 1,
    clearInterval: () => {},
    setTimeout: (cb) => cb(),
    performance: globalThis.performance || { now: () => Date.now() },
    console: {
      log: () => {},
      warn: () => {},
      error: (...args) => console.error('    [Sandbox Error]:', ...args)
    },
    // Expose inspection helpers
    _fetchLog: fetchLog,
    _chartOps: chartOps,
    evalInContext: (code) => vm.runInContext(code, sandbox)
  };

  sandbox.window.document = doc;
  vm.createContext(sandbox);

  // Execute index.html script inside sandbox
  vm.runInContext(scriptContent, sandbox);

  return sandbox;
}

// -------------------------------------------------------------
// SUITE 2: Initial Page Load & Tab 1 Default State
// -------------------------------------------------------------
console.log('\n--- SUITE 2: Initial Page Load & Tab 1 Default State ---');
const doc2 = buildEmulatedDOM();
const sb2 = createSandbox(doc2);

assert(sb2.evalInContext('currentAIProvider') === 'cross', 'Default currentAIProvider is "cross"');
assert(doc2.getElementById('btn-provider-cross').classList.contains('active'), '#btn-provider-cross has .active class');
assert(!doc2.getElementById('btn-provider-agy').classList.contains('active'), '#btn-provider-agy is NOT active');
assert(!doc2.getElementById('btn-provider-codex').classList.contains('active'), '#btn-provider-codex is NOT active');
assert(!doc2.getElementById('btn-provider-claude').classList.contains('active'), '#btn-provider-claude is NOT active');

const crossSec2 = doc2.getElementById('view-section-cross-llm');
const overviewSec2 = doc2.getElementById('view-section-overview');
const tableSec2 = doc2.getElementById('view-section-table');
const agentsSec2 = doc2.getElementById('view-section-agents');
const cardsGrid2 = doc2.querySelector('.cards-grid');
const viewTabs2 = doc2.querySelector('.view-tabs');
const crossLabel2 = doc2.getElementById('cross-toolbar-label');

assert(crossSec2 && crossSec2.style.display === 'flex', '#view-section-cross-llm display is "flex" on init');
assert(overviewSec2 && overviewSec2.style.display === 'none', '#view-section-overview display is "none" on init');
assert(tableSec2 && tableSec2.style.display === 'none', '#view-section-table display is "none" on init');
assert(agentsSec2 && agentsSec2.style.display === 'none', '#view-section-agents display is "none" on init');
assert(cardsGrid2 && cardsGrid2.style.display === 'none', '.cards-grid display is "none" on init');
assert(viewTabs2 && viewTabs2.style.display === 'none', '.view-tabs display is "none" on init');
assert(crossLabel2 && crossLabel2.style.display === 'flex', '#cross-toolbar-label display is "flex" on init');

assert(sb2._fetchLog.some(u => u.includes('/api/projects/leaderboard')), 'Initial loadCrossLLMData() fetch was dispatched');

// -------------------------------------------------------------
// SUITE 3: Tab Switching Transitions: cross -> agy -> cross
// -------------------------------------------------------------
console.log('\n--- SUITE 3: Tab Switching Transitions: cross -> agy -> cross ---');
const doc3 = buildEmulatedDOM();
const sb3 = createSandbox(doc3);

// Step 1: Switch to 'agy'
sb3.evalInContext('switchAIProvider("agy")');

assert(sb3.evalInContext('currentAIProvider') === 'agy', 'Provider switched to "agy"');
assert(doc3.getElementById('btn-provider-agy').classList.contains('active'), '#btn-provider-agy has .active');
assert(!doc3.getElementById('btn-provider-cross').classList.contains('active'), '#btn-provider-cross lost .active');

const crossSec3 = doc3.getElementById('view-section-cross-llm');
const crossLabel3 = doc3.getElementById('cross-toolbar-label');
const cardsGrid3 = doc3.querySelector('.cards-grid');
const viewTabs3 = doc3.querySelector('.view-tabs');
const overviewSec3 = doc3.getElementById('view-section-overview');

assert(crossSec3.style.display === 'none', 'After cross -> agy: #view-section-cross-llm is "none" (HIDDEN)');
assert(crossLabel3.style.display === 'none', 'After cross -> agy: #cross-toolbar-label is "none" (HIDDEN)');
assert(cardsGrid3.style.display === 'grid', 'After cross -> agy: .cards-grid is "grid" (RESTORED)');
assert(viewTabs3.style.display === 'flex', 'After cross -> agy: .view-tabs is "flex" (RESTORED)');
assert(overviewSec3.style.display === 'flex', 'After cross -> agy: #view-section-overview is "flex" (ACTIVE VIEW)');

// Step 2: Switch back to 'cross'
sb3.evalInContext('switchAIProvider("cross")');

assert(sb3.evalInContext('currentAIProvider') === 'cross', 'Provider switched back to "cross"');
assert(doc3.getElementById('btn-provider-cross').classList.contains('active'), '#btn-provider-cross has .active restored');
assert(!doc3.getElementById('btn-provider-agy').classList.contains('active'), '#btn-provider-agy lost .active');
assert(crossSec3.style.display === 'flex', 'After agy -> cross: #view-section-cross-llm is "flex" (VISIBLE)');
assert(crossLabel3.style.display === 'flex', 'After agy -> cross: #cross-toolbar-label is "flex" (VISIBLE)');
assert(cardsGrid3.style.display === 'none', 'After agy -> cross: .cards-grid is "none" (HIDDEN)');
assert(viewTabs3.style.display === 'none', 'After agy -> cross: .view-tabs is "none" (HIDDEN)');
assert(overviewSec3.style.display === 'none', 'After agy -> cross: #view-section-overview is "none" (NO GHOST)');

// -------------------------------------------------------------
// SUITE 4: Tab Switching Transitions: cross -> codex -> cross
// -------------------------------------------------------------
console.log('\n--- SUITE 4: Tab Switching Transitions: cross -> codex -> cross ---');
const doc4 = buildEmulatedDOM();
const sb4 = createSandbox(doc4);

// Switch cross -> codex
sb4.evalInContext('switchAIProvider("codex")');

assert(sb4.evalInContext('currentAIProvider') === 'codex', 'Provider switched to "codex"');
assert(doc4.getElementById('btn-provider-codex').classList.contains('active'), '#btn-provider-codex has .active');
assert(!doc4.getElementById('btn-provider-cross').classList.contains('active'), '#btn-provider-cross lost .active');
assert(doc4.getElementById('view-section-cross-llm').style.display === 'none', 'Cross view is hidden on Codex');
assert(doc4.getElementById('cross-toolbar-label').style.display === 'none', 'Cross label is hidden on Codex');
assert(doc4.querySelector('.cards-grid').style.display === 'grid', 'Cards grid is restored on Codex');
assert(doc4.querySelector('.view-tabs').style.display === 'flex', 'View tabs is restored on Codex');

// Switch codex -> cross
sb4.evalInContext('switchAIProvider("cross")');

assert(sb4.evalInContext('currentAIProvider') === 'cross', 'Provider switched back to "cross"');
assert(doc4.getElementById('btn-provider-cross').classList.contains('active'), '#btn-provider-cross has .active restored');
assert(!doc4.getElementById('btn-provider-codex').classList.contains('active'), '#btn-provider-codex lost .active');
assert(doc4.getElementById('view-section-cross-llm').style.display === 'flex', 'Cross view is visible');
assert(doc4.querySelector('.cards-grid').style.display === 'none', 'Cards grid is hidden on Cross');
assert(doc4.querySelector('.view-tabs').style.display === 'none', 'View tabs is hidden on Cross');

// -------------------------------------------------------------
// SUITE 5: Tab Switching Transitions: cross -> claude -> cross
// -------------------------------------------------------------
console.log('\n--- SUITE 5: Tab Switching Transitions: cross -> claude -> cross ---');
const doc5 = buildEmulatedDOM();
const sb5 = createSandbox(doc5);

// Switch cross -> claude
sb5.evalInContext('switchAIProvider("claude")');

assert(sb5.evalInContext('currentAIProvider') === 'claude', 'Provider switched to "claude"');
assert(doc5.getElementById('btn-provider-claude').classList.contains('active'), '#btn-provider-claude has .active');
assert(!doc5.getElementById('btn-provider-cross').classList.contains('active'), '#btn-provider-cross lost .active');
assert(doc5.getElementById('view-section-cross-llm').style.display === 'none', 'Cross view is hidden on Claude');
assert(doc5.querySelector('.cards-grid').style.display === 'grid', 'Cards grid is restored on Claude');

// Switch claude -> cross
sb5.evalInContext('switchAIProvider("cross")');

assert(sb5.evalInContext('currentAIProvider') === 'cross', 'Provider switched back to "cross"');
assert(doc5.getElementById('btn-provider-cross').classList.contains('active'), '#btn-provider-cross has .active restored');
assert(!doc5.getElementById('btn-provider-claude').classList.contains('active'), '#btn-provider-claude lost .active');
assert(doc5.getElementById('view-section-cross-llm').style.display === 'flex', 'Cross view is visible');
assert(doc5.querySelector('.cards-grid').style.display === 'none', 'Cards grid is hidden on Cross');

// -------------------------------------------------------------
// SUITE 6: Sub-view Isolation Guard while on Tab 1
// -------------------------------------------------------------
console.log('\n--- SUITE 6: Sub-view Isolation Guard while on Tab 1 ---');
const doc6 = buildEmulatedDOM();
const sb6 = createSandbox(doc6);

assert(sb6.evalInContext('currentAIProvider') === 'cross', 'Currently on Cross tab');
sb6.evalInContext('switchMainView("table")');
assert(doc6.getElementById('view-section-table').style.display === 'none',
  'switchMainView("table") is safely ignored while on Cross tab (table remains hidden)');
assert(doc6.getElementById('view-section-cross-llm').style.display === 'flex',
  'Cross view remains visible');

sb6.evalInContext('switchMainView("agents")');
assert(doc6.getElementById('view-section-agents').style.display === 'none',
  'switchMainView("agents") is safely ignored while on Cross tab (agents remains hidden)');

sb6.evalInContext('switchMainView("all")');
assert(doc6.getElementById('view-section-overview').style.display === 'none',
  'switchMainView("all") is safely ignored while on Cross tab (overview remains hidden)');

// -------------------------------------------------------------
// SUITE 7: Sort Button Transitions: tokens -> cost -> activity
// -------------------------------------------------------------
console.log('\n--- SUITE 7: Sort Button Transitions: tokens -> cost -> activity ---');
const doc7 = buildEmulatedDOM();
const sb7 = createSandbox(doc7);

const sampleProjects = [
  { project_id: 'p_alpha', project_name: 'Alpha', total_tokens: 1000, estimated_cost_usd: 50.0, total_calls: 10, agent_tasks: 5 },  // act = 15
  { project_id: 'p_beta',  project_name: 'Beta',  total_tokens: 5000, estimated_cost_usd: 10.0, total_calls: 2,  agent_tasks: 1 },  // act = 3
  { project_id: 'p_gamma', project_name: 'Gamma', total_tokens: 2000, estimated_cost_usd: 80.0, total_calls: 50, agent_tasks: 20 }  // act = 70
];

sb7.evalInContext(`cachedLeaderboardData = { projects: ${JSON.stringify(sampleProjects)} }`);

// Test 1: Sort by 'tokens' (default)
sb7.evalInContext('setLeaderboardSort("tokens")');
assert(sb7.evalInContext('currentCrossSort') === 'tokens', 'currentCrossSort is "tokens"');
assert(doc7.getElementById('btn-sort-tokens').classList.contains('active'), '#btn-sort-tokens is active');
assert(!doc7.getElementById('btn-sort-cost').classList.contains('active'), '#btn-sort-cost is NOT active');
assert(!doc7.getElementById('btn-sort-activity').classList.contains('active'), '#btn-sort-activity is NOT active');

let tbodyHtml = doc7.getElementById('leaderboard-table-body').innerHTML;
let firstRowMatch = tbodyHtml.match(/<div[^>]*>([^<]+)<\/div>/);
assert(firstRowMatch && firstRowMatch[1] === 'Beta',
  `Leader by tokens is Beta (5000 tokens) -> actual first row: ${firstRowMatch ? firstRowMatch[1] : 'none'}`);
assert(tbodyHtml.includes('100%'), 'Beta has 100% progress bar');
assert(tbodyHtml.includes('#1 🥇'), 'Beta has rank #1 gold medal badge');

// Test 2: Sort by 'cost'
sb7.evalInContext('setLeaderboardSort("cost")');
assert(sb7.evalInContext('currentCrossSort') === 'cost', 'currentCrossSort is "cost"');
assert(doc7.getElementById('btn-sort-cost').classList.contains('active'), '#btn-sort-cost is active');
assert(!doc7.getElementById('btn-sort-tokens').classList.contains('active'), '#btn-sort-tokens is NOT active');
assert(!doc7.getElementById('btn-sort-activity').classList.contains('active'), '#btn-sort-activity is NOT active');

tbodyHtml = doc7.getElementById('leaderboard-table-body').innerHTML;
firstRowMatch = tbodyHtml.match(/<div[^>]*>([^<]+)<\/div>/);
assert(firstRowMatch && firstRowMatch[1] === 'Gamma',
  `Leader by cost is Gamma ($80.0) -> actual first row: ${firstRowMatch ? firstRowMatch[1] : 'none'}`);
assert(tbodyHtml.includes('$80.00'), 'Gamma row contains $80.00');

// Test 3: Sort by 'activity'
sb7.evalInContext('setLeaderboardSort("activity")');
assert(sb7.evalInContext('currentCrossSort') === 'activity', 'currentCrossSort is "activity"');
assert(doc7.getElementById('btn-sort-activity').classList.contains('active'), '#btn-sort-activity is active');
assert(!doc7.getElementById('btn-sort-tokens').classList.contains('active'), '#btn-sort-tokens is NOT active');
assert(!doc7.getElementById('btn-sort-cost').classList.contains('active'), '#btn-sort-cost is NOT active');

tbodyHtml = doc7.getElementById('leaderboard-table-body').innerHTML;
firstRowMatch = tbodyHtml.match(/<div[^>]*>([^<]+)<\/div>/);
assert(firstRowMatch && firstRowMatch[1] === 'Gamma',
  `Leader by activity is Gamma (70 calls/tasks) -> actual first row: ${firstRowMatch ? firstRowMatch[1] : 'none'}`);

// Test 4: Cycle back to 'tokens'
sb7.evalInContext('setLeaderboardSort("tokens")');
assert(doc7.getElementById('btn-sort-tokens').classList.contains('active'), 'Cycled back: #btn-sort-tokens is active');
firstRowMatch = doc7.getElementById('leaderboard-table-body').innerHTML.match(/<div[^>]*>([^<]+)<\/div>/);
assert(firstRowMatch && firstRowMatch[1] === 'Beta', 'Cycled back: Leader is Beta');

// -------------------------------------------------------------
// SUITE 8: Time Range Transitions: today -> 24h -> 7d -> 30d -> all
// -------------------------------------------------------------
console.log('\n--- SUITE 8: Time Range Transitions while on Tab 1 ---');
const doc8 = buildEmulatedDOM();
const sb8 = createSandbox(doc8);

const rangesToTest = [
  { r: 'today', badge: 'HÔM NAY' },
  { r: '24h',   badge: '24 GIỜ' },
  { r: '7d',    badge: '7 NGÀY' },
  { r: '30d',   badge: '30 NGÀY' },
  { r: 'all',   badge: 'TOÀN BỘ' }
];

rangesToTest.forEach(({ r, badge }) => {
  sb8.evalInContext(`setTimeRange("${r}")`);
  assert(sb8.evalInContext('currentTimeRange') === r, `currentTimeRange set to "${r}"`);
  assert(doc8.getElementById('range-' + r).classList.contains('active'), `#range-${r} has .active class`);

  const badgeEl = doc8.getElementById('echart-range-badge');
  if (badgeEl) {
    assert(badgeEl.textContent.includes(badge), `Range badge updated to contain "${badge}"`);
  }

  const lastFetch = sb8._fetchLog[sb8._fetchLog.length - 1];
  assert(lastFetch.includes(`range=${r}`), `loadCrossLLMData requested with range=${r} (url: ${lastFetch})`);
});

// -------------------------------------------------------------
// SUITE 9: Combined Time Range & Sort Persistence
// -------------------------------------------------------------
console.log('\n--- SUITE 9: Combined Time Range & Sort Persistence ---');
const doc9 = buildEmulatedDOM();
const sb9 = createSandbox(doc9);

sb9.evalInContext('setLeaderboardSort("cost")');
sb9.evalInContext('setTimeRange("7d")');

const lastFetch9 = sb9._fetchLog[sb9._fetchLog.length - 1];
assert(lastFetch9.includes('range=7d') && lastFetch9.includes('sort=cost'),
  `URL combines range=7d AND sort=cost -> ${lastFetch9}`);

// -------------------------------------------------------------
// SUITE 10: Zero Mock Data Contract & Honest Empty State Rendering
// -------------------------------------------------------------
console.log('\n--- SUITE 10: Zero Mock Data Contract & Honest Empty State ---');
const doc10 = buildEmulatedDOM();
const sb10 = createSandbox(doc10);

sb10.evalInContext('renderCrossLLMKPI({}, [])');
sb10.evalInContext('renderLeaderboardTable([])');
sb10.evalInContext('renderCrossCharts({ projects: [] })');

assert(doc10.getElementById('cross-num-grand-tokens').textContent === '0', 'Grand tokens is 0 on empty payload');
assert(doc10.getElementById('cross-num-est-cost').textContent === '$0.00', 'Total cost is $0.00 on empty payload');
assert(doc10.getElementById('cross-top-consumer-name').textContent === '—', 'Top consumer is "—" on empty payload');
assert(doc10.getElementById('cross-most-active-name').textContent === '—', 'Most active is "—" on empty payload');
assert(doc10.getElementById('cross-num-cache-pct').textContent === '0.0%', 'Cache pct is 0.0% on empty payload');

const emptyTableHtml = doc10.getElementById('leaderboard-table-body').innerHTML;
assert(emptyTableHtml.includes('Zero Data Recorded'),
  'Table body displays honest empty state message: "Zero Data Recorded"');

const barOptions = sb10._chartOps.barSetOption;
const lastBarOpt = barOptions[barOptions.length - 1];
assert(lastBarOpt && lastBarOpt.title && lastBarOpt.title.text.includes('Chưa có dữ liệu dự án'),
  'Stacked Bar Chart displays honest empty placeholder title');

const donutOptions = sb10._chartOps.donutSetOption;
const lastDonutOpt = donutOptions[donutOptions.length - 1];
assert(lastDonutOpt && lastDonutOpt.title && lastDonutOpt.title.text.includes('Chưa có chi phí phát sinh'),
  'Donut Cost Chart displays honest empty placeholder title');

// -------------------------------------------------------------
// SUITE 11: Division-by-Zero Resilience (All 0s)
// -------------------------------------------------------------
console.log('\n--- SUITE 11: Division-by-Zero Resilience (All 0s) ---');
const doc11 = buildEmulatedDOM();
const sb11 = createSandbox(doc11);

const allZeroProjects = [
  { project_id: 'z1', project_name: 'Zero 1', total_tokens: 0, estimated_cost_usd: 0, total_calls: 0, agent_tasks: 0 },
  { project_id: 'z2', project_name: 'Zero 2', total_tokens: 0, estimated_cost_usd: 0, total_calls: 0, agent_tasks: 0 }
];

let threw = false;
try {
  sb11.evalInContext('currentCrossSort = "tokens"');
  sb11.evalInContext(`renderLeaderboardTable(${JSON.stringify(allZeroProjects)})`);
  const tblTokens = doc11.getElementById('leaderboard-table-body').innerHTML;
  assert(!tblTokens.includes('NaN'), 'Tokens sort on 0-tokens projects does NOT produce NaN');

  sb11.evalInContext('currentCrossSort = "cost"');
  sb11.evalInContext(`renderLeaderboardTable(${JSON.stringify(allZeroProjects)})`);
  const tblCost = doc11.getElementById('leaderboard-table-body').innerHTML;
  assert(!tblCost.includes('NaN'), 'Cost sort on 0-cost projects does NOT produce NaN');

  sb11.evalInContext('currentCrossSort = "activity"');
  sb11.evalInContext(`renderLeaderboardTable(${JSON.stringify(allZeroProjects)})`);
  const tblAct = doc11.getElementById('leaderboard-table-body').innerHTML;
  assert(!tblAct.includes('NaN'), 'Activity sort on 0-activity projects does NOT produce NaN');
} catch (e) {
  threw = true;
  console.error(e);
}
assert(!threw, 'Zero-metric projects handled without any thrown errors');

// -------------------------------------------------------------
// SUITE 12: Rapid Fuzzing / Stress Test (100 transitions)
// -------------------------------------------------------------
console.log('\n--- SUITE 12: Rapid Fuzzing & Stress Test (100 State Transitions) ---');
const doc12 = buildEmulatedDOM();
const sb12 = createSandbox(doc12);

const providers = ['cross', 'agy', 'codex', 'claude'];
const sorts = ['tokens', 'cost', 'activity'];
const timeRanges = ['today', '24h', '7d', '30d', 'all'];

let fuzzErrors = 0;
for (let i = 0; i < 100; i++) {
  try {
    const p = providers[Math.floor(Math.random() * providers.length)];
    const s = sorts[Math.floor(Math.random() * sorts.length)];
    const tr = timeRanges[Math.floor(Math.random() * timeRanges.length)];

    sb12.evalInContext(`switchAIProvider("${p}")`);
    sb12.evalInContext(`setLeaderboardSort("${s}")`);
    sb12.evalInContext(`setTimeRange("${tr}")`);
  } catch (err) {
    fuzzErrors++;
    console.error(`Fuzz error at step ${i}:`, err);
  }
}
assert(fuzzErrors === 0, `100 randomized transitions executed with 0 errors (fuzz errors: ${fuzzErrors})`);

// Ensure final state can be cleanly set to 'cross'
sb12.evalInContext('switchAIProvider("cross")');
assert(sb12.evalInContext('currentAIProvider') === 'cross', 'Final state successfully reset to "cross"');
assert(doc12.getElementById('view-section-cross-llm').style.display === 'flex', 'Cross section visible after fuzz test');
assert(doc12.getElementById('view-section-overview').style.display === 'none', 'Overview section hidden after fuzz test');
assert(doc12.getElementById('btn-provider-cross').classList.contains('active'), '#btn-provider-cross is active after fuzz test');

// -------------------------------------------------------------
// SUMMARY & VERDICT
// -------------------------------------------------------------
console.log('\n================================================================');
console.log(`TOTAL CHECKS: ${totalTests}`);
console.log(`PASSED:       ${passedTests}`);
console.log(`FAILED:       ${failedTests}`);
console.log('================================================================');

if (failedTests === 0) {
  console.log('\n🏆 VERDICT: ALL CHALLENGES PASSED! EMPIRICAL VERDICT: APPROVE');
  process.exit(0);
} else {
  console.error(`\n💥 VERDICT: ${failedTests} CHALLENGES FAILED! EMPIRICAL VERDICT: REJECT`);
  process.exit(1);
}

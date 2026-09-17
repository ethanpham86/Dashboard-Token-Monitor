const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const html = fs.readFileSync('web/static/index.html', 'utf8');
for (const match of html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/gi)) {
  if (match[1].trim()) new vm.Script(match[1]);
}
function extract(start, end) {
  const a = html.indexOf(start);
  const b = html.indexOf(end, a + start.length);
  assert.ok(a >= 0 && b > a);
  return html.slice(a, b);
}
const elements = new Map();
const ctx = {
  currentAIProvider: 'codex', currentTab: 'token', chartInstance: null,
  cachedChartPoints: [], cachedModelTimeSeries: [],
  document: {getElementById(id) {
    if (!elements.has(id)) elements.set(id, {textContent:'', getContext: () => ({})});
    return elements.get(id);
  }},
  setOpenAIText(id, value) { ctx.document.getElementById(id).textContent = value; },
  setElementHint() {}, formatNumber: String, formatOpenAIWindow: n => `${n} min`,
  Chart: function (_, config) { this.config = config; this.destroy = () => {}; }
};
vm.createContext(ctx);
vm.runInContext(extract('    function updateCodexHeader(', '    function updateClaudeHeader('), ctx);
ctx.updateCodexHeader({sessions: [], models: []}, null, {});
assert.match(elements.get('txt-plan-name').textContent, /UNKNOWN/);
assert.match(elements.get('txt-quota').textContent, /Chưa có/);
assert.doesNotMatch(elements.get('txt-token-status').textContent, /100%/);
ctx.updateCodexHeader({sessions: [], models: []}, {plan_type:'pro', primary:{used_percent:25, window_minutes:15}, stale:true}, {});
assert.match(elements.get('txt-quota').textContent, /15 min/);
assert.match(elements.get('txt-quota').textContent, /cũ/);
ctx.updateCodexHeader({sessions: [], models: []}, null, {});
assert.match(elements.get('txt-quota').textContent, /Chưa có/);

vm.runInContext(extract('    function renderChart(pts)', '    async function simulateCall()'), ctx);
ctx.renderChart([{time_bucket:'2026-09-15', prompt_tokens:100, cached_tokens:40, output_tokens:30, thinking_tokens:10, model_calls:1}]);
const tokenStack = ctx.chartInstance.config.data.datasets.slice(0,4).reduce((n,d) => n+d.data[0],0);
assert.equal(tokenStack,130, 'cached/reasoning must not inflate the stacked total');
ctx.currentTab = 'model';
ctx.cachedModelTimeSeries = [
  {time_bucket:'2026-09-15', model_name:'model-a',total_tokens:100},
  {time_bucket:'2026-09-15', model_name:'model-b',total_tokens:30}
];
ctx.renderChart([{time_bucket:'2026-09-15'}]);
assert.equal(ctx.chartInstance.config.data.datasets[0].label,'model-a');
assert.equal(ctx.chartInstance.config.data.datasets[0].data[0],100);
assert.equal(ctx.chartInstance.config.data.datasets[1].data[0],30);
console.log('Codex UI audit: syntax, unknown/stale metadata, token stacks and model attribution passed.');

async function checkCodexRequests() {
  const requests = [];
  ctx.currentTimeRange = 'all'; ctx.currentMainView = 'overview';
  ctx.AbortController = AbortController;
  ctx.console = {error() {}};
  ctx.fetch = (url, options) => new Promise(resolve => requests.push({url, options, resolve}));
  for (const name of ['renderChart','renderEChartsDonut','renderModelChips','updateModelDropdown','renderEChartsTimeline','renderProviderDailyTable','renderOpenAILimit','renderOpenAISessions','loadAgentFleetData']) ctx[name] = () => {};
  vm.runInContext(extract('    let codexRequest = null;', '    async function refreshOpenAIData('), ctx);
  const old = ctx.loadOpenAIData();
  assert.equal(ctx.loadOpenAIData(), old, 'overlapping polls should share one request');
  assert.equal(requests.length, 1);
  ctx.currentTimeRange = '7d'; ctx.currentMainView = 'agents';
  const current = ctx.loadOpenAIData();
  assert.equal(requests[0].options.signal.aborted, true);
  assert.match(requests[1].url, /range=7d&include_graph=1/);
  requests[1].resolve({ok:true,json:async()=>({summary:{total_tokens:77}, sessions:[], models:[], time_series:[], model_time_series:[]})});
  await current;
  requests[0].resolve({ok:true,json:async()=>({summary:{total_tokens:999}})});
  await old;
  assert.equal(elements.get('num-grand-tokens').textContent,'77','late response overwrote the selected range');
  ctx.currentTimeRange = '24h';
  const offline=ctx.loadOpenAIData(); requests[2].resolve({ok:false,status:503}); await offline;
  assert.match(elements.get('txt-plan-badge').textContent,/OFFLINE/);
  console.log('Codex requests: coalescing, cancellation, bundled graph, stale response and failure state passed.');
}
checkCodexRequests().catch(error => { console.error(error); process.exitCode=1; });

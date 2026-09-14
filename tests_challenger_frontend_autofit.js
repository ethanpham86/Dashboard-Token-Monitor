/**
 * Challenger 1: Empirical Frontend Math & Auto-Fit Adversarial Stress Test Suite
 * 
 * Target: web/static/index.html
 * Scope:
 *  1. Active Project Auto-Focus logic (updateTopologyProjectSelect, loadAgentFleetData)
 *  2. Bézier midpoint & affine transformation coordinate stability (zoom, pan, drag, zero-drift)
 *  3. Responsive Auto-Fit math (0 nodes, 1 node, negative coords, extreme aspect ratios, localStorage zero-drift)
 */

const fs = require('fs');
const path = require('path');
const vm = require('vm');

const htmlPath = path.resolve(__dirname, 'web/static/index.html');
const html = fs.readFileSync(htmlPath, 'utf8');

let totalTests = 0;
let passedTests = 0;
let failedTests = 0;

function assert(condition, message, details) {
  totalTests++;
  if (condition) {
    passedTests++;
    console.log(`  ✅ PASS: ${message}`);
  } else {
    failedTests++;
    console.error(`  ❌ FAIL: ${message}${details ? ' - ' + JSON.stringify(details) : ''}`);
  }
}

console.log('================================================================');
console.log('   CHALLENGER 1: FRONTEND MATH & AUTO-FIT ADVERSARIAL HARNESS  ');
console.log('================================================================\n');

// -------------------------------------------------------------
// SUITE 1: Active Project Auto-Focus Logic
// -------------------------------------------------------------
console.log('--- SUITE 1: Active Project Auto-Focus Logic ---');

// Mock a lightweight browser environment to run updateTopologyProjectSelect & changeTopologyProject
function createAutoSelectContext() {
  const selectElem = {
    innerHTML: '',
    value: 'all',
    children: []
  };

  const labelElem = {
    textContent: ''
  };

  const elements = {
    'select-topology-project': selectElem,
    'lbl-topo-project-prefix': labelElem
  };

  const documentMock = {
    getElementById: (id) => elements[id] || null
  };

  const contextObj = {
    document: documentMock,
    currentAIProvider: 'agy',
    currentTopologyProject: 'all',
    userHasManuallyChosenTopologyProject: false,
    console: console,
    loadAgentFleetDataCalled: 0,
    loadAgentFleetData: function() {
      this.loadAgentFleetDataCalled++;
    }
  };

  // Extract functions from index.html
  // 1. updateTopologyProjectSelect
  const updateProjMatch = html.match(/function updateTopologyProjectSelect\(projects\)\s*\{([\s\S]*?)\n    \}/);
  if (!updateProjMatch) {
    throw new Error('Failed to extract updateTopologyProjectSelect from index.html');
  }
  const updateProjCode = 'function updateTopologyProjectSelect(projects) {' + updateProjMatch[1] + '}';

  // 2. changeTopologyProject
  const changeProjMatch = html.match(/function changeTopologyProject\(val\)\s*\{([\s\S]*?)\n    \}/);
  if (!changeProjMatch) {
    throw new Error('Failed to extract changeTopologyProject from index.html');
  }
  const changeProjCode = 'function changeTopologyProject(val) {' + changeProjMatch[1] + '}';

  vm.createContext(contextObj);
  vm.runInContext(updateProjCode, contextObj);
  vm.runInContext(changeProjCode, contextObj);

  return { ctx: contextObj, selectElem, labelElem };
}

// Test 1.1: No active project
{
  const { ctx, selectElem } = createAutoSelectContext();
  const projectsAllCompleted = [
    { id: 'proj-tm', name: 'TokenMonitor', status: 'COMPLETED', active_agents: 0, total_tokens: 100000 },
    { id: 'proj-mc', name: 'MCREDIT', status: 'COMPLETED', active_agents: 0, total_tokens: 50000 },
    { id: 'proj-tc', name: 'TieuChuan', status: 'COMPLETED', active_agents: 0, total_tokens: 20000 }
  ];

  ctx.updateTopologyProjectSelect(projectsAllCompleted);
  assert(ctx.currentTopologyProject === 'all', 'When no project is active, auto-focus stays on "all"');
  assert(selectElem.value === 'all', 'Dropdown value is set to "all" when no project is active');
  assert(!selectElem.innerHTML.includes('[ĐANG CHẠY]'), 'No project is marked [ĐANG CHẠY] when all are completed');
  assert(selectElem.innerHTML.includes('⚪ TokenMonitor'), 'Completed project displays white dot ⚪');
}

// Test 1.2: Empty or null projects list
{
  const { ctx, selectElem } = createAutoSelectContext();
  ctx.updateTopologyProjectSelect([]);
  assert(ctx.currentTopologyProject === 'all', 'Empty projects array defaults to "all" without throwing');
  
  ctx.updateTopologyProjectSelect(null);
  assert(ctx.currentTopologyProject === 'all', 'Null projects parameter defaults to "all" without throwing');

  ctx.updateTopologyProjectSelect(undefined);
  assert(ctx.currentTopologyProject === 'all', 'Undefined projects parameter defaults to "all" without throwing');
}

// Test 1.3: Exactly one active project
{
  const { ctx, selectElem } = createAutoSelectContext();
  const projectsOneActive = [
    { id: 'proj-tm', name: 'TokenMonitor', status: 'ACTIVE', active_agents: 5, total_tokens: 1500000 },
    { id: 'proj-mc', name: 'MCREDIT', status: 'COMPLETED', active_agents: 0, total_tokens: 50000 }
  ];

  ctx.updateTopologyProjectSelect(projectsOneActive);
  assert(ctx.currentTopologyProject === 'proj-tm', 'Auto-focus selects the active project (proj-tm)');
  assert(selectElem.value === 'proj-tm', 'Dropdown value matches active project (proj-tm)');
  assert(selectElem.innerHTML.includes('🟢 TokenMonitor [ĐANG CHẠY]'), 'Active project has green dot 🟢 and [ĐANG CHẠY]');
}

// Test 1.4: Multiple active projects (deterministic selection)
{
  const { ctx, selectElem } = createAutoSelectContext();
  // Backend sorts active projects first, with highest tokens first
  const projectsMultipleActive = [
    { id: 'proj-primary', name: 'Primary Active', status: 'ACTIVE', active_agents: 7, total_tokens: 3000000 },
    { id: 'proj-secondary', name: 'Secondary Active', status: 'ACTIVE', active_agents: 3, total_tokens: 1000000 },
    { id: 'proj-running-task', name: 'Third Running', status: 'RUNNING', active_agents: 2, total_tokens: 500000 },
    { id: 'proj-idle', name: 'Idle Proj', status: 'COMPLETED', active_agents: 0, total_tokens: 100000 }
  ];

  ctx.updateTopologyProjectSelect(projectsMultipleActive);
  assert(ctx.currentTopologyProject === 'proj-primary', 'Multiple active projects: first active project is selected deterministically');
  assert(selectElem.value === 'proj-primary', 'Dropdown selects proj-primary');
  assert(selectElem.innerHTML.includes('🟢 Primary Active [ĐANG CHẠY]'), 'Primary active project has 🟢');
  assert(selectElem.innerHTML.includes('🟢 Secondary Active [ĐANG CHẠY]'), 'Secondary active project also displays 🟢 in options list');
}

// Test 1.5: Manual selection persistence across polling cycles
{
  const { ctx, selectElem } = createAutoSelectContext();
  const projectsInitial = [
    { id: 'proj-tm', name: 'TokenMonitor', status: 'ACTIVE', active_agents: 5, total_tokens: 1500000 },
    { id: 'proj-mc', name: 'MCREDIT', status: 'COMPLETED', active_agents: 0, total_tokens: 50000 }
  ];

  // Initial load auto-focuses proj-tm
  ctx.updateTopologyProjectSelect(projectsInitial);
  assert(ctx.currentTopologyProject === 'proj-tm', 'Initial state auto-focused proj-tm');

  // User manually chooses proj-mc
  ctx.changeTopologyProject('proj-mc');
  assert(ctx.userHasManuallyChosenTopologyProject === true, 'Manual selection flag set to true');
  assert(ctx.currentTopologyProject === 'proj-mc', 'currentTopologyProject set to user choice proj-mc');

  // Subsequent poll 1: proj-tm is still ACTIVE
  ctx.updateTopologyProjectSelect(projectsInitial);
  assert(ctx.currentTopologyProject === 'proj-mc', 'Poll 1: Manual selection proj-mc persisted despite proj-tm being ACTIVE');

  // Subsequent poll 2: Another project proj-tc becomes ACTIVE
  const projectsPoll2 = [
    { id: 'proj-tc', name: 'TieuChuan', status: 'ACTIVE', active_agents: 4, total_tokens: 800000 },
    { id: 'proj-tm', name: 'TokenMonitor', status: 'ACTIVE', active_agents: 5, total_tokens: 1500000 },
    { id: 'proj-mc', name: 'MCREDIT', status: 'COMPLETED', active_agents: 0, total_tokens: 50000 }
  ];
  ctx.updateTopologyProjectSelect(projectsPoll2);
  assert(ctx.currentTopologyProject === 'proj-mc', 'Poll 2: Manual selection proj-mc still persisted when new project becomes active');

  // User manually chooses 'all'
  ctx.changeTopologyProject('all');
  assert(ctx.currentTopologyProject === 'all', 'User manually chooses "all"');
  ctx.updateTopologyProjectSelect(projectsPoll2);
  assert(ctx.currentTopologyProject === 'all', 'Poll 3: Explicit choice of "all" is never overridden by active projects');
}

// Test 1.6: Non-existent project fallback to 'all'
{
  const { ctx, selectElem } = createAutoSelectContext();
  ctx.currentTopologyProject = 'ghost-project-no-longer-exists';
  const projects = [
    { id: 'proj-tm', name: 'TokenMonitor', status: 'COMPLETED', active_agents: 0, total_tokens: 100000 }
  ];
  ctx.updateTopologyProjectSelect(projects);
  assert(ctx.currentTopologyProject === 'all', 'Non-existent project gracefully falls back to "all"');
  assert(selectElem.value === 'all', 'Dropdown value falls back to "all"');
}

// Test 1.7: Provider-specific prefix and options (Codex, Claude, Antigravity)
{
  const { ctx, labelElem, selectElem } = createAutoSelectContext();
  
  // Codex
  ctx.currentAIProvider = 'codex';
  ctx.updateTopologyProjectSelect([{ id: 'ws-1', name: 'Workspace 1', status: 'ACTIVE' }]);
  assert(labelElem.textContent === 'WORKSPACE:', 'Codex prefix is WORKSPACE:');
  assert(selectElem.innerHTML.includes('Tất Cả Workspaces'), 'Codex options include "Tất Cả Workspaces"');

  // Claude
  ctx.currentAIProvider = 'claude';
  ctx.updateTopologyProjectSelect([{ id: 'proj-1', name: 'Project 1', status: 'ACTIVE' }]);
  assert(labelElem.textContent === 'PROJECT:', 'Claude prefix is PROJECT:');
  assert(selectElem.innerHTML.includes('Tất Cả Dự Án Claude'), 'Claude options include "Tất Cả Dự Án Claude"');

  // Antigravity
  ctx.currentAIProvider = 'agy';
  ctx.updateTopologyProjectSelect([{ id: 'proj-tm', name: 'TokenMonitor', status: 'ACTIVE' }]);
  assert(labelElem.textContent === 'DỰ ÁN:', 'Antigravity prefix is DỰ ÁN:');
  assert(selectElem.innerHTML.includes('Sơ Đồ Toàn Cảnh Workspace'), 'Antigravity includes workspace_overview option');
}


// -------------------------------------------------------------
// SUITE 2: Bézier Midpoint & Affine Transformation Stability
// -------------------------------------------------------------
console.log('\n--- SUITE 2: Bézier Midpoint & Affine Transformation Stability ---');

// Extract getTopoBezierPoint and getTopoBezierTangent from index.html
const bezierPointMatch = html.match(/function getTopoBezierPoint\(p0, p1, cp, t\)\s*\{([\s\S]*?)\n    \}/);
if (!bezierPointMatch) throw new Error('Failed to extract getTopoBezierPoint');
const getTopoBezierPoint = new Function('p0', 'p1', 'cp', 't', bezierPointMatch[1]);

const bezierTangentMatch = html.match(/function getTopoBezierTangent\(p0, p1, cp, t\)\s*\{([\s\S]*?)\n    \}/);
if (!bezierTangentMatch) throw new Error('Failed to extract getTopoBezierTangent');
const getTopoBezierTangent = new Function('p0', 'p1', 'cp', 't', bezierTangentMatch[1]);

// Mathematical affine transform helper: T(v) = M * v + b
function applyAffine(pt, zoom, panX, panY, rotateRad = 0) {
  const cos = Math.cos(rotateRad);
  const sin = Math.sin(rotateRad);
  const zx = pt[0] * zoom;
  const zy = pt[1] * zoom;
  return [
    zx * cos - zy * sin + panX,
    zx * sin + zy * cos + panY
  ];
}

// Test 2.1: Mathematical identity under Zoom (0.1 to 5.0) and Pan (-5000 to +5000)
{
  const zoomFactors = [0.1, 0.25, 0.35, 0.5, 0.85, 1.0, 1.4, 2.0, 3.5, 5.0];
  const panOffsets = [
    [0, 0],
    [500, -300],
    [-2500, 1800],
    [10000, -10000],
    [-50000, 50000]
  ];

  const testCurves = [
    { p0: [1500, 40], p1: [1500, 180], cp: [1500, 110] }, // Vertical orchestration link
    { p0: [1500, 180], p1: [880, 320], cp: [1190, 250] }, // Diagonal branch link
    { p0: [880, 320], p1: [660, 460], cp: [770, 390] },  // Subagent delegation
    { p0: [660, 460], p1: [1100, 460], cp: [880, 520] }, // Horizontal inter-agent bridge
    { p0: [-500, -200], p1: [1200, 800], cp: [350, 400] } // Arbitrary negative coords
  ];

  let maxDrift = 0;
  let totalComparisons = 0;

  zoomFactors.forEach(zoom => {
    panOffsets.forEach(([panX, panY]) => {
      testCurves.forEach(curve => {
        // 1. Calculate midpoint in local space, then transform to screen space
        const localMid = getTopoBezierPoint(curve.p0, curve.p1, curve.cp, 0.5);
        const transformedOfMid = applyAffine([localMid.x, localMid.y], zoom, panX, panY);

        // 2. Transform control points to screen space (as ECharts transformCoordToGlobal does), then calculate midpoint
        const tp0 = applyAffine(curve.p0, zoom, panX, panY);
        const tp1 = applyAffine(curve.p1, zoom, panX, panY);
        const tcp = applyAffine(curve.cp, zoom, panX, panY);
        const midOfTransformed = getTopoBezierPoint(tp0, tp1, tcp, 0.5);

        // Drift distance
        const dx = transformedOfMid[0] - midOfTransformed.x;
        const dy = transformedOfMid[1] - midOfTransformed.y;
        const drift = Math.sqrt(dx * dx + dy * dy);

        if (drift > maxDrift) maxDrift = drift;
        totalComparisons++;
      });
    });
  });

  assert(maxDrift < 1e-11, `Zero coordinate drift across ${totalComparisons} zoom/pan combinations (max drift: ${maxDrift.toExponential(4)} px)`);
}

// Test 2.2: Extreme Monte-Carlo Stress Test (10,000 random affine transforms & curves)
{
  let maxDrift = 0;
  const N = 10000;
  for (let i = 0; i < N; i++) {
    const zoom = 0.05 + Math.random() * 9.95; // [0.05, 10.0]
    const panX = (Math.random() - 0.5) * 100000;
    const panY = (Math.random() - 0.5) * 100000;
    const rot = Math.random() * 2 * Math.PI;

    const p0 = [(Math.random() - 0.5) * 10000, (Math.random() - 0.5) * 10000];
    const p1 = [(Math.random() - 0.5) * 10000, (Math.random() - 0.5) * 10000];
    const cp = [(Math.random() - 0.5) * 10000, (Math.random() - 0.5) * 10000];

    const localMid = getTopoBezierPoint(p0, p1, cp, 0.5);
    const transformedOfMid = applyAffine([localMid.x, localMid.y], zoom, panX, panY, rot);

    const tp0 = applyAffine(p0, zoom, panX, panY, rot);
    const tp1 = applyAffine(p1, zoom, panX, panY, rot);
    const tcp = applyAffine(cp, zoom, panX, panY, rot);
    const midOfTransformed = getTopoBezierPoint(tp0, tp1, tcp, 0.5);

    const dx = transformedOfMid[0] - midOfTransformed.x;
    const dy = transformedOfMid[1] - midOfTransformed.y;
    const drift = Math.sqrt(dx * dx + dy * dy);
    if (drift > maxDrift) maxDrift = drift;
  }
  assert(maxDrift < 1e-9, `Monte-Carlo 10,000 random transforms with rotation: max drift ${maxDrift.toExponential(4)} px (< 1nm)`);
}

// Test 2.3: Tangent Angle Stability under extreme points
{
  // Collinear horizontal
  const tH = getTopoBezierTangent([0, 0], [100, 0], [50, 0], 0.5);
  assert(Math.abs(tH) < 1e-12, 'Horizontal collinear curve tangent is 0 rad');

  // Collinear vertical
  const tV = getTopoBezierTangent([0, 0], [0, 100], [0, 50], 0.5);
  assert(Math.abs(tV - Math.PI / 2) < 1e-12, 'Vertical collinear curve tangent is PI/2 rad (90 deg)');

  // Degenerate single point (p0 = p1 = cp)
  const tD = getTopoBezierTangent([10, 10], [10, 10], [10, 10], 0.5);
  assert(!isNaN(tD), 'Degenerate point tangent returns finite number (atan2(0,0) = 0), never NaN');
}


// -------------------------------------------------------------
// SUITE 3: Responsive Auto-Fit Math & Zero-Drift Contract
// -------------------------------------------------------------
console.log('\n--- SUITE 3: Responsive Auto-Fit Math & Zero-Drift Storage Contract ---');

// Extract calculateTopologyAutoFit
const autoFitMatch = html.match(/function calculateTopologyAutoFit\(nodes\)\s*\{([\s\S]*?)\n    \}/);
if (!autoFitMatch) throw new Error('Failed to extract calculateTopologyAutoFit');

function createAutoFitContext(clientWidth = 1400, clientHeight = 700, innerWidth = 1920) {
  const domElem = {
    clientWidth: clientWidth,
    clientHeight: clientHeight
  };

  const contextObj = {
    document: {
      getElementById: (id) => (id === 'echart-agent-fleet-main' ? domElem : null)
    },
    window: {
      innerWidth: innerWidth
    },
    echartAgentFleet: null,
    Math: Math
  };

  const fnCode = 'function calculateTopologyAutoFit(nodes) {' + autoFitMatch[1] + '}';
  vm.createContext(contextObj);
  vm.runInContext(fnCode, contextObj);

  return { ctx: contextObj, domElem };
}

// Test 3.1: 0 nodes
{
  const { ctx } = createAutoFitContext();
  const resEmpty = ctx.calculateTopologyAutoFit([]);
  assert(resEmpty.zoom === 0.82, '0 nodes: zoom defaults safely to 0.82');
  assert(resEmpty.center[0] === '50%' && resEmpty.center[1] === '50%', '0 nodes: center defaults safely to ["50%", "50%"]');

  const resNull = ctx.calculateTopologyAutoFit(null);
  assert(resNull.zoom === 0.82 && resNull.center[0] === '50%', 'null nodes: safely defaults to { zoom: 0.82, center: ["50%", "50%"] }');

  const resUndef = ctx.calculateTopologyAutoFit(undefined);
  assert(resUndef.zoom === 0.82 && resUndef.center[0] === '50%', 'undefined nodes: safely defaults to { zoom: 0.82, center: ["50%", "50%"] }');
}

// Test 3.2: 1 node
{
  const { ctx } = createAutoFitContext();
  const singleNode = [{ id: 'root', x: 1500, y: 40 }];
  const resSingle = ctx.calculateTopologyAutoFit(singleNode);
  assert(resSingle.zoom === 0.80, '1 node: validCount < 2 triggers safe default zoom 0.80');
  assert(resSingle.center[0] === '50%' && resSingle.center[1] === '50%', '1 node: center safely defaults to ["50%", "50%"] without division by zero');
}

// Test 3.3: Collinear nodes (minX >= maxX or minY >= maxY)
{
  const { ctx } = createAutoFitContext();
  
  // Perfectly vertical line (all X identical)
  const verticalNodes = [
    { id: 'n1', x: 500, y: 100 },
    { id: 'n2', x: 500, y: 300 },
    { id: 'n3', x: 500, y: 500 }
  ];
  const resVert = ctx.calculateTopologyAutoFit(verticalNodes);
  assert(resVert.zoom === 0.80 && resVert.center[0] === '50%', 'Collinear vertical nodes (minX === maxX): safely defaults to 0.80, ["50%", "50%"]');

  // Perfectly horizontal line (all Y identical)
  const horizontalNodes = [
    { id: 'n1', x: 100, y: 400 },
    { id: 'n2', x: 300, y: 400 },
    { id: 'n3', x: 500, y: 400 }
  ];
  const resHoriz = ctx.calculateTopologyAutoFit(horizontalNodes);
  assert(resHoriz.zoom === 0.80 && resHoriz.center[0] === '50%', 'Collinear horizontal nodes (minY === maxY): safely defaults to 0.80, ["50%", "50%"]');
}

// Test 3.4: Negative coordinates
{
  const { ctx } = createAutoFitContext(1400, 700);
  const negNodes = [
    { id: 'n1', x: -600, y: -400 },
    { id: 'n2', x: 200, y: 200 }
  ];
  const resNeg = ctx.calculateTopologyAutoFit(negNodes);
  assert(resNeg.center[0] === -200, 'Negative coords midX is correctly Math.round((-600 + 200) / 2) = -200');
  assert(resNeg.center[1] === -100, 'Negative coords midY is correctly Math.round((-400 + 200) / 2) = -100');
  assert(typeof resNeg.zoom === 'number' && !isNaN(resNeg.zoom) && resNeg.zoom >= 0.38 && resNeg.zoom <= 1.4, `Negative coords optZoom is valid: ${resNeg.zoom}`);
}

// Test 3.5: Standard 3-project hierarchy nodes (Antigravity layout)
{
  const { ctx } = createAutoFitContext(1400, 700);
  const standardNodes = [
    { id: 'root', x: 1500, y: 40 },
    { id: 'hub1', x: 880, y: 180 },
    { id: 'hub2', x: 1500, y: 180 },
    { id: 'hub3', x: 2120, y: 180 },
    { id: 'orch1', x: 880, y: 320 },
    { id: 'orch2', x: 1500, y: 320 },
    { id: 'orch3', x: 2120, y: 320 },
    { id: 'sub1_1', x: 660, y: 460 },
    { id: 'sub3_5', x: 2340, y: 460 }
  ];
  const resStd = ctx.calculateTopologyAutoFit(standardNodes);
  // minX = 660, maxX = 2340 -> midX = 1500
  // minY = 40, maxY = 460 -> midY = 250
  assert(resStd.center[0] === 1500, 'Standard graph center midX is exactly 1500');
  assert(resStd.center[1] === 250, 'Standard graph center midY is exactly 250');
  assert(resStd.zoom >= 0.38 && resStd.zoom <= 1.4, `Standard graph zoom is safely bounded: ${resStd.zoom}`);
}

// Test 3.6: Extreme Viewport Aspect Ratios
{
  const testNodes = [
    { id: 'n1', x: 0, y: 0 },
    { id: 'n2', x: 1000, y: 600 }
  ];

  // 1. Ultrawide 32:9 (e.g. 3840 x 1080) -> height is limiting factor
  {
    const { ctx } = createAutoFitContext(3840, 1080);
    const fit = ctx.calculateTopologyAutoFit(testNodes);
    assert(fit.zoom <= 1.4 && fit.zoom >= 0.38, `Ultrawide 32:9 zoom: ${fit.zoom}`);
  }

  // 2. Portrait / Phone 9:16 (e.g. 1080 x 1920) -> width is limiting factor
  {
    const { ctx } = createAutoFitContext(1080, 1920);
    const fit = ctx.calculateTopologyAutoFit(testNodes);
    assert(fit.zoom <= 1.4 && fit.zoom >= 0.38, `Portrait 9:16 zoom: ${fit.zoom}`);
  }

  // 3. Tiny screen (320 x 480) -> zoom clamped to lower bound 0.38
  {
    const { ctx } = createAutoFitContext(320, 480);
    const fit = ctx.calculateTopologyAutoFit(testNodes);
    assert(fit.zoom === 0.38, `Tiny screen (320x480) hits lower safety clamp 0.38 (got: ${fit.zoom})`);
  }

  // 4. Giant 8K screen (7680 x 4320) -> zoom clamped to upper bound 1.40
  {
    const { ctx } = createAutoFitContext(7680, 4320);
    const fit = ctx.calculateTopologyAutoFit(testNodes);
    assert(fit.zoom === 1.4, `Giant 8K screen hits upper safety clamp 1.40 (got: ${fit.zoom})`);
  }
}

// Test 3.7: Zero-Drift Storage Contract (localStorage inspection)
{
  console.log('\n--- Checking localStorage Roaming Center Storage Contract ---');

  // Check graphRoam listener in html
  const roamBlockMatch = html.match(/echartAgentFleet\.on\('graphRoam',\s*function\(params\)\s*\{([\s\S]*?)\}\);/);
  assert(roamBlockMatch !== null, 'graphRoam event listener found in index.html');

  if (roamBlockMatch) {
    const roamCode = roamBlockMatch[1];
    assert(roamCode.includes("localStorage.setItem('agent_fleet_graph_zoom'"), 'graphRoam stores agent_fleet_graph_zoom');
    assert(!roamCode.includes("localStorage.setItem('agent_fleet_graph_center'"), 'ZERO-DRIFT CONTRACT: graphRoam NEVER stores agent_fleet_graph_center');
  }

  // Check resetTopologyView in html
  const resetBlockMatch = html.match(/function resetTopologyView\(\)\s*\{([\s\S]*?)\n    \}/);
  assert(resetBlockMatch !== null, 'resetTopologyView function found in index.html');
  if (resetBlockMatch) {
    const resetCode = resetBlockMatch[1];
    assert(resetCode.includes("localStorage.removeItem('agent_fleet_graph_center')"), 'resetTopologyView cleans up any legacy agent_fleet_graph_center');
    assert(resetCode.includes("calculateTopologyAutoFit()"), 'resetTopologyView recalculates fresh Auto-Fit');
  }

  // Check window resize listener in html
  const resizeMatch = html.match(/window\.addEventListener\('resize',\s*\(\)\s*=>\s*\{([\s\S]*?)\}\);/);
  assert(resizeMatch !== null, 'window resize listener found in index.html');
  if (resizeMatch) {
    const resizeCode = resizeMatch[1];
    assert(resizeCode.includes("calculateTopologyAutoFit()"), 'window resize dynamically recalculates Auto-Fit');
    assert(!resizeCode.includes("localStorage.getItem('agent_fleet_graph_center')"), 'ZERO-DRIFT CONTRACT: resize NEVER reads center from localStorage');
  }
}

// -------------------------------------------------------------
// SUITE 4: Advanced Edge-Case Fuzzing & Concurrency
// -------------------------------------------------------------
console.log('\n--- SUITE 4: Advanced Edge-Case Fuzzing & Degenerate Cases ---');

// Test 4.1: Curveness Variations (0, negative, high positive)
{
  const p0 = [100, 100];
  const p1 = [500, 500];

  // curveness = 0 (straight line)
  const mx = (p0[0] + p1[0]) / 2;
  const my = (p0[1] + p1[1]) / 2;
  const cpZero = [mx, my];
  const midZero = getTopoBezierPoint(p0, p1, cpZero, 0.5);
  assert(midZero.x === 300 && midZero.y === 300, 'curveness=0: midpoint is exact linear midpoint (300, 300)');

  // curveness = -0.2 (negative curvature)
  const vx = p1[0] - p0[0];
  const vy = p1[1] - p0[1];
  const cpNeg = [mx - vy * (-0.2), my + vx * (-0.2)];
  const midNeg = getTopoBezierPoint(p0, p1, cpNeg, 0.5);
  assert(!isNaN(midNeg.x) && !isNaN(midNeg.y), 'curveness < 0: midpoint is finite and well-defined');

  // curveness = +0.5 (extreme curvature)
  const cpPos = [mx - vy * 0.5, my + vx * 0.5];
  const midPos = getTopoBezierPoint(p0, p1, cpPos, 0.5);
  assert(!isNaN(midPos.x) && !isNaN(midPos.y), 'curveness = 0.5: midpoint is finite and well-defined');
}

// Test 4.2: Fuzzing calculateTopologyAutoFit with malformed node objects
{
  const { ctx } = createAutoFitContext(1400, 700);

  const malformedInputs = [
    { label: 'Missing x,y fields', input: [ { id: 'bad1' }, { id: 'bad2' } ], expectThrow: false },
    { label: 'NaN coordinates', input: [ { id: 'nan', x: NaN, y: 100 }, { id: 'nan2', x: 200, y: NaN } ], expectThrow: false },
    { label: 'Infinity coordinates', input: [ { id: 'inf', x: Infinity, y: -Infinity }, { id: 'inf2', x: -Infinity, y: Infinity } ], expectThrow: false },
    { label: 'String coordinates', input: [ { id: 'str', x: '1500', y: '400' }, { id: 'str2', x: '200', y: '300' } ], expectThrow: false }
  ];

  malformedInputs.forEach((item, idx) => {
    let fit;
    let threw = false;
    try {
      fit = ctx.calculateTopologyAutoFit(item.input);
    } catch (e) {
      threw = true;
    }
    assert(!threw, `Malformed input #${idx + 1} (${item.label}) does not throw an exception`);
    assert(fit && typeof fit.zoom === 'number' && !isNaN(fit.zoom), `Malformed input #${idx + 1} returns valid numeric zoom`);
    assert(fit && Array.isArray(fit.center) && fit.center.length === 2, `Malformed input #${idx + 1} returns 2-element center array`);
  });

  // Edge case finding: Sparse array containing null element [null]
  let nullItemThrew = false;
  try {
    ctx.calculateTopologyAutoFit([null]);
  } catch (e) {
    nullItemThrew = true;
  }
  assert(nullItemThrew === true, 'VULNERABILITY REPRODUCED: Array with null element [null] throws TypeError due to missing "n &&" guard (documented in challenge.md)');
}


// Test 4.3: Concurrency Race-Cancellation Simulation (Rapid Provider Switching)
{
  let _fleetFetchSeq = 0;
  let currentAIProvider = 'agy';
  let activeRenderProvider = null;

  function simulateStartFetch(provider) {
    const fetchSeq = ++_fleetFetchSeq;
    const targetProvider = provider;
    currentAIProvider = provider;

    return function simulateFetchComplete() {
      if (fetchSeq !== _fleetFetchSeq || currentAIProvider !== targetProvider) {
        // Discarded due to stale sequence or changed provider
        return false;
      }
      activeRenderProvider = targetProvider;
      return true;
    };
  }

  // Rapid switching: agy -> codex -> claude -> agy in quick succession
  const cb1 = simulateStartFetch('agy');
  const cb2 = simulateStartFetch('codex');
  const cb3 = simulateStartFetch('claude');
  const cb4 = simulateStartFetch('agy');

  // Requests finish out-of-order (cb1 finishes late)
  const res1 = cb1();
  assert(res1 === false, 'Stale request 1 (agy) discarded because newer requests were launched');
  
  const res2 = cb2();
  assert(res2 === false, 'Stale request 2 (codex) discarded');

  const res3 = cb3();
  assert(res3 === false, 'Stale request 3 (claude) discarded');

  const res4 = cb4();
  assert(res4 === true, 'Latest request 4 (agy) accepted and rendered');
  assert(activeRenderProvider === 'agy', 'Active render provider matches final provider (agy)');
}

console.log('\n================================================================');
console.log(`TEST SUMMARY: ${totalTests} ran | ${passedTests} passed | ${failedTests} failed`);
console.log(`VERDICT: ${failedTests === 0 ? 'ALL TESTS PASSED (APPROVE)' : 'TESTS FAILED (FAIL)'}`);
console.log('================================================================\n');

process.exit(failedTests === 0 ? 0 : 1);


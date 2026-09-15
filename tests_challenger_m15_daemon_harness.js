/**
 * Challenger M15-1: Live Daemon Adversarial Stress Test & Fuzzing Harness
 * Target: http://127.0.0.1:9090
 * 
 * Scope:
 *  1. Core Daemon Endpoints Live Verification (healthz, account, graph, leaderboard, docs)
 *  2. Query Parameter Fuzzing & Boundaries (range & sort combinations)
 *  3. Injection Attack Resilience (SQLi, XSS Defense, Path Traversal Defense)
 *  4. HTTP Method Tampering (POST, PUT, DELETE, PATCH on GET-only routes)
 *  5. Concurrent Burst Stress Test (50 simultaneous requests)
 *  6. Error Handling Contract (Zero 500 Internal Server Errors)
 */

const http = require('http');
const { URL } = require('url');

const BASE_URL = 'http://127.0.0.1:9090';

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

function request(options, postData = null) {
  return new Promise((resolve, reject) => {
    const u = new URL(options.path || '/', BASE_URL);
    const headers = Object.assign({}, options.headers || {});

    if (postData) {
      if (!headers['Content-Type']) {
        headers['Content-Type'] = 'application/json';
      }
      headers['Content-Length'] = Buffer.byteLength(postData);
    }

    const reqOptions = {
      hostname: u.hostname,
      port: u.port,
      path: u.pathname + u.search,
      method: options.method || 'GET',
      headers: headers
    };

    const req = http.request(reqOptions, (res) => {
      let body = '';
      res.on('data', chunk => { body += chunk; });
      res.on('end', () => {
        resolve({
          statusCode: res.statusCode,
          headers: res.headers,
          body: body
        });
      });
    });

    req.on('error', (err) => {
      reject(err);
    });

    if (postData) {
      req.write(postData);
    }
    req.end();
  });
}

async function runHarness() {
  console.log('================================================================');
  console.log('   CHALLENGER M15-1: LIVE DAEMON ADVERSARIAL STRESS HARNESS    ');
  console.log('================================================================\n');

  // -------------------------------------------------------------
  // SUITE 1: Core Daemon Endpoints Live Verification
  // -------------------------------------------------------------
  console.log('--- SUITE 1: Core Daemon Endpoints Live Verification ---');

  // 1.1 /healthz
  try {
    const res = await request({ path: '/healthz' });
    assert(res.statusCode === 200, 'GET /healthz returns 200 OK');
    const json = JSON.parse(res.body);
    assert(json.status === 'UP', '/healthz payload status is UP');
    assert(typeof json.timestamp === 'string', '/healthz payload contains timestamp');
  } catch (err) {
    assert(false, 'GET /healthz threw error: ' + err.message);
  }

  // 1.2 /api/account
  try {
    const res = await request({ path: '/api/account' });
    assert(res.statusCode === 200, 'GET /api/account returns 200 OK');
    const json = JSON.parse(res.body);
    assert(json.email && json.email.includes('@'), '/api/account returns valid email address');
    assert(json.plan_name && json.plan_name.length > 0, '/api/account returns detected plan_name');
    assert(json.token_status === 'VALID', '/api/account returns token_status VALID');
  } catch (err) {
    assert(false, 'GET /api/account threw error: ' + err.message);
  }

  // 1.3 /api/agents/graph?range=today
  try {
    const res = await request({ path: '/api/agents/graph?range=today' });
    assert(res.statusCode === 200, 'GET /api/agents/graph?range=today returns 200 OK');
    const json = JSON.parse(res.body);
    assert(Array.isArray(json.nodes), '/api/agents/graph returns nodes array');
    assert(Array.isArray(json.links), '/api/agents/graph returns links array');
    assert(Array.isArray(json.projects), '/api/agents/graph returns projects array');
    assert(Array.isArray(json.categories), '/api/agents/graph returns categories array');
    assert(json.nodes.length > 0, `/api/agents/graph contains live nodes (found: ${json.nodes.length})`);
    assert(json.links.length > 0, `/api/agents/graph contains live links (found: ${json.links.length})`);
  } catch (err) {
    assert(false, 'GET /api/agents/graph?range=today threw error: ' + err.message);
  }

  // 1.4 /api/projects/leaderboard?range=today
  try {
    const res = await request({ path: '/api/projects/leaderboard?range=today' });
    assert(res.statusCode === 200, 'GET /api/projects/leaderboard?range=today returns 200 OK');
    const json = JSON.parse(res.body);
    assert(json.time_range === 'today', 'Leaderboard response echoes time_range=today');
    assert(json.sort_by === 'tokens', 'Leaderboard default sort_by is tokens');
    assert(json.kpis && typeof json.kpis.grand_total_tokens === 'number', 'Leaderboard contains valid KPIs object');
    assert(Array.isArray(json.projects), 'Leaderboard contains projects array');
    assert(json.projects.length > 0, `Leaderboard contains projects (found: ${json.projects.length})`);
  } catch (err) {
    assert(false, 'GET /api/projects/leaderboard?range=today threw error: ' + err.message);
  }

  // 1.5 /docs/
  try {
    const res = await request({ path: '/docs/' });
    assert(res.statusCode === 200, 'GET /docs/ returns 200 OK');
    assert(res.body.includes('TokenMonitor') || res.body.includes('DOCS_DATA'), '/docs/ serves master offline documentation HTML');
    assert(res.headers['content-type'].includes('text/html'), '/docs/ serves text/html content-type');
  } catch (err) {
    assert(false, 'GET /docs/ threw error: ' + err.message);
  }

  // -------------------------------------------------------------
  // SUITE 2: Range & Sort Query Variations
  // -------------------------------------------------------------
  console.log('\n--- SUITE 2: Range & Sort Query Variations ---');

  const validRanges = ['today', '24h', '7d', '30d', 'all'];
  const validSorts = ['tokens', 'cost', 'activity'];

  // Test all range variations on /api/agents/graph
  for (const r of validRanges) {
    try {
      const res = await request({ path: `/api/agents/graph?range=${r}` });
      assert(res.statusCode === 200, `GET /api/agents/graph?range=${r} returns 200 OK`);
      const json = JSON.parse(res.body);
      assert(Array.isArray(json.nodes), `Range ${r} returns valid nodes array`);
    } catch (err) {
      assert(false, `GET /api/agents/graph?range=${r} failed: ` + err.message);
    }
  }

  // Test all range & sort combinations on /api/projects/leaderboard
  for (const r of validRanges) {
    for (const s of validSorts) {
      try {
        const res = await request({ path: `/api/projects/leaderboard?range=${r}&sort=${s}` });
        assert(res.statusCode === 200, `GET /api/projects/leaderboard?range=${r}&sort=${s} returns 200 OK`);
        const json = JSON.parse(res.body);
        assert(json.time_range === r, `Payload time_range matches '${r}'`);
        assert(json.sort_by === s, `Payload sort_by matches '${s}'`);
        assert(Array.isArray(json.projects), `Payload contains projects array`);
      } catch (err) {
        assert(false, `GET /api/projects/leaderboard?range=${r}&sort=${s} failed: ` + err.message);
      }
    }
  }

  // -------------------------------------------------------------
  // SUITE 3: Adversarial Input & Fuzzing (Zero 500 Guarantee)
  // -------------------------------------------------------------
  console.log('\n--- SUITE 3: Adversarial Input & Fuzzing (Zero 500 Guarantee) ---');

  const adversarialQueryParams = [
    '',
    'range=',
    'range=invalid_range_123',
    'range=100d',
    'range=yesterday',
    'range=-1d',
    'range=today!',
    'range=9999999999999999999999999999999999999999',
    'range=null',
    'range=undefined',
    'range=%00',
    'range=' + 'a'.repeat(2048),
    'sort=invalid_sort_xyz',
    'sort=name',
    'sort=id',
    'sort=rank',
    'sort=tokenss',
    'sort=%00',
    'sort=' + 'b'.repeat(2048)
  ];

  for (const query of adversarialQueryParams) {
    const url = `/api/projects/leaderboard?${query}`;
    try {
      const res = await request({ path: url });
      // The handler must either return 400 Bad Request or gracefully handle it (e.g., 200 with default).
      // Under NO circumstances should it return 500 Internal Server Error or crash!
      assert(res.statusCode !== 500, `Fuzzing '${query.slice(0, 40)}...' on leaderboard: Zero 500 (Status: ${res.statusCode})`);
    } catch (err) {
      assert(false, `Fuzzing '${query.slice(0, 40)}...' threw network/crash error: ` + err.message);
    }
  }

  // Fuzzing on /api/agents/graph
  const graphFuzzParams = [
    'range=xyz',
    'range=9999',
    'project=non_existent_project_999',
    'range=&project=',
    'range=all&project=%27%20OR%201=1'
  ];

  for (const query of graphFuzzParams) {
    const url = `/api/agents/graph?${query}`;
    try {
      const res = await request({ path: url });
      assert(res.statusCode !== 500, `Fuzzing '${query}' on /api/agents/graph: Zero 500 (Status: ${res.statusCode})`);
    } catch (err) {
      assert(false, `Fuzzing '${query}' threw error: ` + err.message);
    }
  }

  // -------------------------------------------------------------
  // SUITE 4: Injection Attack Resilience (SQLi, XSS, Path Traversal)
  // -------------------------------------------------------------
  console.log('\n--- SUITE 4: Injection Attack Resilience ---');

  const sqliPayloads = [
    "range=' OR 1=1 --",
    "range=today' UNION SELECT 1,2,3--",
    "sort=tokens; DROP TABLE users;--",
    "sort=cost' OR '1'='1",
    "range=today' AND SLEEP(5)--"
  ];

  for (const payload of sqliPayloads) {
    const url = `/api/projects/leaderboard?${encodeURI(payload)}`;
    try {
      const res = await request({ path: url });
      assert(res.statusCode !== 500, `SQLi defense for [${payload}]: Status ${res.statusCode} (Not 500)`);
    } catch (err) {
      assert(false, `SQLi test [${payload}] failed: ` + err.message);
    }
  }

  const xssPayloads = [
    'range=<script>alert("xss")</script>',
    'sort=<img src=x onerror=alert(1)>',
    'project=<svg/onload=alert(1)>'
  ];

  for (const payload of xssPayloads) {
    const url = `/api/projects/leaderboard?${encodeURI(payload)}`;
    try {
      const res = await request({ path: url });
      assert(res.statusCode !== 500, `XSS defense for [${payload}]: Status ${res.statusCode}`);
      // When error is returned with user input, verify Content-Type is text/plain and nosniff is set
      if (res.statusCode >= 400) {
        const ct = res.headers['content-type'] || '';
        const nosniff = res.headers['x-content-type-options'] || '';
        assert(ct.includes('text/plain'), `Error response Content-Type is text/plain (got: ${ct})`);
        assert(nosniff === 'nosniff', `Error response has X-Content-Type-Options: nosniff`);
      }
    } catch (err) {
      assert(false, `XSS test [${payload}] failed: ` + err.message);
    }
  }

  // Path traversal defense tests
  const pathTraversalPayloads = [
    '/docs/../main.go',
    '/docs/../../data/token_monitor.db',
    '/docs/..%2f..%2fdata/token_monitor.db',
    '/docs/../../../../windows/win.ini',
    '/docs/%2e%2e/%2e%2e/data/token_monitor.db'
  ];

  for (const path of pathTraversalPayloads) {
    try {
      const res = await request({ path });
      assert(res.statusCode === 404 || res.statusCode === 400 || res.statusCode === 403 || res.statusCode === 307,
        `Path traversal defense for [${path}]: Status ${res.statusCode} (Non-200)`);
      assert(!res.body.includes('func main()') && !res.body.includes('SQLite format 3') && !res.body.includes('[extensions]'),
        `Path traversal defense: Sensitive source or database file NOT leaked`);
    } catch (err) {
      assert(false, `Path traversal test [${path}] failed: ` + err.message);
    }
  }

  // -------------------------------------------------------------
  // SUITE 5: HTTP Method Tampering (405 Method Not Allowed)
  // -------------------------------------------------------------
  console.log('\n--- SUITE 5: HTTP Method Tampering ---');

  const disallowedMethods = ['POST', 'PUT', 'DELETE', 'PATCH'];
  const testEndpoints = ['/api/projects/leaderboard', '/healthz', '/api/account', '/api/agents/graph'];

  for (const ep of testEndpoints) {
    for (const m of disallowedMethods) {
      try {
        const res = await request({ path: ep, method: m }, JSON.stringify({ attack: true }));
        // Should return 405 Method Not Allowed, 200, 400, or 404, but NEVER 500
        assert(res.statusCode !== 500, `${m} ${ep} handled safely (Status: ${res.statusCode}, not 500)`);
        if (ep === '/api/projects/leaderboard') {
          assert(res.statusCode === 405, `${m} ${ep} strictly returns 405 Method Not Allowed (got: ${res.statusCode})`);
        }
      } catch (err) {
        assert(false, `${m} ${ep} failed: ` + err.message);
      }
    }
  }

  // Non-existent route returns 404
  try {
    const res = await request({ path: '/api/does_not_exist_xyz123' });
    assert(res.statusCode === 404, 'GET /api/does_not_exist_xyz123 returns 404 Not Found');
  } catch (err) {
    assert(false, 'GET 404 route failed: ' + err.message);
  }

  // -------------------------------------------------------------
  // SUITE 6: High Concurrency Burst Stress Test
  // -------------------------------------------------------------
  console.log('\n--- SUITE 6: High Concurrency Burst Stress Test (50 Parallel Requests) ---');

  const burstEndpoints = [
    '/healthz',
    '/api/account',
    '/api/agents/graph?range=today',
    '/api/projects/leaderboard?range=today&sort=tokens',
    '/api/projects/leaderboard?range=24h&sort=cost',
    '/api/projects/leaderboard?range=7d&sort=activity',
    '/api/projects/leaderboard?range=30d&sort=tokens',
    '/api/projects/leaderboard?range=all&sort=cost',
    '/api/metrics/summary?range=today',
    '/docs/'
  ];

  const totalBurst = 50;
  const burstPromises = [];
  const startTime = Date.now();

  for (let i = 0; i < totalBurst; i++) {
    const ep = burstEndpoints[i % burstEndpoints.length];
    burstPromises.push(
      request({ path: ep }).then(res => ({
        index: i,
        endpoint: ep,
        statusCode: res.statusCode
      }))
    );
  }

  try {
    const results = await Promise.all(burstPromises);
    const duration = Date.now() - startTime;
    const allSuccessful = results.every(r => r.statusCode === 200);
    const has500 = results.some(r => r.statusCode === 500);

    assert(!has500, `Zero 500 errors across ${totalBurst} concurrent requests in burst`);
    assert(allSuccessful, `All ${totalBurst} concurrent requests completed with 200 OK (Duration: ${duration}ms, Avg: ${(duration/totalBurst).toFixed(1)}ms/req)`);
  } catch (err) {
    assert(false, 'Burst stress test threw error: ' + err.message);
  }

  // -------------------------------------------------------------
  // Final Evaluation
  // -------------------------------------------------------------
  console.log('\n================================================================');
  console.log(`TOTAL CHECKS: ${totalTests}`);
  console.log(`PASSED:       ${passedTests}`);
  console.log(`FAILED:       ${failedTests}`);
  console.log('================================================================\n');

  if (failedTests === 0) {
    console.log('🏆 VERDICT: ALL DAEMON ADVERSARIAL CHALLENGES PASSED (APPROVE)\n');
    process.exit(0);
  } else {
    console.error(`🚨 VERDICT: ${failedTests} CHALLENGES FAILED (REJECT)\n`);
    process.exit(1);
  }
}

runHarness().catch(err => {
  console.error('Fatal harness error:', err);
  process.exit(1);
});

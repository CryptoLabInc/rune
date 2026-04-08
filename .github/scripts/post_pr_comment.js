'use strict';
const fs = require('fs');

module.exports = async ({ github, context }) => {
  const rawOutput = fs.readFileSync('pytest_output.txt', 'utf8');
  const exitCode = process.env.EXIT_CODE;
  const passed = exitCode === '0';
  const status = passed ? '✅ All tests passed' : '❌ Some tests failed';

  // Parse pytest final summary line for authoritative counts
  // e.g. "4 failed, 261 passed, 2 skipped in 4.06s"
  function parsePytestSummary(output) {
    const lines = output.split('\n');
    for (let i = lines.length - 1; i >= 0; i--) {
      const line = lines[i];
      if (/^=+\s+/.test(line) && /\d+\s+(passed|failed)/.test(line)) {
        const counts = { passed: 0, failed: 0, error: 0, skipped: 0, xfailed: 0, xpassed: 0 };
        for (const [, n, k] of line.matchAll(/(\d+)\s+(passed|failed|error|skipped|xfailed|xpassed)/g)) {
          counts[k] = parseInt(n);
        }
        counts.total = Object.values(counts).reduce((a, b) => a + b, 0);
        return counts;
      }
    }
    return null;
  }

  // Parse SKIPPED entries from pytest's "short test summary info" section
  // Lines like: "SKIPPED [1] agents/tests/test_foo.py:175: reason text"
  // This section exists in the main pytest run output and covers both
  // runtime skips (with reason) and collection-level skips.
  function parseShortSummarySkips(output) {
    const entries = [];
    let inSummary = false;
    for (const line of output.split('\n')) {
      if (/=+\s+short test summary info\s+=+/.test(line)) { inSummary = true; continue; }
      if (inSummary) {
        if (/^={3,}/.test(line)) break;
        const m = line.match(/^SKIPPED\s+\[\d+\]\s+(.+?):(\d+):\s+(.+)/);
        if (m) entries.push({ file: m[1], line: parseInt(m[2]), reason: m[3].trim() });
      }
    }
    return entries;
  }

  // Parse individual test results from verbose pytest output
  // e.g. "tests/foo.py::TestClass::test_bar PASSED     [ 50%]"
  // e.g. "tests/foo.py::TestClass::test_bar SKIPPED (reason)  [ 50%]"
  const testResults = [];
  for (const line of rawOutput.split('\n')) {
    const m = line.match(/^(\S+::\S+)\s+(PASSED|FAILED|ERROR|SKIPPED|XFAIL|XPASS)(?:\s+\(([^)]*)\))?/);
    if (m) testResults.push({ id: m[1], result: m[2], reason: m[3] || null });
  }

  const pytestSummary = parsePytestSummary(rawOutput);


  const resultIcon = { PASSED: '✅', FAILED: '❌', ERROR: '💥', SKIPPED: '⏭️', XFAIL: '🔕', XPASS: '⚠️', 'NOT RUN': '⚪' };

  // Parse skip reasons from the short test summary section of the main output
  const shortSummarySkips = parseShortSummarySkips(rawOutput);

  const resultById = {};
  for (const t of testResults) {
    resultById[t.id] = t.result;
  }

  // Tests Added/Modified in This PR — per-test PASS/FAIL and summary
  const hasChanged = process.env.HAS_CHANGED === 'true';
  const changedIds = (process.env.TEST_IDS || '').trim().split(' ').filter(Boolean);

  // For parameterized tests, ci_changed_tests.py outputs the base function name
  // (e.g. "test_foo") but pytest IDs include params (e.g. "test_foo[param1-param2]").
  // Resolve a changed ID to its aggregate result across all matching param variants.
  const resolveResult = (id) => {
    if (resultById[id]) return resultById[id];
    const prefix = id + '[';
    const variants = testResults.filter(t => t.id.startsWith(prefix));
    if (variants.length === 0) return 'NOT RUN';
    if (variants.some(t => t.result === 'FAILED' || t.result === 'ERROR')) return 'FAILED';
    if (variants.every(t => t.result === 'PASSED')) return 'PASSED';
    if (variants.every(t => t.result === 'SKIPPED' || t.result === 'XFAIL')) return 'SKIPPED';
    return variants[0].result;
  };

  let changedSection = '';
  if (hasChanged && changedIds.length > 0) {
    const changedPass = changedIds.filter(id => resolveResult(id) === 'PASSED').length;
    const notPassedRows = changedIds
      .map(id => ({ id, res: resolveResult(id) }))
      .filter(({ res }) => res !== 'PASSED')
      .map(({ id, res }) => `| ${resultIcon[res]} ${res} | \`${id}\` |`);
    const notableTable = notPassedRows.length > 0
      ? ['', '| Result | Test |', '|--------|------|', ...notPassedRows, ''].join('\n')
      : '';
    changedSection = [
      '### Tests Added/Modified in This PR',
      '',
      `**${changedPass} / ${changedIds.length} passed**`,
      notableTable,
    ].join('\n');
  }

  // Full Test Results: summary counts + failed + skipped tests
  const failedTests = testResults.filter(t => t.result === 'FAILED' || t.result === 'ERROR');
  const skippedTests = testResults.filter(t => t.result === 'SKIPPED' || t.result === 'XFAIL');
  const totalPass = testResults.filter(t => t.result === 'PASSED').length;
  const totalFail = failedTests.length;
  const totalSkip = skippedTests.length;

  // Detect collection-skipped: short summary entries whose file has no collected tests
  const collectedFiles = new Set(testResults.map(t => t.id.split('::')[0]));
  const collectionSkips = shortSummarySkips.filter(s => !collectedFiles.has(s.file));
  const summarySkipped = pytestSummary ? pytestSummary.skipped : 0;
  const collectionSkipCount = Math.max(0, summarySkipped - totalSkip);

  // Build corrected totals using pytest summary as authoritative source
  const authoritativeTotal = pytestSummary ? pytestSummary.total : testResults.length;
  const correctedTotal = testResults.length + collectionSkipCount;

  // Warn if corrected total still doesn't match summary after reconciliation
  const stillMismatched = pytestSummary && correctedTotal !== authoritativeTotal;

  const runUrl = `https://github.com/${process.env.GITHUB_REPOSITORY}/actions/runs/${process.env.GITHUB_RUN_ID}`;

  let tableSection = '';
  if (testResults.length > 0) {
    const correctedSkip = totalSkip + collectionSkipCount;
    const skipPart = correctedSkip > 0 ? `, ${correctedSkip} skipped` : '';
    const summary = `**${totalPass} passed, ${totalFail} failed${skipPart}** (${authoritativeTotal} total) — [See CI logs](${runUrl})`;
    const notableRows = [
      ...failedTests.map(t => `| ${resultIcon[t.result]} ${t.result} | \`${t.id}\` |`),
      ...skippedTests.map(t => `| ${resultIcon[t.result]} ${t.result} | \`${t.id}\` |`),
      ...(collectionSkipCount > 0
        ? (collectionSkips.length > 0
            ? collectionSkips.map(s => `| ⏭️ SKIPPED (collection) | \`${s.file}:${s.line}\` |`)
            : [`| ⏭️ SKIPPED (collection) | ${collectionSkipCount} test(s) skipped during collection — names unavailable |`])
        : []),
    ];
    const notableTable = notableRows.length > 0
      ? ['', '| Result | Test |', '|--------|------|', ...notableRows, ''].join('\n')
      : '';
    tableSection = ['### Full Test Results', '', summary, notableTable].join('\n');
  }

  // Warning if test counts still don't reconcile after correction
  const warningSection = stillMismatched
    ? [
        '---',
        '### ⚠️ Count Mismatch Warning',
        '',
        `pytest reported **${authoritativeTotal} total** tests in its summary, but the reconciled count is **${correctedTotal}**.`,
        'This may indicate XFAIL/XPASS results or other collection anomalies not reflected above.',
        '',
      ].join('\n')
    : '';

  const headSha = process.env.HEAD_SHA;
  const shortSha = headSha.slice(0, 7);
  const commitUrl = `https://github.com/${process.env.GITHUB_REPOSITORY}/commit/${headSha}`;

  const marker = '<!-- pr-tests-bot -->';
  const bodyParts = [
    marker,
    '## ' + status,
    '',
    `> Ran on commit [\`${shortSha}\`](${commitUrl})`,
    '',
  ];

  if (warningSection) bodyParts.push(warningSection);
  if (changedSection) bodyParts.push(changedSection);
  if (tableSection) bodyParts.push(tableSection);

  const LIMIT = 60000;
  const TRUNCATION = `\n\n> ⚠️ Comment truncated — [See CI logs](${runUrl}) for the full results.`;

  let body = bodyParts.join('\n');
  if (body.length > LIMIT) {
    const cutAt = body.lastIndexOf('\n', LIMIT - TRUNCATION.length);
    body = body.slice(0, cutAt > 0 ? cutAt : LIMIT - TRUNCATION.length) + TRUNCATION;
  }

  const { data: comments } = await github.rest.issues.listComments({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: context.issue.number,
  });

  const existing = comments.find(c => c.body && c.body.includes(marker));

  if (existing) {
    await github.rest.issues.updateComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      comment_id: existing.id,
      body,
    });
  } else {
    await github.rest.issues.createComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      issue_number: context.issue.number,
      body,
    });
  }
};

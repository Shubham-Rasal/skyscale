import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const API_ENDPOINT = __ENV.API_URL || 'http://n8n.maximalstudio.in:8080/api';
const FUNCTION_NAME = __ENV.FUNCTION_NAME || `load-test-${Date.now()}`;
const HANDLE_SLEEP_SEC = Number(__ENV.HANDLE_SLEEP_SEC || '3');

const vmDuration = new Trend('vm_duration_ms', true);
const invokeOk = new Counter('invoke_ok');
const invokeFail = new Counter('invoke_fail');

const SAMPLE_CSV = `customer_id,plan,monthly_spend,events_last_30d,churn_risk,region
1001,pro,129.50,84,0.12,NA
1002,starter,19.00,12,0.43,EU
1003,enterprise,899.00,412,0.04,US
1004,pro,,55,0.20,US
1005,starter,19.00,,0.62,
1006,enterprise,1200.00,530,0.03,APAC
1007,pro,149.00,73,,EU
1008,starter,9.00,4,0.81,
`;

const PROFILER_CODE = `import base64
import csv
import io
import math
import time

NULLS = {"", "null", "none", "nan", "n/a", "na"}

def is_null(value):
    return value is None or str(value).strip().lower() in NULLS

def to_float(value):
    if is_null(value):
        return None
    try:
        n = float(str(value).strip())
        return None if math.isnan(n) or math.isinf(n) else n
    except Exception:
        return None

def handle(event, context):
    time.sleep(${HANDLE_SLEEP_SEC})
    csv_text = base64.b64decode(event.get("csv_b64", "")).decode("utf-8")
    reader = csv.DictReader(io.StringIO(csv_text))
    rows = list(reader)
    columns = reader.fieldnames or []
    report = []
    for column_name in columns:
        values = [row.get(column_name, "") for row in rows]
        null_count = sum(1 for value in values if is_null(value))
        non_null = [value for value in values if not is_null(value)]
        nums = [to_float(value) for value in non_null]
        nums = [value for value in nums if value is not None]
        numeric = bool(non_null) and len(nums) == len(non_null)
        item = {
            "name": column_name,
            "type": "number" if numeric else "string",
            "null_count": null_count,
            "null_pct": round(null_count * 100 / len(rows), 2) if rows else 0,
            "unique_count": len({str(value).strip() for value in non_null}),
        }
        if numeric and nums:
            ordered = sorted(nums)
            item["stats"] = {
                "min": min(nums),
                "max": max(nums),
                "mean": round(sum(nums) / len(nums), 3),
                "p50": ordered[len(ordered) // 2],
            }
        report.append(item)
    issues = []
    for col in report:
        if col["null_pct"] >= 30:
            issues.append({"severity": "high", "column": col["name"], "message": str(col["null_pct"]) + "% missing"})
        elif col["null_count"]:
            issues.append({"severity": "medium", "column": col["name"], "message": str(col["null_count"]) + " missing values"})
    score = max(0, round(100 - sum(c["null_pct"] for c in report) / max(len(report), 1), 1))
    return {"ok": True, "row_count": len(rows), "column_count": len(columns), "quality_score": score, "columns": report, "issues": issues}
`;

export const options = {
  scenarios: {
    burst: {
      executor: 'shared-iterations',
      vus: 100,
      iterations: 100,
      maxDuration: '15m',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    invoke_ok: ['count>=95'],
  },
};

export function setup() {
  const headers = { 'Content-Type': 'application/json' };
  const payload = JSON.stringify({
    name: FUNCTION_NAME,
    runtime: 'python3',
    memory: 128,
    timeout: 60,
    code: PROFILER_CODE,
    requirements: '',
    config: '',
  });

  const res = http.post(`${API_ENDPOINT}/functions`, payload, { headers });
  if (res.status !== 200) {
    throw new Error(`register failed: ${res.status} ${res.body}`);
  }

  const csvB64 = encoding.b64encode(SAMPLE_CSV);
  return { functionName: FUNCTION_NAME, csvB64 };
}

export default function (data) {
  const headers = { 'Content-Type': 'application/json' };
  const payload = JSON.stringify({
    input: { csv_b64: data.csvB64 },
    sync: true,
    job_type: 'faas_function',
    hardware_type: 'cpu',
  });

  const res = http.post(
    `${API_ENDPOINT}/functions/name/${encodeURIComponent(data.functionName)}/invoke`,
    payload,
    { headers, timeout: '10m' },
  );

  const ok = check(res, {
    'invoke status 200': (r) => r.status === 200,
  });

  if (ok) {
    invokeOk.add(1);
    try {
      const body = JSON.parse(res.body);
      if (body.duration_ms) vmDuration.add(body.duration_ms);
    } catch (_) {}
  } else {
    invokeFail.add(1);
  }
}

// k6 lacks btoa in setup(); use this minimal encoder
const encoding = {
  b64encode(str) {
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=';
    let output = '';
    let i = 0;
    while (i < str.length) {
      const chr1 = str.charCodeAt(i++);
      const chr2 = str.charCodeAt(i++);
      const chr3 = str.charCodeAt(i++);
      const enc1 = chr1 >> 2;
      const enc2 = ((chr1 & 3) << 4) | (chr2 >> 4);
      let enc3 = ((chr2 & 15) << 2) | (chr3 >> 6);
      let enc4 = chr3 & 63;
      if (Number.isNaN(chr2)) { enc3 = enc4 = 64; }
      else if (Number.isNaN(chr3)) { enc4 = 64; }
      output += chars.charAt(enc1) + chars.charAt(enc2) + chars.charAt(enc3) + chars.charAt(enc4);
    }
    return output;
  },
};

import {
  BedrockRuntimeClient,
  InvokeModelCommand,
  InvokeModelWithResponseStreamCommand,
} from "@aws-sdk/client-bedrock-runtime";

const BEDROCK_REGION = process.env.BEDROCK_REGION || "us-east-1";
const DEFAULT_MODEL = process.env.DEFAULT_MODEL || "claude-opus-4-6";

// auto: 根据 BEDROCK_REGION 自动加 us. / eu. / apac.
// none: 不加 cross-region prefix
// us/eu/ap/apac/global: 强制加指定 prefix
const CROSS_REGION_MODE = (process.env.CROSS_REGION_MODE || "auto").toLowerCase();

const DEBUG_RAW = (process.env.DEBUG_RAW || "false").toLowerCase() === "true";

const bedrock = new BedrockRuntimeClient({
  region: BEDROCK_REGION,
  maxAttempts: 2,
});

const AWS_MODEL_ID_MAP = {
  // Claude 3
  "claude-3-sonnet-20240229": "anthropic.claude-3-sonnet-20240229-v1:0",
  "claude-3-opus-20240229": "anthropic.claude-3-opus-20240229-v1:0",
  "claude-3-haiku-20240307": "anthropic.claude-3-haiku-20240307-v1:0",

  // Claude 3.5 / 3.7
  "claude-3-5-sonnet-20240620": "anthropic.claude-3-5-sonnet-20240620-v1:0",
  "claude-3-5-sonnet-20241022": "anthropic.claude-3-5-sonnet-20241022-v2:0",
  "claude-3-5-haiku-20241022": "anthropic.claude-3-5-haiku-20241022-v1:0",
  "claude-3-7-sonnet-20250219": "anthropic.claude-3-7-sonnet-20250219-v1:0",

  // Claude 4
  "claude-sonnet-4-20250514": "anthropic.claude-sonnet-4-20250514-v1:0",
  "claude-opus-4-20250514": "anthropic.claude-opus-4-20250514-v1:0",
  "claude-opus-4-1-20250805": "anthropic.claude-opus-4-1-20250805-v1:0",
  "claude-sonnet-4-5-20250929": "anthropic.claude-sonnet-4-5-20250929-v1:0",
  "claude-sonnet-4-6": "anthropic.claude-sonnet-4-6",
  "claude-haiku-4-5-20251001": "anthropic.claude-haiku-4-5-20251001-v1:0",
  "claude-opus-4-5-20251101": "anthropic.claude-opus-4-5-20251101-v1:0",
  "claude-opus-4-6": "anthropic.claude-opus-4-6-v1",
  "claude-opus-4-7": "anthropic.claude-opus-4-7",
  "claude-opus-4-8": "anthropic.claude-opus-4-8",

  // 常用短别名
  "opus-4-6": "anthropic.claude-opus-4-6-v1",
  "opus-4.6": "anthropic.claude-opus-4-6-v1",
  "opus-4-7": "anthropic.claude-opus-4-7",
  "opus-4.7": "anthropic.claude-opus-4-7",
  "opus-4-8": "anthropic.claude-opus-4-8",
  "opus-4.8": "anthropic.claude-opus-4-8",
  "sonnet-4-6": "anthropic.claude-sonnet-4-6",
  "sonnet-4.6": "anthropic.claude-sonnet-4-6",
  "sonnet-4-5": "anthropic.claude-sonnet-4-5-20250929-v1:0",
  "sonnet-4.5": "anthropic.claude-sonnet-4-5-20250929-v1:0",
  "haiku-4-5": "anthropic.claude-haiku-4-5-20251001-v1:0",
  "haiku-4.5": "anthropic.claude-haiku-4-5-20251001-v1:0",

  // Claude 5 aliases: 账号没权限时会返回 AccessDeniedException
  "claude-sonnet-5": "anthropic.claude-sonnet-5",
  "sonnet-5": "anthropic.claude-sonnet-5",
  "claude-fable-5": "anthropic.claude-fable-5",
  "fable-5": "anthropic.claude-fable-5",

  // Nova models：本 Lambda 是 Anthropic SDK /v1/messages 兼容入口，不直接服务 Nova streaming
  "nova-micro-v1:0": "amazon.nova-micro-v1:0",
  "nova-lite-v1:0": "amazon.nova-lite-v1:0",
  "nova-pro-v1:0": "amazon.nova-pro-v1:0",
  "nova-premier-v1:0": "amazon.nova-premier-v1:0",
  "nova-canvas-v1:0": "amazon.nova-canvas-v1:0",
  "nova-reel-v1:0": "amazon.nova-reel-v1:0",
  "nova-reel-v1:1": "amazon.nova-reel-v1:1",
  "nova-sonic-v1:0": "amazon.nova-sonic-v1:0",

  "nova-micro": "amazon.nova-micro-v1:0",
  "nova-lite": "amazon.nova-lite-v1:0",
  "nova-pro": "amazon.nova-pro-v1:0",
  "nova-premier": "amazon.nova-premier-v1:0",
  "nova-canvas": "amazon.nova-canvas-v1:0",
  "nova-reel": "amazon.nova-reel-v1:0",
  "nova-sonic": "amazon.nova-sonic-v1:0",
};

const AWS_MODEL_CAN_CROSS_REGION_MAP = {
  "anthropic.claude-3-sonnet-20240229-v1:0": { us: true, eu: true, ap: true },
  "anthropic.claude-3-opus-20240229-v1:0": { us: true },
  "anthropic.claude-3-haiku-20240307-v1:0": { us: true, eu: true, ap: true },
  "anthropic.claude-3-5-sonnet-20240620-v1:0": { us: true, eu: true, ap: true },
  "anthropic.claude-3-5-sonnet-20241022-v2:0": { us: true, ap: true },
  "anthropic.claude-3-5-haiku-20241022-v1:0": { us: true },
  "anthropic.claude-3-7-sonnet-20250219-v1:0": { us: true, ap: true, eu: true },

  "anthropic.claude-sonnet-4-20250514-v1:0": { us: true, ap: true, eu: true },
  "anthropic.claude-opus-4-20250514-v1:0": { us: true },
  "anthropic.claude-opus-4-1-20250805-v1:0": { us: true },
  "anthropic.claude-sonnet-4-5-20250929-v1:0": { us: true, ap: true, eu: true },
  "anthropic.claude-sonnet-4-6": { us: true, ap: true, eu: true },
  "anthropic.claude-opus-4-5-20251101-v1:0": { us: true, ap: true, eu: true },
  "anthropic.claude-opus-4-6-v1": { us: true, ap: true, eu: true },
  "anthropic.claude-opus-4-7": { us: true, ap: true, eu: true },
  "anthropic.claude-opus-4-8": { us: true, ap: true, eu: true },
  "anthropic.claude-haiku-4-5-20251001-v1:0": { us: true, ap: true, eu: true },

  "anthropic.claude-sonnet-5": { us: true, global: true },
  "anthropic.claude-fable-5": { us: true, eu: true, global: true },

  "amazon.nova-micro-v1:0": { us: true, eu: true, apac: true },
  "amazon.nova-lite-v1:0": { us: true, eu: true, apac: true },
  "amazon.nova-pro-v1:0": { us: true, eu: true, apac: true },
  "amazon.nova-premier-v1:0": { us: true },
  "amazon.nova-canvas-v1:0": { us: true, eu: true, apac: true },
  "amazon.nova-reel-v1:0": { us: true, eu: true, apac: true },
  "amazon.nova-reel-v1:1": { us: true },
  "amazon.nova-sonic-v1:0": { us: true, eu: true, apac: true },
};

const AWS_REGION_CROSS_MODEL_PREFIX_MAP = {
  us: "us",
  eu: "eu",
  ap: "apac",
  apac: "apac",
  global: "global",
};

const GPT_COMPAT_ALIAS = {
  "gpt-5": "claude-opus-4-8",
  "gpt-4": "claude-sonnet-4-6",
  "gpt-4o": "claude-sonnet-4-6",
  "gpt-4.1": "claude-sonnet-4-6",
  "gpt-4-1": "claude-sonnet-4-6",
  "o3": "claude-opus-4-8",
  "o3-pro": "claude-opus-4-8",
  "gpt-4o-mini": "claude-haiku-4-5-20251001",
  "gpt-4.1-mini": "claude-haiku-4-5-20251001",
  "gpt-4-1-mini": "claude-haiku-4-5-20251001",
  "o3-mini": "claude-sonnet-4-6",
  "o4-mini": "claude-haiku-4-5-20251001",
  "gpt-3.5-turbo": "claude-haiku-4-5-20251001",
  "gpt-35-turbo": "claude-haiku-4-5-20251001",
};

export const handler = awslambda.streamifyResponse(async (event, responseStream, context) => {
  let started = false;
  let out = responseStream;

  const startResponse = (statusCode, headers) => {
    if (started) {
      return out;
    }

    started = true;

    out = awslambda.HttpResponseStream.from(responseStream, {
      statusCode,
      headers,
    });

    return out;
  };

  try {
    const req = parseJsonBody(event);
    req._anthropic_beta = extractAnthropicBeta(req, event);

    const rawModel = req.modelId || req.model || DEFAULT_MODEL;
    const modelId = resolveModelId(rawModel);

    if (DEBUG_RAW) {
      console.log(JSON.stringify({
        path: getRequestPath(event),
        raw_model: rawModel,
        resolved_model: modelId,
        stream: req.stream,
        max_tokens: getMaxTokens(req),
        anthropic_beta: req._anthropic_beta,
      }));
    }

    if (!isAnthropicModel(modelId)) {
      return respondJson(startResponse, 400, {
        error: "This streaming Lambda is Anthropic Messages SDK compatible and only supports Claude models.",
        resolved_model: modelId,
      });
    }

    if (!Array.isArray(req.messages)) {
      return respondJson(startResponse, 400, {
        error: "Missing required field: messages",
      });
    }

    if (req.stream === true) {
      await handleAnthropicStream(req, event, modelId, startResponse);
      return;
    }

    await handleAnthropicNonStream(req, event, modelId, startResponse);
  } catch (err) {
    console.error("UNHANDLED_ERROR:", err);

    if (!started) {
      return respondJson(startResponse, getHttpStatusFromError(err), {
        error: getErrorMessage(err),
        code: err.name || err.Code || "Error",
        type: err.name || "Error",
      });
    }

    writeSse(out, "error", {
      type: "error",
      error: {
        type: "api_error",
        message: getErrorMessage(err),
      },
    });

    out.end();
  }
});

async function handleAnthropicStream(req, event, modelId, startResponse) {
  const bedrockBody = buildAnthropicBedrockBody(req, event, modelId);

  const command = new InvokeModelWithResponseStreamCommand({
    modelId,
    contentType: "application/json",
    accept: "application/json",
    body: Buffer.from(JSON.stringify(bedrockBody), "utf8"),
  });

  const resp = await bedrock.send(command);

  const out = startResponse(200, {
    "content-type": "text/event-stream; charset=utf-8",
    "cache-control": "no-cache, no-transform",
    "connection": "keep-alive",
    "x-accel-buffering": "no",
  });

  await pipeBedrockClaudeStreamToAnthropicSse(resp, out);
}

async function handleAnthropicNonStream(req, event, modelId, startResponse) {
  const bedrockBody = buildAnthropicBedrockBody(req, event, modelId);

  const command = new InvokeModelCommand({
    modelId,
    contentType: "application/json",
    accept: "application/json",
    body: Buffer.from(JSON.stringify(bedrockBody), "utf8"),
  });

  const resp = await bedrock.send(command);
  const bodyText = new TextDecoder().decode(resp.body);

  const out = startResponse(200, {
    "content-type": "application/json",
    "cache-control": "no-cache",
  });

  out.write(bodyText);
  out.end();
}

async function pipeBedrockClaudeStreamToAnthropicSse(resp, out) {
  const decoder = new TextDecoder();

  for await (const part of resp.body) {
    if (part.chunk && part.chunk.bytes) {
      const text = decoder.decode(part.chunk.bytes);
      let payload;

      try {
        payload = JSON.parse(text);
      } catch (e) {
        writeSse(out, "error", {
          type: "error",
          error: {
            type: "parse_error",
            message: "Failed to parse Bedrock stream chunk as JSON.",
            raw: text,
          },
        });
        continue;
      }

      const eventName = payload.type || "message";
      writeSse(out, eventName, payload);

      if (eventName === "message_stop") {
        break;
      }

      continue;
    }

    const serviceError = pickBedrockStreamError(part);
    if (serviceError) {
      writeSse(out, "error", {
        type: "error",
        error: {
          type: serviceError.type,
          message: serviceError.message,
        },
      });
      break;
    }
  }

  out.end();
}

function buildAnthropicBedrockBody(req, event, modelId) {
  let body = {
    anthropic_version: "bedrock-2023-05-31",
    max_tokens: getMaxTokens(req),
    messages: req.messages,
  };

  const anthropicBeta = req._anthropic_beta;
  if (anthropicBeta && anthropicBeta.length > 0) {
    body.anthropic_beta = anthropicBeta;
  }

  const copyKeys = [
    "system",
    "temperature",
    "top_p",
    "top_k",
    "stop_sequences",
    "metadata",
    "tools",
    "tool_choice",
    "thinking",
    "container",
    "context_management",
  ];

  for (const key of copyKeys) {
    if (Object.prototype.hasOwnProperty.call(req, key)) {
      body[key] = req[key];
    }
  }

  if (req.stop && !body.stop_sequences) {
    body.stop_sequences = Array.isArray(req.stop) ? req.stop : [req.stop];
  }

  body = sanitizeAnthropicBodyForModel(body, modelId);
  return body;
}

function sanitizeAnthropicBodyForModel(body, modelId) {
  const base = stripCrossPrefix(modelId);

  if (base === "anthropic.claude-opus-4-7") {
    delete body.temperature;
    delete body.top_p;
    delete body.top_k;
  }

  if (base === "anthropic.claude-fable-5") {
    delete body.top_k;

    if (Object.prototype.hasOwnProperty.call(body, "temperature")) {
      const t = Number(body.temperature);
      if (!Number.isFinite(t) || t !== 1.0) {
        delete body.temperature;
      }
    }

    if (Object.prototype.hasOwnProperty.call(body, "top_p")) {
      const p = Number(body.top_p);
      if (!Number.isFinite(p) || p < 0.99 || p >= 1.0) {
        delete body.top_p;
      }
    }
  }

  return body;
}

function writeSse(out, eventName, data) {
  out.write(`event: ${eventName}\n`);
  out.write(`data: ${JSON.stringify(data)}\n\n`);
}

function respondJson(startResponse, statusCode, obj) {
  const out = startResponse(statusCode, {
    "content-type": "application/json",
    "cache-control": "no-cache",
  });

  out.write(JSON.stringify(obj));
  out.end();
}

function parseJsonBody(event) {
  let raw = event.body || "{}";

  if (event.isBase64Encoded) {
    raw = Buffer.from(raw, "base64").toString("utf8");
  }

  return JSON.parse(raw);
}

function getRequestPath(event) {
  return (
    event.path ||
    event.rawPath ||
    event.requestContext?.http?.path ||
    ""
  );
}

function getHeader(event, name) {
  const target = name.toLowerCase();

  const headers = event.headers || {};
  for (const [k, v] of Object.entries(headers)) {
    if (k && k.toLowerCase() === target) {
      return v;
    }
  }

  const multi = event.multiValueHeaders || {};
  for (const [k, v] of Object.entries(multi)) {
    if (k && k.toLowerCase() === target) {
      if (Array.isArray(v)) {
        return v.join(",");
      }
      return v;
    }
  }

  return undefined;
}

function extractAnthropicBeta(req, event) {
  const values = [];

  for (const key of ["anthropic_beta", "anthropic-beta", "anthropicBeta", "betas"]) {
    if (req[key]) {
      values.push(...normalizeBetaValues(req[key]));
    }
  }

  const headerBeta = getHeader(event, "anthropic-beta");
  if (headerBeta) {
    values.push(...normalizeBetaValues(headerBeta));
  }

  const seen = new Set();
  const out = [];

  for (const item of values) {
    const x = String(item).trim();
    if (x && !seen.has(x)) {
      seen.add(x);
      out.push(x);
    }
  }

  return out;
}

function normalizeBetaValues(v) {
  if (v == null) {
    return [];
  }

  if (Array.isArray(v)) {
    return v.flatMap(normalizeBetaValues);
  }

  if (typeof v === "string") {
    return v
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);
  }

  return [String(v).trim()].filter(Boolean);
}

function normalizeKey(s) {
  return String(s || "")
    .trim()
    .toLowerCase()
    .replace(/_/g, "-")
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9.\-:]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

function splitCrossPrefix(modelId) {
  const s = String(modelId || "").trim();

  for (const p of ["global.", "us.", "eu.", "apac."]) {
    if (s.startsWith(p)) {
      return [p.slice(0, -1), s.slice(p.length)];
    }
  }

  return [null, s];
}

function resolveModelId(model) {
  const raw = String(model || DEFAULT_MODEL).trim();

  if (raw.startsWith("arn:aws:bedrock:")) {
    return raw;
  }

  const [explicitPrefix, rawWithoutPrefix] = splitCrossPrefix(raw);

  let key = normalizeKey(rawWithoutPrefix);

  if (GPT_COMPAT_ALIAS[key]) {
    key = normalizeKey(GPT_COMPAT_ALIAS[key]);
  }

  const candidateKeys = [
    key,
    key.replace(/\./g, "-"),
  ];

  for (const providerPrefix of [
    "anthropic.",
    "amazon.",
    "openai.",
    "meta.",
    "mistral.",
    "cohere.",
    "ai21.",
    "deepseek.",
    "stability.",
  ]) {
    if (key.startsWith(providerPrefix)) {
      const providerless = key.slice(providerPrefix.length);
      candidateKeys.push(providerless);
      candidateKeys.push(providerless.replace(/\./g, "-"));
    }
  }

  let baseModel = null;

  for (const ck of candidateKeys) {
    if (AWS_MODEL_ID_MAP[ck]) {
      baseModel = AWS_MODEL_ID_MAP[ck];
      break;
    }
  }

  if (!baseModel) {
    if (looksLikeNativeBaseModelId(rawWithoutPrefix)) {
      baseModel = rawWithoutPrefix;
    } else {
      return raw;
    }
  }

  if (baseModel.startsWith("openai.")) {
    return baseModel;
  }

  if (explicitPrefix) {
    return `${explicitPrefix}.${baseModel}`;
  }

  return applyCrossRegionPrefix(baseModel);
}

function looksLikeNativeBaseModelId(modelId) {
  const s = String(modelId || "");
  return (
    s.startsWith("anthropic.") ||
    s.startsWith("amazon.") ||
    s.startsWith("openai.") ||
    s.startsWith("meta.") ||
    s.startsWith("mistral.") ||
    s.startsWith("cohere.") ||
    s.startsWith("ai21.") ||
    s.startsWith("deepseek.") ||
    s.startsWith("stability.")
  );
}

function regionGroupFromAwsRegion(region) {
  const r = String(region || "").toLowerCase();

  if (r.startsWith("us-")) return "us";
  if (r.startsWith("eu-")) return "eu";
  if (r.startsWith("ap-")) return "ap";

  return "us";
}

function applyCrossRegionPrefix(baseModel) {
  if (looksPrefixedOrArn(baseModel)) {
    return baseModel;
  }

  if (baseModel.startsWith("openai.")) {
    return baseModel;
  }

  if (CROSS_REGION_MODE === "none") {
    return baseModel;
  }

  if (CROSS_REGION_MODE === "global") {
    return `global.${baseModel}`;
  }

  const supported = AWS_MODEL_CAN_CROSS_REGION_MAP[baseModel];
  if (!supported) {
    return baseModel;
  }

  let group;
  if (["us", "eu", "ap", "apac"].includes(CROSS_REGION_MODE)) {
    group = CROSS_REGION_MODE === "apac" ? "ap" : CROSS_REGION_MODE;
  } else {
    group = regionGroupFromAwsRegion(BEDROCK_REGION);
  }

  const prefix = AWS_REGION_CROSS_MODEL_PREFIX_MAP[group] || group;

  if (supported[group] || supported[prefix]) {
    return `${prefix}.${baseModel}`;
  }

  return baseModel;
}

function looksPrefixedOrArn(modelId) {
  const s = String(modelId || "");
  return (
    s.startsWith("arn:aws:bedrock:") ||
    s.startsWith("us.") ||
    s.startsWith("eu.") ||
    s.startsWith("apac.") ||
    s.startsWith("global.")
  );
}

function stripCrossPrefix(modelId) {
  const s = String(modelId || "");
  for (const p of ["us.", "eu.", "apac.", "global."]) {
    if (s.startsWith(p)) {
      return s.slice(p.length);
    }
  }
  return s;
}

function isAnthropicModel(modelId) {
  return stripCrossPrefix(modelId).startsWith("anthropic.");
}

function parseTokenLimit(v) {
  if (v == null) {
    return null;
  }

  if (typeof v === "number") {
    return Math.trunc(v);
  }

  const s = String(v).trim().toLowerCase().replace(/,/g, "");
  if (!s) {
    return null;
  }

  if (s.endsWith("k")) {
    return Math.trunc(Number(s.slice(0, -1)) * 1000);
  }

  return Math.trunc(Number(s));
}

function getMaxTokens(req) {
  for (const key of [
    "max_tokens",
    "max_completion_tokens",
    "maxTokens",
    "max_output_tokens",
    "maxOutputTokens",
    "max_tokens_to_sample",
    "maxTokensToSample",
    "output_token_limit",
    "outputTokenLimit",
  ]) {
    if (Object.prototype.hasOwnProperty.call(req, key) && req[key] != null) {
      const n = parseTokenLimit(req[key]);
      if (Number.isFinite(n) && n > 0) {
        return n;
      }
    }
  }

  return 512;
}

function pickBedrockStreamError(part) {
  const possible = [
    "internalServerException",
    "modelStreamErrorException",
    "modelTimeoutException",
    "throttlingException",
    "validationException",
    "serviceUnavailableException",
    "accessDeniedException",
  ];

  for (const key of possible) {
    if (part[key]) {
      return {
        type: key,
        message: part[key].message || JSON.stringify(part[key]),
      };
    }
  }

  return null;
}

function getHttpStatusFromError(err) {
  return (
    err?.$metadata?.httpStatusCode ||
    err?.statusCode ||
    500
  );
}

function getErrorMessage(err) {
  return (
    err?.message ||
    err?.Message ||
    String(err)
  );
}
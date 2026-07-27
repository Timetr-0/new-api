#!/usr/bin/env python3
"""Create AWS Bedrock channels through the New API admin API."""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any


DEFAULT_ACCESSIBLE_REGIONS = [
    # ==================== 亚太：南亚 ====================

    # "ap-south-2",       # 亚太（海得拉巴），印度南部
    # "ap-south-1",       # 亚太（孟买），印度西部

    # ==================== 欧洲：南欧 ====================

    # "eu-south-1",       # 欧洲（米兰），意大利
    # "eu-south-2",       # 欧洲（西班牙）

    # ==================== 中东 ====================

    # "il-central-1",     # 以色列（特拉维夫）
    # "me-south-1",       # 中东（巴林）
    # "me-central-1",     # 中东（阿联酋）

    # ==================== 加拿大、墨西哥 ====================

    # "ca-central-1",   # 加拿大（中部）
    # "ca-west-1",      # 加拿大西部（卡尔加里）
    # "mx-central-1",   # 墨西哥（中部）

    # ==================== 欧洲：中欧 ====================

    "eu-central-1",     # 欧洲（法兰克福），德国
    # "eu-central-2",     # 欧洲（苏黎世），瑞士

    # ==================== 美国西部 ====================

    "us-west-1",        # 美国西部（北加利福尼亚）
    "us-west-2",        # 美国西部（俄勒冈）

    # ==================== 非洲 ====================

    # "af-south-1",     # 非洲（开普敦），南非

    # ==================== 欧洲：北欧、西欧 ====================

    "eu-north-1",       # 欧洲（斯德哥尔摩），瑞典
    "eu-west-3",        # 欧洲（巴黎），法国
    "eu-west-2",        # 欧洲（伦敦），英国
    "eu-west-1",        # 欧洲（爱尔兰）

    # ==================== 亚太：东北亚 ====================

    # "ap-northeast-3", # 亚太（大阪），日本
    # "ap-northeast-2", # 亚太（首尔），韩国
    # "ap-northeast-1", # 亚太（东京），日本

    # ==================== 南美洲 ====================

    # "sa-east-1",      # 南美洲（圣保罗），巴西

    # ==================== 亚太：东南亚和大洋洲 ====================

    # "ap-southeast-1", # 亚太（新加坡）
    # "ap-southeast-2", # 亚太（悉尼），澳大利亚
    # "ap-southeast-3", # 亚太（雅加达），印度尼西亚
    # "ap-southeast-4", # 亚太（墨尔本），澳大利亚
    # "ap-southeast-5", # 亚太（马来西亚）
    # "ap-southeast-6", # 亚太（新西兰）
    # "ap-southeast-7", # 亚太（泰国）

    # ==================== 美国东部 ====================

    "us-east-1",        # 美国东部（北弗吉尼亚）
    "us-east-2",        # 美国东部（俄亥俄）

    # ==================== 亚太：东亚 ====================

    # "ap-east-1",        # 亚太（香港）
    # "ap-east-2",      # 亚太（台北），中国台湾
]

DEFAULT_REGION_WEIGHT_TEMPLATES: dict[str, dict[str, int]] = {
    "us-first": {
        "eu-central-1": 50,
        "us-west-1": 80,
        "us-west-2": 100,
        "eu-north-1": 40,
        "eu-west-3": 50,
        "eu-west-2": 60,
        "eu-west-1": 60,
        "us-east-1": 100,
        "us-east-2": 80,
    },
    "eu-first": {
        "eu-central-1": 100,
        "us-west-1": 40,
        "us-west-2": 50,
        "eu-north-1": 70,
        "eu-west-3": 80,
        "eu-west-2": 90,
        "eu-west-1": 100,
        "us-east-1": 50,
        "us-east-2": 40,
    },
}

AWS_CLAUDE_PRESET_MODELS = [
    "claude-3-sonnet-20240229",
    "claude-3-opus-20240229",
    "claude-3-haiku-20240307",
    "claude-3-5-sonnet-20240620",
    "claude-3-5-sonnet-20241022",
    "claude-3-5-haiku-20241022",
    "claude-3-7-sonnet-20250219",
    "claude-sonnet-4-20250514",
    "claude-opus-4-20250514",
    "claude-opus-4-1-20250805",
    "claude-sonnet-4-5-20250929",
    "claude-sonnet-4-6",
    "claude-haiku-4-5-20251001",
    "claude-opus-4-5-20251101",
    "claude-opus-4-6",
    # "claude-opus-4-7",
    # "claude-opus-4-8",
    "global.anthropic.claude-3-sonnet-20240229-v1:0",
    "global.anthropic.claude-3-opus-20240229-v1:0",
    "global.anthropic.claude-3-haiku-20240307-v1:0",
    "global.anthropic.claude-3-5-sonnet-20240620-v1:0",
    "global.anthropic.claude-3-5-sonnet-20241022-v2:0",
    "global.anthropic.claude-3-5-haiku-20241022-v1:0",
    "global.anthropic.claude-3-7-sonnet-20250219-v1:0",
    "global.anthropic.claude-sonnet-4-20250514-v1:0",
    "global.anthropic.claude-opus-4-20250514-v1:0",
    "global.anthropic.claude-opus-4-1-20250805-v1:0",
    "global.anthropic.claude-sonnet-4-5-20250929-v1:0",
    "global.anthropic.claude-sonnet-4-6",
    "global.anthropic.claude-haiku-4-5-20251001-v1:0",
    "global.anthropic.claude-opus-4-5-20251101-v1:0",
    "global.anthropic.claude-opus-4-6-v1",
    # "global.anthropic.claude-opus-4-7",
    # "global.anthropic.claude-opus-4-8",
]

CHANNEL_STATUS_ENABLED = 1
AWS_KEY_TYPE_AKSK = "ak_sk"
AWS_KEY_TYPE_API_KEY = "api_key"
AWS_KEY_TYPE_ROLE_ARN = "role_arn"


class NoRedirectHandler(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def unique_csv(values: list[str]) -> str:
    seen = set()
    items = []
    for value in values:
        value = value.strip()
        if value and value not in seen:
            seen.add(value)
            items.append(value)
    return ",".join(items)


def parse_csv(value: str) -> list[str]:
    return [item.strip() for item in value.split(",") if item.strip()]


def load_regions(args: argparse.Namespace) -> list[str]:
    regions = list(DEFAULT_ACCESSIBLE_REGIONS)
    if args.regions:
        regions = parse_csv(args.regions)
    if args.region_file:
        with open(args.region_file, "r", encoding="utf-8") as region_file:
            regions = [
                line.split("#", 1)[0].strip()
                for line in region_file
                if line.split("#", 1)[0].strip()
            ]
    return list(dict.fromkeys(regions))


def resolve_models_and_mapping(args: argparse.Namespace) -> tuple[str, str | None]:
    if args.models:
        models = unique_csv(parse_csv(args.models))
    elif args.preset in {"aws-claude", "aws-global-claude"}:
        models = unique_csv(AWS_CLAUDE_PRESET_MODELS)
    else:
        models = ""

    if args.model_mapping:
        mapping = json.dumps(json.loads(args.model_mapping), separators=(",", ":"))
    elif args.preset in {"aws-claude", "aws-global-claude"}:
        mapping = None
    else:
        mapping = None

    return models, mapping


def build_payload(
    args: argparse.Namespace,
    region: str,
    index: int,
    models: str,
    model_mapping: str | None,
) -> dict[str, Any]:
    safe_region = region.replace("-", "_")
    session_name = args.session_name.format(
        region=region,
        safe_region=safe_region,
        index=index,
    )
    channel_key = build_channel_key(args, region, session_name)
    channel: dict[str, Any] = {
        "name": args.name_template.format(
            region=region,
            safe_region=safe_region,
            index=index,
        ),
        "type": 33,
        "base_url": None,
        "key": channel_key,
        "openai_organization": None,
        "models": models,
        "group": args.group,
        "model_mapping": model_mapping,
        "priority": args.priority,
        "weight": resolve_region_weight(args, region),
        "test_model": args.test_model or None,
        "auto_ban": args.auto_ban,
        "status": args.status,
        "status_code_mapping": None,
        "tag": args.tag or None,
        "remark": args.remark,
        "setting": json.dumps(
            {
                "force_format": False,
                "thinking_to_content": False,
                "proxy": "",
                "pass_through_body_enabled": False,
                "system_prompt": "",
                "system_prompt_override": False,
            },
            separators=(",", ":"),
        ),
        "param_override": None,
        "header_override": None,
        "settings": json.dumps(
            {"aws_key_type": args.aws_key_type},
            separators=(",", ":"),
        ),
        "other": "",
    }
    return {"mode": "single", "channel": channel}


def resolve_region_weight(args: argparse.Namespace, region: str) -> int:
    if args.weight_template == "none":
        return args.weight
    return DEFAULT_REGION_WEIGHT_TEMPLATES[args.weight_template].get(region, args.weight)


def validate_default_weight_templates() -> None:
    for template_name, template in DEFAULT_REGION_WEIGHT_TEMPLATES.items():
        missing_regions = [
            region for region in DEFAULT_ACCESSIBLE_REGIONS if region not in template
        ]
        if missing_regions:
            raise RuntimeError(
                f"weight template {template_name!r} missing default regions: "
                + ", ".join(missing_regions)
            )


def build_channel_key(args: argparse.Namespace, region: str, session_name: str) -> str:
    if args.aws_key_type == AWS_KEY_TYPE_ROLE_ARN:
        return f"{args.role_arn}|{region}|{session_name}"
    if args.aws_key_type == AWS_KEY_TYPE_API_KEY:
        return f"{args.api_key}|{region}"
    if args.aws_key_type == AWS_KEY_TYPE_AKSK:
        return f"{args.access_key_id}|{args.secret_access_key}|{region}"
    raise ValueError(f"unsupported AWS key type: {args.aws_key_type}")


def build_auth_headers(args: argparse.Namespace) -> dict[str, str]:
    headers = {
        "Content-Type": "application/json",
        "New-Api-User": args.user_id,
        "User-Agent": "new-api-aws-channel-cli",
    }
    if args.cookie:
        headers["Cookie"] = args.cookie
    if args.access_token:
        token = args.access_token.strip()
        if token.lower().startswith("bearer "):
            token = token[7:].strip()
        headers["Authorization"] = token
    return headers


def post_channel(
    base_url: str,
    headers: dict[str, str],
    payload: dict[str, Any],
    timeout: int,
    retries: int = 0,
    retry_delay: float = 3.0,
) -> dict[str, Any]:
    body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    url = f"{base_url.rstrip('/')}/api/channel/"
    opener = urllib.request.build_opener(NoRedirectHandler)
    for attempt in range(retries + 1):
        request = urllib.request.Request(
            url,
            data=body,
            method="POST",
            headers=headers,
        )
        try:
            with opener.open(request, timeout=timeout) as response:
                data = response.read().decode("utf-8")
            return json.loads(data)
        except urllib.error.HTTPError as exc:
            if exc.code in {301, 302, 303, 307, 308}:
                location = exc.headers.get("Location", "")
                raise RuntimeError(
                    f"request was redirected to {location}; use the final base URL, "
                    "for example --base-url https://api.example.com"
                ) from exc
            if exc.code == 429 and attempt < retries:
                time.sleep(resolve_retry_delay(exc, retry_delay))
                continue
            raise
    raise RuntimeError("request retry loop exited unexpectedly")


def resolve_retry_delay(exc: urllib.error.HTTPError, fallback: float) -> float:
    retry_after = exc.headers.get("Retry-After")
    try:
        return float(retry_after) if retry_after else fallback
    except ValueError:
        return fallback


def request_json(
    base_url: str,
    path: str,
    headers: dict[str, str],
    timeout: int,
    method: str = "GET",
    retries: int = 0,
    retry_delay: float = 3.0,
) -> dict[str, Any]:
    url = f"{base_url.rstrip('/')}{path}"
    opener = urllib.request.build_opener(NoRedirectHandler)
    for attempt in range(retries + 1):
        request = urllib.request.Request(url, method=method, headers=headers)
        try:
            with opener.open(request, timeout=timeout) as response:
                data = response.read().decode("utf-8")
            return json.loads(data)
        except urllib.error.HTTPError as exc:
            if exc.code in {301, 302, 303, 307, 308}:
                location = exc.headers.get("Location", "")
                raise RuntimeError(
                    f"request was redirected to {location}; use the final base URL"
                ) from exc
            if exc.code == 429 and attempt < retries:
                time.sleep(resolve_retry_delay(exc, retry_delay))
                continue
            raise
    raise RuntimeError("request retry loop exited unexpectedly")


def find_channel_id(
    base_url: str,
    headers: dict[str, str],
    timeout: int,
    name: str,
    group: str,
    retries: int = 0,
    retry_delay: float = 3.0,
) -> int | None:
    query_params = {
        "keyword": name,
        "type": 33,
        "page_size": 50,
        "id_sort": "true",
    }
    if group:
        query_params["group"] = group
    query = urllib.parse.urlencode(query_params)
    result = request_json(
        base_url,
        f"/api/channel/search?{query}",
        headers,
        timeout,
        retries=retries,
        retry_delay=retry_delay,
    )
    if not result.get("success"):
        raise RuntimeError(result.get("message") or result)

    items = result.get("data", {}).get("items", [])
    for item in items:
        if item.get("name") == name and item.get("group") == group:
            channel_id = item.get("id")
            if isinstance(channel_id, int):
                return channel_id
    return None


def find_channel_records(
    base_url: str,
    headers: dict[str, str],
    timeout: int,
    name: str,
    group: str,
    retries: int = 0,
    retry_delay: float = 3.0,
) -> list[dict[str, Any]]:
    query_params = {
        "keyword": name,
        "type": 33,
        "page_size": 50,
        "id_sort": "true",
    }
    if group:
        query_params["group"] = group
    query = urllib.parse.urlencode(query_params)
    result = request_json(
        base_url,
        f"/api/channel/search?{query}",
        headers,
        timeout,
        retries=retries,
        retry_delay=retry_delay,
    )
    if not result.get("success"):
        raise RuntimeError(result.get("message") or result)

    records = []
    items = result.get("data", {}).get("items", [])
    for item in items:
        if (
            item.get("name") == name
            and item.get("group") == group
            and isinstance(item.get("id"), int)
        ):
            records.append(
                {
                    "id": item["id"],
                    "status": item.get("status"),
                }
            )
    return records


def get_channel_status(
    base_url: str,
    headers: dict[str, str],
    timeout: int,
    channel_id: int,
    retries: int = 0,
    retry_delay: float = 3.0,
) -> int | None:
    result = request_json(
        base_url,
        f"/api/channel/{channel_id}",
        headers,
        timeout,
        retries=retries,
        retry_delay=retry_delay,
    )
    if not result.get("success"):
        raise RuntimeError(result.get("message") or result)
    status = result.get("data", {}).get("status")
    return status if isinstance(status, int) else None


def test_channel(
    base_url: str,
    headers: dict[str, str],
    timeout: int,
    channel_id: int,
    test_model: str,
    retries: int = 0,
    retry_delay: float = 3.0,
) -> dict[str, Any]:
    params = {}
    if test_model:
        params["model"] = test_model
    query = urllib.parse.urlencode(params)
    suffix = f"?{query}" if query else ""
    return request_json(
        base_url,
        f"/api/channel/test/{channel_id}{suffix}",
        headers,
        timeout,
        retries=retries,
        retry_delay=retry_delay,
    )


def delete_channel(
    base_url: str,
    headers: dict[str, str],
    timeout: int,
    channel_id: int,
    retries: int = 0,
    retry_delay: float = 3.0,
) -> dict[str, Any]:
    return request_json(
        base_url,
        f"/api/channel/{channel_id}",
        headers,
        timeout,
        method="DELETE",
        retries=retries,
        retry_delay=retry_delay,
    )


def format_test_failure(result: dict[str, Any]) -> str:
    message = result.get("message") or result.get("error") or str(result)
    error_code = result.get("error_code")
    if error_code:
        return f"{message} ({error_code})"
    return str(message)


def test_and_maybe_delete_channel(
    args: argparse.Namespace,
    headers: dict[str, str],
    channel_name: str,
    channel_id: int,
) -> tuple[bool, str]:
    test_result = test_channel(
        args.base_url,
        headers,
        args.timeout,
        channel_id,
        args.test_model,
        retries=args.retries,
        retry_delay=args.retry_delay,
    )
    if test_result.get("success"):
        response_time = test_result.get("time")
        if isinstance(response_time, (int, float)):
            return True, f"test={response_time:.2f}s"
        return True, "test=passed"

    error = format_test_failure(test_result)
    try:
        status = get_channel_status(
            args.base_url,
            headers,
            args.timeout,
            channel_id,
            retries=args.retries,
            retry_delay=args.retry_delay,
        )
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError, RuntimeError):
        status = None
    if status is not None and status != CHANNEL_STATUS_ENABLED:
        return False, f"test failed: {error} kept disabled status={status}"

    keep_error_tokens = [
        "InvalidClientTokenId",
        "failed to refresh cached credentials",
        "STS: AssumeRole",
        "get identity",
    ]
    if test_result.get("error_code") == "model_price_error" or any(
        token in error for token in keep_error_tokens
    ):
        return False, f"test failed: {error} kept"
    deleted = ""
    if not args.keep_on_test_failure:
        delete_result = delete_channel(
            args.base_url,
            headers,
            args.timeout,
            channel_id,
            retries=args.retries,
            retry_delay=args.retry_delay,
        )
        deleted = " deleted" if delete_result.get("success") else " delete_failed"
    return False, f"test failed: {error}{deleted}"


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Create AWS channels under one New API group.",
    )
    parser.add_argument("--base-url", default=os.getenv("NEW_API_BASE_URL", ""))
    parser.add_argument("--cookie", default=os.getenv("NEW_API_COOKIE", ""))
    parser.add_argument("--access-token", default=os.getenv("NEW_API_ACCESS_TOKEN", ""))
    parser.add_argument("--user-id", default=os.getenv("NEW_API_USER_ID", ""))
    parser.add_argument(
        "--aws-key-type",
        "--key-type",
        choices=[AWS_KEY_TYPE_AKSK, AWS_KEY_TYPE_API_KEY, AWS_KEY_TYPE_ROLE_ARN],
        default=os.getenv("AWS_BEDROCK_KEY_TYPE", AWS_KEY_TYPE_ROLE_ARN),
        dest="aws_key_type",
        help="AWS credential mode stored in channel settings.aws_key_type.",
    )
    parser.add_argument("--role-arn", default=os.getenv("AWS_BEDROCK_ROLE_ARN", ""))
    parser.add_argument("--api-key", default=os.getenv("AWS_BEDROCK_API_KEY", ""))
    parser.add_argument("--access-key-id", default=os.getenv("AWS_ACCESS_KEY_ID", ""))
    parser.add_argument("--secret-access-key", default=os.getenv("AWS_SECRET_ACCESS_KEY", ""))
    parser.add_argument("--group", required=True)
    parser.add_argument("--regions", help="Comma-separated region list. Defaults to known accessible regions.")
    parser.add_argument("--region-file", help="One region per line. Overrides --regions.")
    parser.add_argument("--name-template", default="{region}")
    parser.add_argument("--session-name", default="{safe_region}")
    parser.add_argument("--preset", choices=["aws-claude", "aws-global-claude", "none"], default="aws-claude")
    parser.add_argument("--models", help="Comma-separated model list. Overrides preset models.")
    parser.add_argument("--model-mapping", help="JSON object string. Overrides preset mapping.")
    parser.add_argument("--priority", type=int, default=0)
    parser.add_argument("--weight", type=int, default=0)
    parser.add_argument(
        "--weight-template",
        choices=["none", *sorted(DEFAULT_REGION_WEIGHT_TEMPLATES)],
        default=os.getenv("AWS_BEDROCK_WEIGHT_TEMPLATE", "none"),
        help="Preset per-region channel weights; --weight is used as fallback for regions outside the template.",
    )
    parser.add_argument("--status", type=int, choices=[1, 2, 3], default=1)
    parser.add_argument("--auto-ban", type=int, choices=[0, 1], default=1)
    parser.add_argument("--tag", default="")
    parser.add_argument("--remark", default="Created by scripts/create_aws_role_channels.py")
    parser.add_argument("--test-model", default="")
    parser.add_argument("--test-after-create", action="store_true")
    parser.add_argument("--keep-on-test-failure", action="store_true")
    parser.add_argument("--allow-duplicate", action="store_true")
    parser.add_argument("--timeout", type=int, default=30)
    parser.add_argument("--retries", type=int, default=3)
    parser.add_argument("--retry-delay", type=float, default=5.0)
    parser.add_argument("--request-delay", type=float, default=1.0)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--continue-on-error", action="store_true")
    args = parser.parse_args()
    if (
        args.weight_template != "none"
        and args.weight_template not in DEFAULT_REGION_WEIGHT_TEMPLATES
    ):
        parser.error(
            "--weight-template/AWS_BEDROCK_WEIGHT_TEMPLATE must be one of: "
            + ", ".join(["none", *sorted(DEFAULT_REGION_WEIGHT_TEMPLATES)])
        )
    validate_default_weight_templates()

    missing = []
    if not args.base_url:
        missing.append("--base-url or NEW_API_BASE_URL")
    if args.aws_key_type == AWS_KEY_TYPE_ROLE_ARN and not args.role_arn:
        missing.append("--role-arn or AWS_BEDROCK_ROLE_ARN")
    if args.aws_key_type == AWS_KEY_TYPE_API_KEY and not args.api_key:
        missing.append("--api-key or AWS_BEDROCK_API_KEY")
    if args.aws_key_type == AWS_KEY_TYPE_AKSK:
        if not args.access_key_id:
            missing.append("--access-key-id or AWS_ACCESS_KEY_ID")
        if not args.secret_access_key:
            missing.append("--secret-access-key or AWS_SECRET_ACCESS_KEY")
    if not args.dry_run and not args.user_id:
        missing.append("--user-id or NEW_API_USER_ID")
    if missing or (not args.dry_run and not (args.cookie or args.access_token)):
        if not args.dry_run:
            missing.append("--cookie/NEW_API_COOKIE or --access-token/NEW_API_ACCESS_TOKEN")
        parser.error("missing required value(s): " + ", ".join(missing))

    regions = load_regions(args)
    models, model_mapping = resolve_models_and_mapping(args)
    if not regions:
        parser.error("no regions provided")
    if not models:
        parser.error("model list is empty; pass --models or use --preset aws-claude")

    payloads = [
        build_payload(args, region, index + 1, models, model_mapping)
        for index, region in enumerate(regions)
    ]

    if args.dry_run:
        print(json.dumps(payloads, ensure_ascii=False, indent=2))
        return 0

    ok = 0
    failed = 0
    auth_headers = build_auth_headers(args)
    for index, payload in enumerate(payloads):
        if index > 0 and args.request_delay > 0:
            time.sleep(args.request_delay)
        channel = payload["channel"]
        if not args.allow_duplicate:
            try:
                existing_channels = find_channel_records(
                    args.base_url,
                    auth_headers,
                    args.timeout,
                    channel["name"],
                    channel["group"],
                    retries=args.retries,
                    retry_delay=args.retry_delay,
                )
            except (
                urllib.error.URLError,
                TimeoutError,
                json.JSONDecodeError,
                RuntimeError,
            ) as exc:
                failed += 1
                print(f"[FAIL] {channel['name']}: duplicate check failed: {exc}", file=sys.stderr)
                if not args.continue_on_error:
                    return 1
                continue
            if existing_channels:
                disabled_existing = next(
                    (
                        record
                        for record in existing_channels
                        if record.get("status") != CHANNEL_STATUS_ENABLED
                    ),
                    None,
                )
                if disabled_existing:
                    print(
                        f"[SKIP] {channel['name']} already exists id={disabled_existing['id']} "
                        f"disabled status={disabled_existing.get('status')}"
                    )
                    continue

                existing_id = existing_channels[0]["id"]
                if not args.test_after_create:
                    print(f"[SKIP] {channel['name']} already exists id={existing_id}")
                    continue

                try:
                    test_ok, detail = test_and_maybe_delete_channel(
                        args,
                        auth_headers,
                        channel["name"],
                        existing_id,
                    )
                except (
                    urllib.error.URLError,
                    TimeoutError,
                    json.JSONDecodeError,
                    RuntimeError,
                ) as exc:
                    failed += 1
                    print(f"[FAIL] {channel['name']} existing id={existing_id}: test/delete error: {exc}", file=sys.stderr)
                    if not args.continue_on_error:
                        return 1
                    continue

                if test_ok:
                    print(f"[SKIP] {channel['name']} already exists id={existing_id} {detail}")
                else:
                    failed += 1
                    print(f"[FAIL] {channel['name']} existing id={existing_id}: {detail}", file=sys.stderr)
                    if not args.continue_on_error:
                        return 1
                continue

        try:
            result = post_channel(
                args.base_url,
                auth_headers,
                payload,
                args.timeout,
                retries=args.retries,
                retry_delay=args.retry_delay,
            )
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError, RuntimeError) as exc:
            failed += 1
            print(f"[FAIL] {channel['name']}: {exc}", file=sys.stderr)
            if not args.continue_on_error:
                return 1
            continue

        if result.get("success") and "data" not in result:
            if not args.test_after_create:
                ok += 1
                print(f"[OK] {channel['name']}")
                continue

            channel_id = None
            try:
                channel_id = find_channel_id(
                    args.base_url,
                    auth_headers,
                    args.timeout,
                    channel["name"],
                    channel["group"],
                    retries=args.retries,
                    retry_delay=args.retry_delay,
                )
                if channel_id is None:
                    raise RuntimeError("created channel was not found by name")

                test_ok, detail = test_and_maybe_delete_channel(
                    args,
                    auth_headers,
                    channel["name"],
                    channel_id,
                )
                if test_ok:
                    ok += 1
                    print(f"[OK] {channel['name']} {detail}")
                    continue

                failed += 1
                print(f"[FAIL] {channel['name']}: {detail}", file=sys.stderr)
                if not args.continue_on_error:
                    return 1
                continue
            except (
                urllib.error.URLError,
                TimeoutError,
                json.JSONDecodeError,
                RuntimeError,
            ) as exc:
                failed += 1
                deleted = ""
                if channel_id is not None and not args.keep_on_test_failure:
                    try:
                        delete_result = delete_channel(
                            args.base_url,
                            auth_headers,
                            args.timeout,
                            channel_id,
                            retries=args.retries,
                            retry_delay=args.retry_delay,
                        )
                        deleted = " deleted" if delete_result.get("success") else " delete_failed"
                    except (
                        urllib.error.URLError,
                        TimeoutError,
                        json.JSONDecodeError,
                        RuntimeError,
                    ) as delete_exc:
                        deleted = f" delete_error={delete_exc}"
                print(f"[FAIL] {channel['name']}: test/delete error: {exc}", file=sys.stderr)
                if deleted:
                    print(f"[INFO] {channel['name']} cleanup:{deleted}", file=sys.stderr)
                if not args.continue_on_error:
                    return 1
                continue

        failed += 1
        print(
            f"[FAIL] {channel['name']}: {result.get('message') or result}",
            file=sys.stderr,
        )
        if not args.continue_on_error:
            return 1

    print(f"Done. success={ok}, failed={failed}")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())

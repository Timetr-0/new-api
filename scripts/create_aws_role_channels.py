#!/usr/bin/env python3
"""Create AWS Bedrock Role ARN channels through the New API admin API."""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from typing import Any


DEFAULT_ACCESSIBLE_REGIONS = [
    "ap-south-2",
    "ap-south-1",
    "eu-south-1",
    "eu-south-2",
    "il-central-1",
    "ca-central-1",
    "ap-east-2",
    "mx-central-1",
    "eu-central-1",
    "eu-central-2",
    "us-west-1",
    "us-west-2",
    "af-south-1",
    "eu-north-1",
    "eu-west-3",
    "eu-west-2",
    "eu-west-1",
    "ap-northeast-3",
    "ap-northeast-2",
    "ap-northeast-1",
    "sa-east-1",
    "ca-west-1",
    "ap-southeast-1",
    "ap-southeast-2",
    "ap-southeast-3",
    "ap-southeast-4",
    "us-east-1",
    "ap-southeast-5",
    "ap-southeast-6",
    "us-east-2",
    "ap-southeast-7",
]

AWS_GLOBAL_CLAUDE_MODEL_MAPPINGS = [
    ("claude-3-sonnet-20240229", "global.anthropic.claude-3-sonnet-20240229-v1:0"),
    ("claude-3-opus-20240229", "global.anthropic.claude-3-opus-20240229-v1:0"),
    ("claude-3-haiku-20240307", "global.anthropic.claude-3-haiku-20240307-v1:0"),
    ("claude-3-5-sonnet-20240620", "global.anthropic.claude-3-5-sonnet-20240620-v1:0"),
    ("claude-3-5-sonnet-20241022", "global.anthropic.claude-3-5-sonnet-20241022-v2:0"),
    ("claude-3-5-haiku-20241022", "global.anthropic.claude-3-5-haiku-20241022-v1:0"),
    ("claude-3-7-sonnet-20250219", "global.anthropic.claude-3-7-sonnet-20250219-v1:0"),
    ("claude-sonnet-4-20250514", "global.anthropic.claude-sonnet-4-20250514-v1:0"),
    ("claude-opus-4-20250514", "global.anthropic.claude-opus-4-20250514-v1:0"),
    ("claude-opus-4-1-20250805", "global.anthropic.claude-opus-4-1-20250805-v1:0"),
    ("claude-sonnet-4-5-20250929", "global.anthropic.claude-sonnet-4-5-20250929-v1:0"),
    ("claude-sonnet-4-6", "global.anthropic.claude-sonnet-4-6"),
    ("claude-haiku-4-5-20251001", "global.anthropic.claude-haiku-4-5-20251001-v1:0"),
    ("claude-opus-4-5-20251101", "global.anthropic.claude-opus-4-5-20251101-v1:0"),
    ("claude-opus-4-6", "global.anthropic.claude-opus-4-6-v1"),
    ("claude-opus-4-6-v1", "global.anthropic.claude-opus-4-6-v1"),
    ("claude-opus-4-7", "global.anthropic.claude-opus-4-7"),
    ("claude-opus-4-8", "global.anthropic.claude-opus-4-8"),
]


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
    elif args.preset == "aws-global-claude":
        models = unique_csv(
            [item for pair in AWS_GLOBAL_CLAUDE_MODEL_MAPPINGS for item in pair]
        )
    else:
        models = ""

    if args.model_mapping:
        mapping = json.dumps(json.loads(args.model_mapping), separators=(",", ":"))
    elif args.preset == "aws-global-claude":
        mapping = json.dumps(
            dict(AWS_GLOBAL_CLAUDE_MODEL_MAPPINGS),
            separators=(",", ":"),
        )
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
    channel: dict[str, Any] = {
        "name": args.name_template.format(
            region=region,
            safe_region=safe_region,
            index=index,
        ),
        "type": 33,
        "base_url": None,
        "key": f"{args.role_arn}|{region}|{session_name}",
        "openai_organization": None,
        "models": models,
        "group": args.group,
        "model_mapping": model_mapping,
        "priority": args.priority,
        "weight": args.weight,
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
        "settings": json.dumps({"aws_key_type": "role_arn"}, separators=(",", ":")),
        "other": "",
    }
    return {"mode": "single", "channel": channel}


def build_auth_headers(args: argparse.Namespace) -> dict[str, str]:
    headers = {
        "Content-Type": "application/json",
        "New-Api-User": args.user_id,
        "User-Agent": "new-api-aws-role-channel-cli",
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
) -> dict[str, Any]:
    body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    request = urllib.request.Request(
        f"{base_url.rstrip('/')}/api/channel/",
        data=body,
        method="POST",
        headers=headers,
    )
    opener = urllib.request.build_opener(NoRedirectHandler)
    try:
        with opener.open(request, timeout=timeout) as response:
            data = response.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        if exc.code in {301, 302, 303, 307, 308}:
            location = exc.headers.get("Location", "")
            raise RuntimeError(
                f"request was redirected to {location}; use the final base URL, "
                "for example --base-url https://api.example.com"
            ) from exc
        raise
    return json.loads(data)


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Create AWS Role ARN channels under one New API group.",
    )
    parser.add_argument("--base-url", default=os.getenv("NEW_API_BASE_URL", ""))
    parser.add_argument("--cookie", default=os.getenv("NEW_API_COOKIE", ""))
    parser.add_argument("--access-token", default=os.getenv("NEW_API_ACCESS_TOKEN", ""))
    parser.add_argument("--user-id", default=os.getenv("NEW_API_USER_ID", ""))
    parser.add_argument("--role-arn", default=os.getenv("AWS_BEDROCK_ROLE_ARN", ""))
    parser.add_argument("--group", required=True)
    parser.add_argument("--regions", help="Comma-separated region list. Defaults to known accessible regions.")
    parser.add_argument("--region-file", help="One region per line. Overrides --regions.")
    parser.add_argument("--name-template", default="bedrock-{region}")
    parser.add_argument("--session-name", default="new-api-{safe_region}")
    parser.add_argument("--preset", choices=["aws-global-claude", "none"], default="aws-global-claude")
    parser.add_argument("--models", help="Comma-separated model list. Overrides preset models.")
    parser.add_argument("--model-mapping", help="JSON object string. Overrides preset mapping.")
    parser.add_argument("--priority", type=int, default=0)
    parser.add_argument("--weight", type=int, default=0)
    parser.add_argument("--status", type=int, choices=[1, 2, 3], default=1)
    parser.add_argument("--auto-ban", type=int, choices=[0, 1], default=1)
    parser.add_argument("--tag", default="")
    parser.add_argument("--remark", default="Created by scripts/create_aws_role_channels.py")
    parser.add_argument("--test-model", default="")
    parser.add_argument("--timeout", type=int, default=30)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--continue-on-error", action="store_true")
    args = parser.parse_args()

    missing = []
    if not args.base_url:
        missing.append("--base-url or NEW_API_BASE_URL")
    if not args.role_arn:
        missing.append("--role-arn or AWS_BEDROCK_ROLE_ARN")
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
        parser.error("model list is empty; pass --models or use --preset aws-global-claude")

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
    for payload in payloads:
        channel = payload["channel"]
        try:
            result = post_channel(args.base_url, auth_headers, payload, args.timeout)
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError, RuntimeError) as exc:
            failed += 1
            print(f"[FAIL] {channel['name']}: {exc}", file=sys.stderr)
            if not args.continue_on_error:
                return 1
            continue

        if result.get("success") and "data" not in result:
            ok += 1
            print(f"[OK] {channel['name']}")
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

#!/usr/bin/env python3
"""Send paced hello requests to verify AWS Bedrock upstream rate limiting."""

from __future__ import annotations

import argparse
import concurrent.futures
import http.server
import json
import os
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any


@dataclass
class Result:
    index: int
    scheduled_at: float
    started_at: float
    ended_at: float
    status: int | None
    body: str
    error: str


class MockBedrockState:
    def __init__(self, response_delay: float) -> None:
        self.response_delay = response_delay
        self.lock = threading.Lock()
        self.arrivals: list[float] = []

    def record_arrival(self) -> int:
        with self.lock:
            self.arrivals.append(time.monotonic())
            return len(self.arrivals)


def start_mock_bedrock_server(host: str, port: int, response_delay: float):
    state = MockBedrockState(response_delay)

    class MockBedrockHandler(http.server.BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def do_POST(self) -> None:
            length = int(self.headers.get("Content-Length", "0") or "0")
            if length > 0:
                self.rfile.read(length)
            request_id = state.record_arrival()
            if state.response_delay > 0:
                time.sleep(state.response_delay)

            body = json.dumps(
                {
                    "id": f"msg_mock_{request_id}",
                    "type": "message",
                    "role": "assistant",
                    "model": "mock-bedrock",
                    "content": [{"type": "text", "text": "hello"}],
                    "stop_reason": "end_turn",
                    "usage": {"input_tokens": 1, "output_tokens": 1},
                },
                separators=(",", ":"),
            ).encode("utf-8")

            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.send_header("x-amzn-requestid", f"mock-{request_id}")
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, format: str, *args: Any) -> None:
            return

    server = http.server.ThreadingHTTPServer((host, port), MockBedrockHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    bind_host, bind_port = server.server_address[:2]
    display_host = "127.0.0.1" if bind_host in {"", "0.0.0.0"} else bind_host
    return server, state, f"http://{display_host}:{bind_port}"


def build_admin_headers(args: argparse.Namespace) -> dict[str, str]:
    headers = {
        "Content-Type": "application/json",
        "User-Agent": "new-api-bedrock-rate-limit-test/1.0",
    }
    if args.admin_user_id:
        headers["New-Api-User"] = args.admin_user_id
    if args.admin_cookie:
        headers["Cookie"] = args.admin_cookie
    if args.admin_access_token:
        token = args.admin_access_token.strip()
        if token.lower().startswith("bearer "):
            token = token[7:].strip()
        headers["Authorization"] = token
    return headers


def admin_request(
    args: argparse.Namespace,
    method: str,
    path: str,
    payload: dict[str, Any] | None = None,
) -> dict[str, Any]:
    body = None
    if payload is not None:
        body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    request = urllib.request.Request(
        args.base_url.rstrip("/") + path,
        data=body,
        method=method,
        headers=build_admin_headers(args),
    )
    with urllib.request.urlopen(request, timeout=args.timeout) as response:
        data = response.read().decode("utf-8")
    return json.loads(data)


def create_mock_channel(args: argparse.Namespace, mock_base_url: str) -> None:
    if not args.admin_access_token and not args.admin_cookie:
        raise RuntimeError("--create-mock-channel requires --admin-access-token or --admin-cookie")
    if not args.group:
        raise RuntimeError("--create-mock-channel requires --group")

    channel = {
        "name": args.mock_channel_name,
        "type": 33,
        "base_url": mock_base_url,
        "key": f"{args.mock_api_key}|{args.mock_region}",
        "openai_organization": None,
        "models": args.model,
        "group": args.group,
        "model_mapping": None,
        "priority": args.mock_channel_priority,
        "weight": args.mock_channel_weight,
        "test_model": None,
        "auto_ban": 0,
        "status": 1,
        "status_code_mapping": None,
        "tag": args.mock_channel_tag or None,
        "remark": "Created by scripts/test_bedrock_upstream_rate_limit.py",
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
        "settings": json.dumps({"aws_key_type": "api_key"}, separators=(",", ":")),
        "other": "",
    }
    payload = {
        "mode": "single",
        "channel": channel,
    }
    result = admin_request(args, "POST", "/api/channel/", payload)
    if not result.get("success"):
        raise RuntimeError(f"create mock channel failed: {result.get('message') or result}")
    print(
        f"created mock AWS channel name={args.mock_channel_name!r} "
        f"model={args.model!r} base_url={mock_base_url}"
    )


def find_mock_channel_ids(args: argparse.Namespace) -> list[int]:
    query = urllib.parse.urlencode(
        {
            "keyword": args.mock_channel_name,
            "group": args.group,
            "model": args.model,
            "type": 33,
            "page_size": 100,
        }
    )
    result = admin_request(args, "GET", f"/api/channel/search?{query}")
    if not result.get("success"):
        raise RuntimeError(f"find mock channel failed: {result.get('message') or result}")

    data = result.get("data") or {}
    items = data.get("items") or []
    ids: list[int] = []
    for item in items:
        if item.get("name") == args.mock_channel_name and item.get("type") == 33:
            channel_id = item.get("id")
            if isinstance(channel_id, int):
                ids.append(channel_id)
    return ids


def delete_mock_channels(args: argparse.Namespace) -> None:
    for channel_id in find_mock_channel_ids(args):
        result = admin_request(args, "DELETE", f"/api/channel/{channel_id}")
        if not result.get("success"):
            raise RuntimeError(
                f"delete mock channel {channel_id} failed: {result.get('message') or result}"
            )
        print(f"deleted mock AWS channel id={channel_id}")


def build_payload(args: argparse.Namespace) -> dict[str, Any]:
    if args.format == "claude":
        return {
            "model": args.model,
            "max_tokens": args.max_tokens,
            "messages": [
                {
                    "role": "user",
                    "content": args.message,
                }
            ],
            "metadata": {
                "user_id": args.metadata_user_id,
            },
        }

    return {
        "model": args.model,
        "max_tokens": args.max_tokens,
        "messages": [
            {
                "role": "user",
                "content": args.message,
            }
        ],
    }


def endpoint_path(request_format: str) -> str:
    if request_format == "claude":
        return "/v1/messages"
    return "/v1/chat/completions"


def post_once(args: argparse.Namespace, index: int, scheduled_at: float) -> Result:
    now = time.monotonic()
    if scheduled_at > now:
        time.sleep(scheduled_at - now)

    started_at = time.monotonic()
    payload = build_payload(args)
    data = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    url = args.base_url.rstrip("/") + endpoint_path(args.format)
    headers = {
        "Authorization": "Bearer " + args.api_key,
        "Content-Type": "application/json",
        "User-Agent": "new-api-bedrock-rate-limit-test/1.0",
    }
    if args.affinity_key:
        headers["X-Affinity-Key"] = args.affinity_key

    request = urllib.request.Request(url, data=data, headers=headers, method="POST")
    status: int | None = None
    body = ""
    error = ""
    try:
        with urllib.request.urlopen(request, timeout=args.timeout) as response:
            status = response.status
            body = response.read(args.body_bytes).decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        status = exc.code
        body = exc.read(args.body_bytes).decode("utf-8", errors="replace")
        error = exc.reason
    except Exception as exc:  # noqa: BLE001
        error = str(exc)

    return Result(
        index=index,
        scheduled_at=scheduled_at,
        started_at=started_at,
        ended_at=time.monotonic(),
        status=status,
        body=body,
        error=error,
    )


def short_body(body: str) -> str:
    text = " ".join(body.split())
    if len(text) > 180:
        return text[:177] + "..."
    return text


def max_sliding_count(times: list[float], window: float) -> int:
    max_count = 0
    left = 0
    for right, value in enumerate(times):
        while left <= right and value - times[left] >= window:
            left += 1
        max_count = max(max_count, right - left + 1)
    return max_count


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Send hello requests at a fixed RPM to test Bedrock upstream throttling."
    )
    parser.add_argument(
        "--base-url",
        default=os.getenv("NEW_API_BASE_URL", "http://localhost:3000"),
        help="New API base URL, default: NEW_API_BASE_URL or http://localhost:3000.",
    )
    parser.add_argument(
        "--api-key",
        default=os.getenv("NEW_API_API_KEY", os.getenv("NEW_API_KEY", "")),
        help="New API token, default: NEW_API_API_KEY or NEW_API_KEY.",
    )
    parser.add_argument(
        "--model",
        default=os.getenv("NEW_API_TEST_MODEL", "claude-sonnet-4-6"),
        help="Model routed to AWS Bedrock.",
    )
    parser.add_argument(
        "--format",
        choices=["openai", "claude"],
        default="openai",
        help="Request format, default: openai.",
    )
    parser.add_argument("--message", default="hello")
    parser.add_argument("--count", type=int, default=30, help="Total requests.")
    parser.add_argument("--rpm", type=float, default=60.0, help="Start rate.")
    parser.add_argument("--max-workers", type=int, default=64)
    parser.add_argument("--timeout", type=float, default=120.0)
    parser.add_argument("--max-tokens", type=int, default=8)
    parser.add_argument("--body-bytes", type=int, default=2048)
    parser.add_argument("--affinity-key", default="")
    parser.add_argument("--metadata-user-id", default="bedrock-rate-limit-test")
    parser.add_argument(
        "--mock-upstream",
        action="store_true",
        help="Start a local fake Bedrock upstream and print its base_url.",
    )
    parser.add_argument("--mock-host", default="127.0.0.1")
    parser.add_argument("--mock-port", type=int, default=0)
    parser.add_argument(
        "--mock-public-url",
        default="",
        help="Base URL the New API process can reach. Defaults to the local mock URL.",
    )
    parser.add_argument("--mock-response-delay", type=float, default=0.0)
    parser.add_argument(
        "--create-mock-channel",
        action="store_true",
        help="Create a temporary AWS API-key channel pointing to the mock upstream.",
    )
    parser.add_argument("--admin-access-token", default=os.getenv("NEW_API_ACCESS_TOKEN", ""))
    parser.add_argument("--admin-cookie", default=os.getenv("NEW_API_COOKIE", ""))
    parser.add_argument("--admin-user-id", default=os.getenv("NEW_API_USER_ID", ""))
    parser.add_argument("--group", default=os.getenv("NEW_API_TEST_GROUP", "default"))
    parser.add_argument(
        "--mock-channel-name",
        default=f"bedrock-rate-limit-test-{int(time.time())}",
    )
    parser.add_argument("--mock-channel-priority", type=int, default=999999)
    parser.add_argument("--mock-channel-weight", type=int, default=100)
    parser.add_argument("--mock-channel-tag", default="rate-limit-test")
    parser.add_argument("--mock-api-key", default="mock-bedrock-api-key")
    parser.add_argument("--mock-region", default="us-east-1")
    parser.add_argument(
        "--keep-mock-channel",
        action="store_true",
        help="Keep the created mock channel after the test. By default it is deleted.",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if not args.api_key:
        print("missing --api-key or NEW_API_API_KEY", file=sys.stderr)
        return 2
    if args.count < 1:
        print("--count must be positive", file=sys.stderr)
        return 2
    if args.rpm <= 0:
        print("--rpm must be positive", file=sys.stderr)
        return 2

    mock_server = None
    mock_state = None
    created_mock_channel = False
    exit_code = 0
    if args.mock_upstream or args.create_mock_channel:
        mock_server, mock_state, local_mock_url = start_mock_bedrock_server(
            args.mock_host,
            args.mock_port,
            args.mock_response_delay,
        )
        mock_base_url = args.mock_public_url or local_mock_url
        print(f"mock_bedrock_local_url={local_mock_url}")
        print(f"mock_bedrock_channel_base_url={mock_base_url}")
        if args.create_mock_channel:
            create_mock_channel(args, mock_base_url)
            created_mock_channel = True

    try:
        interval = 60.0 / args.rpm
        max_workers = max(1, min(args.max_workers, args.count))
        start = time.monotonic()

        print(
            f"base_url={args.base_url} format={args.format} model={args.model} "
            f"count={args.count} rpm={args.rpm:g} interval={interval:.3f}s"
        )
        print("idx status start_s latency_s body")

        results: list[Result] = []
        with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
            futures = []
            for index in range(1, args.count + 1):
                scheduled_at = start + (index - 1) * interval
                futures.append(executor.submit(post_once, args, index, scheduled_at))

            for future in concurrent.futures.as_completed(futures):
                result = future.result()
                results.append(result)
                start_s = result.started_at - start
                latency_s = result.ended_at - result.started_at
                status = result.status if result.status is not None else "ERR"
                detail = short_body(result.body or result.error)
                print(f"{result.index:03d} {status} {start_s:8.2f} {latency_s:9.2f} {detail}")

        results.sort(key=lambda item: item.index)
        ok_count = sum(1 for item in results if item.status is not None and 200 <= item.status < 300)
        rate_limited_count = sum(1 for item in results if item.status == 429)
        error_count = len(results) - ok_count - rate_limited_count
        latencies = sorted(item.ended_at - item.started_at for item in results)
        p50 = latencies[len(latencies) // 2]
        p95 = latencies[min(len(latencies) - 1, int(len(latencies) * 0.95))]

        print()
        print(
            f"summary total={len(results)} ok={ok_count} "
            f"rate_limited_429={rate_limited_count} other={error_count} "
            f"p50_latency={p50:.2f}s p95_latency={p95:.2f}s"
        )
        print(
            "expected: with AWS Bedrock base limit=10/min and one enabled AWS channel "
            "in the effective group, only about the first 10 should be fast. "
            "With N enabled AWS channels, the effective limit is 10*N/min."
        )
        if mock_state is not None:
            with mock_state.lock:
                arrivals = list(mock_state.arrivals)
            if arrivals:
                first = arrivals[0]
                offsets = [value - first for value in arrivals]
                preview = ", ".join(f"{offset:.2f}" for offset in offsets[:20])
                if len(offsets) > 20:
                    preview += ", ..."
                print(
                    f"mock_upstream_received={len(arrivals)} "
                    f"max_1s_window={max_sliding_count(arrivals, 1.0)} "
                    f"max_60s_window={max_sliding_count(arrivals, 60.0)}"
                )
                print(f"mock_upstream_arrival_offsets_s=[{preview}]")
            else:
                print("mock_upstream_received=0")
    finally:
        if created_mock_channel and not args.keep_mock_channel:
            try:
                delete_mock_channels(args)
            except Exception as exc:  # noqa: BLE001
                print(f"cleanup mock channel failed: {exc}", file=sys.stderr)
                exit_code = 1
        if mock_server is not None:
            mock_server.shutdown()
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())

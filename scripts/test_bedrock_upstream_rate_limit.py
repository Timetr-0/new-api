#!/usr/bin/env python3
"""Send paced hello requests to verify AWS Bedrock upstream rate limiting."""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import sys
import time
import urllib.error
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
        "expected: with AWS Bedrock limit=10/min and send rate=60/min, "
        "only about the first 10 should be fast; later requests should wait or return 429."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

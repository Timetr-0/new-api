#!/usr/bin/env python3
"""Send paced hello requests to verify AWS Bedrock upstream rate limiting."""

from __future__ import annotations

import argparse
import concurrent.futures
import http.server
import json
import os
import subprocess
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
    target_label: str
    group: str
    scheduled_at: float
    started_at: float
    ended_at: float
    status: int | None
    body: str
    error: str


@dataclass
class RequestTarget:
    label: str
    group: str
    api_key: str


@dataclass
class ResourceSample:
    elapsed_s: float
    cpu_percent: float | None
    rss_bytes: int | None
    backlog_estimate: int
    completed: int
    ok_count: int
    rate_limited_count: int
    other_count: int
    mock_received: int | None


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


def create_mock_channel(
    args: argparse.Namespace,
    mock_base_url: str,
    group: str,
    channel_name: str,
) -> int | None:
    if not args.admin_access_token and not args.admin_cookie:
        raise RuntimeError("--create-mock-channel requires --admin-access-token or --admin-cookie")
    if not group:
        raise RuntimeError("--create-mock-channel requires --group or --groups")

    channel = {
        "name": channel_name,
        "type": 33,
        "base_url": mock_base_url,
        "key": f"{args.mock_api_key}|{args.mock_region}",
        "openai_organization": None,
        "models": args.model,
        "group": group,
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
    channel_ids = find_mock_channel_ids(args, group, channel_name)
    channel_id = max(channel_ids) if channel_ids else None
    id_text = f" id={channel_id}" if channel_id is not None else ""
    print(
        f"created mock AWS channel{id_text} name={channel_name!r} group={group!r} "
        f"model={args.model!r} base_url={mock_base_url}"
    )
    return channel_id


def fix_channel_abilities(args: argparse.Namespace) -> None:
    result = admin_request(args, "POST", "/api/channel/fix")
    if not result.get("success"):
        raise RuntimeError(f"fix channel abilities failed: {result.get('message') or result}")
    data = result.get("data") or {}
    print(
        "refreshed channel abilities "
        f"success={data.get('success', '?')} fails={data.get('fails', '?')}"
    )


def find_mock_channel_ids(args: argparse.Namespace, group: str, channel_name: str) -> list[int]:
    query = urllib.parse.urlencode(
        {
            "keyword": channel_name,
            "group": group,
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
        if item.get("name") == channel_name and item.get("type") == 33:
            channel_id = item.get("id")
            if isinstance(channel_id, int):
                ids.append(channel_id)
    return ids


def delete_mock_channels(args: argparse.Namespace, channels: list[tuple[str, str]]) -> None:
    for group, channel_name in channels:
        for channel_id in find_mock_channel_ids(args, group, channel_name):
            result = admin_request(args, "DELETE", f"/api/channel/{channel_id}")
            if not result.get("success"):
                raise RuntimeError(
                    f"delete mock channel {channel_id} failed: {result.get('message') or result}"
                )
            print(f"deleted mock AWS channel id={channel_id} group={group!r}")


def parse_csv(value: str) -> list[str]:
    return [item.strip() for item in value.split(",") if item.strip()]


def parse_size(value: str) -> int:
    text = value.strip().lower()
    if not text:
        return 0
    multipliers = {
        "k": 1024,
        "kb": 1024,
        "m": 1024 * 1024,
        "mb": 1024 * 1024,
        "g": 1024 * 1024 * 1024,
        "gb": 1024 * 1024 * 1024,
    }
    for suffix, multiplier in sorted(multipliers.items(), key=lambda item: len(item[0]), reverse=True):
        if text.endswith(suffix):
            return int(float(text[: -len(suffix)].strip()) * multiplier)
    return int(float(text))


def build_large_message(args: argparse.Namespace) -> str:
    if args.payload_bytes:
        target_bytes = parse_size(args.payload_bytes)
    elif args.payload_tokens:
        target_bytes = int(args.payload_tokens * args.bytes_per_token)
    else:
        return args.message

    if target_bytes <= 0:
        return args.message

    seed = args.message or "hello"
    if not seed.endswith(" "):
        seed += " "
    seed_bytes = seed.encode("utf-8")
    repeats = target_bytes // len(seed_bytes) + 1
    return (seed_bytes * repeats)[:target_bytes].decode("utf-8", errors="ignore")


def sanitize_name_part(value: str) -> str:
    sanitized = "".join(char if char.isalnum() or char in {"-", "_"} else "-" for char in value)
    return sanitized.strip("-") or "group"


def build_request_targets(args: argparse.Namespace) -> list[RequestTarget]:
    groups = parse_csv(args.groups) if args.groups else parse_csv(args.group)
    api_keys = parse_csv(args.api_keys) if args.api_keys else parse_csv(args.api_key)

    if not groups:
        raise ValueError("missing --group or --groups")
    if not api_keys:
        raise ValueError("missing --api-key/--api-keys or NEW_API_API_KEY/NEW_API_API_KEYS")

    if len(groups) == 1 and len(api_keys) > 1:
        return [
            RequestTarget(label=f"{groups[0]}#{index + 1}", group=groups[0], api_key=api_key)
            for index, api_key in enumerate(api_keys)
        ]

    if len(groups) != len(api_keys):
        raise ValueError("--groups and --api-keys must have the same count for multi-group tests")

    return [
        RequestTarget(label=group, group=group, api_key=api_key)
        for group, api_key in zip(groups, api_keys)
    ]


def append_specific_channel_id(api_key: str, channel_id: int) -> str:
    return f"{api_key.strip()}-{channel_id}"


def unique_groups(targets: list[RequestTarget]) -> list[str]:
    seen: set[str] = set()
    groups: list[str] = []
    for target in targets:
        if target.group in seen:
            continue
        seen.add(target.group)
        groups.append(target.group)
    return groups


def mock_channel_name_for_group(args: argparse.Namespace, group: str, group_count: int) -> str:
    if group_count == 1:
        return args.mock_channel_name
    return f"{args.mock_channel_name}-{sanitize_name_part(group)}"


def mock_channel_name_for_index(base_name: str, index: int, count: int) -> str:
    if count == 1:
        return base_name
    return f"{base_name}-{index + 1}"


def build_payload(args: argparse.Namespace) -> dict[str, Any]:
    message = args.generated_message if args.generated_message is not None else args.message
    if args.format == "claude":
        return {
            "model": args.model,
            "max_tokens": args.max_tokens,
            "messages": [
                {
                    "role": "user",
                    "content": message,
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
                "content": message,
            }
        ],
    }


def endpoint_path(request_format: str) -> str:
    if request_format == "claude":
        return "/v1/messages"
    return "/v1/chat/completions"


def post_once(
    args: argparse.Namespace,
    target: RequestTarget,
    index: int,
    scheduled_at: float,
) -> Result:
    now = time.monotonic()
    if scheduled_at > now:
        time.sleep(scheduled_at - now)

    started_at = time.monotonic()
    payload = build_payload(args)
    data = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    url = args.base_url.rstrip("/") + endpoint_path(args.format)
    headers = {
        "Authorization": "Bearer " + target.api_key,
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
        target_label=target.label,
        group=target.group,
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


def format_bytes(value: int | None) -> str:
    if value is None:
        return "?"
    units = ["B", "KiB", "MiB", "GiB"]
    size = float(value)
    for unit in units:
        if size < 1024 or unit == units[-1]:
            return f"{size:.1f}{unit}"
        size /= 1024
    return f"{size:.1f}GiB"


def resolve_process_pid(args: argparse.Namespace) -> int | None:
    if args.monitor_pid > 0:
        return args.monitor_pid
    if args.monitor_pid_file:
        try:
            return int(open(args.monitor_pid_file, encoding="utf-8").read().strip())
        except Exception as exc:  # noqa: BLE001
            print(f"process_monitor: cannot read pid file: {exc}", file=sys.stderr)
            return None
    if not args.monitor_process_name:
        return None
    try:
        output = subprocess.check_output(
            ["pgrep", "-n", "-f", args.monitor_process_name],
            text=True,
            stderr=subprocess.DEVNULL,
        ).strip()
    except Exception:
        return None
    return int(output) if output else None


def read_process_stats(pid: int, previous: tuple[float, int] | None) -> tuple[float | None, int | None, tuple[float, int] | None]:
    try:
        with open(f"/proc/{pid}/stat", encoding="utf-8") as stat_file:
            stat = stat_file.read().split()
        utime_ticks = int(stat[13])
        stime_ticks = int(stat[14])
        total_ticks = utime_ticks + stime_ticks
        now = time.monotonic()

        rss_bytes = None
        with open(f"/proc/{pid}/status", encoding="utf-8") as status_file:
            for line in status_file:
                if line.startswith("VmRSS:"):
                    rss_bytes = int(line.split()[1]) * 1024
                    break

        cpu_percent = None
        if previous is not None:
            previous_time, previous_ticks = previous
            elapsed = now - previous_time
            if elapsed > 0:
                ticks_per_second = os.sysconf(os.sysconf_names["SC_CLK_TCK"])
                cpu_percent = ((total_ticks - previous_ticks) / ticks_per_second) / elapsed * 100
        return cpu_percent, rss_bytes, (now, total_ticks)
    except FileNotFoundError:
        return None, None, None


def monitor_resources(
    args: argparse.Namespace,
    start: float,
    results: list[Result],
    results_lock: threading.Lock,
    stop_event: threading.Event,
    mock_state: MockBedrockState | None,
    samples: list[ResourceSample],
) -> None:
    pid = resolve_process_pid(args)
    if pid is None and (args.monitor_pid or args.monitor_pid_file or args.monitor_process_name):
        print("process_monitor: target process not found", file=sys.stderr)

    previous_stats: tuple[float, int] | None = None
    while not stop_event.wait(args.resource_sample_interval):
        elapsed = time.monotonic() - start
        with results_lock:
            completed = len(results)
            ok_count = sum(1 for item in results if item.status is not None and 200 <= item.status < 300)
            rate_limited_count = sum(1 for item in results if item.status == 429)
        other_count = completed - ok_count - rate_limited_count
        scheduled = min(args.count, int(elapsed / (60.0 / args.rpm)) + 1)
        backlog_estimate = max(0, scheduled - completed)
        mock_received = None
        if mock_state is not None:
            with mock_state.lock:
                mock_received = len(mock_state.arrivals)

        cpu_percent = None
        rss_bytes = None
        if pid is not None:
            cpu_percent, rss_bytes, previous_stats = read_process_stats(pid, previous_stats)

        sample = ResourceSample(
            elapsed_s=elapsed,
            cpu_percent=cpu_percent,
            rss_bytes=rss_bytes,
            backlog_estimate=backlog_estimate,
            completed=completed,
            ok_count=ok_count,
            rate_limited_count=rate_limited_count,
            other_count=other_count,
            mock_received=mock_received,
        )
        samples.append(sample)
        cpu_text = "?" if sample.cpu_percent is None else f"{sample.cpu_percent:.1f}%"
        mock_text = "?" if sample.mock_received is None else str(sample.mock_received)
        print(
            "resource_sample "
            f"t={sample.elapsed_s:.1f}s cpu={cpu_text} rss={format_bytes(sample.rss_bytes)} "
            f"scheduled_backlog={sample.backlog_estimate} completed={sample.completed} "
            f"ok={sample.ok_count} 429={sample.rate_limited_count} other={sample.other_count} "
            f"mock_received={mock_text}"
        )


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
        help="New API token, default: NEW_API_API_KEY or NEW_API_KEY. Use --api-keys for multi-group tests.",
    )
    parser.add_argument(
        "--api-keys",
        default=os.getenv("NEW_API_API_KEYS", ""),
        help="Comma-separated New API tokens matched one-to-one with --groups.",
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
    parser.add_argument(
        "--duration-seconds",
        type=float,
        default=0.0,
        help="Run for this many seconds; when set, count is calculated from rpm.",
    )
    parser.add_argument("--rpm", type=float, default=60.0, help="Start rate.")
    parser.add_argument("--max-workers", type=int, default=64)
    parser.add_argument("--timeout", type=float, default=120.0)
    parser.add_argument("--max-tokens", type=int, default=8)
    parser.add_argument("--body-bytes", type=int, default=2048)
    parser.add_argument(
        "--payload-tokens",
        type=int,
        default=0,
        help="Generate a large prompt with approximately this many tokens.",
    )
    parser.add_argument(
        "--bytes-per-token",
        type=float,
        default=4.0,
        help="Estimated bytes per token used with --payload-tokens.",
    )
    parser.add_argument(
        "--payload-bytes",
        default="",
        help="Generate a prompt with this byte size, e.g. 800k or 2m. Overrides --payload-tokens.",
    )
    parser.add_argument(
        "--resource-sample-interval",
        type=float,
        default=5.0,
        help="Seconds between resource samples.",
    )
    parser.add_argument(
        "--monitor-pid",
        type=int,
        default=int(os.getenv("NEW_API_MONITOR_PID", "0") or "0"),
        help="PID of the new-api process to sample.",
    )
    parser.add_argument(
        "--monitor-pid-file",
        default=os.getenv("NEW_API_MONITOR_PID_FILE", ""),
        help="PID file for the new-api process.",
    )
    parser.add_argument(
        "--monitor-process-name",
        default=os.getenv("NEW_API_MONITOR_PROCESS_NAME", "new-api"),
        help="Process name pattern used by pgrep when --monitor-pid is not set.",
    )
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
        "--channel-warmup-seconds",
        type=float,
        default=0.0,
        help="Wait after creating mock channels so distributor caches can refresh.",
    )
    parser.add_argument(
        "--force-mock-channel",
        action="store_true",
        help=(
            "Append the created mock channel ID to each API key to force routing. "
            "The token user must be an admin."
        ),
    )
    parser.add_argument(
        "--groups",
        default=os.getenv("NEW_API_TEST_GROUPS", ""),
        help="Comma-separated group labels matched one-to-one with --api-keys.",
    )
    parser.add_argument(
        "--mock-channel-name",
        default=f"bedrock-rate-limit-test-{int(time.time())}",
    )
    parser.add_argument("--mock-channel-priority", type=int, default=999999)
    parser.add_argument("--mock-channel-weight", type=int, default=100)
    parser.add_argument(
        "--mock-channels-per-group",
        type=int,
        default=1,
        help="Number of temporary mock AWS channels to create for each group.",
    )
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
    try:
        targets = build_request_targets(args)
    except ValueError as exc:
        print(str(exc), file=sys.stderr)
        return 2
    if args.duration_seconds > 0:
        args.count = max(1, int(args.rpm * args.duration_seconds / 60.0))
    if args.count < 1:
        print("--count must be positive", file=sys.stderr)
        return 2
    if args.rpm <= 0:
        print("--rpm must be positive", file=sys.stderr)
        return 2
    if args.mock_channels_per_group < 1:
        print("--mock-channels-per-group must be positive", file=sys.stderr)
        return 2
    if args.resource_sample_interval <= 0:
        print("--resource-sample-interval must be positive", file=sys.stderr)
        return 2

    args.generated_message = build_large_message(args)
    payload_bytes = len(args.generated_message.encode("utf-8"))

    mock_server = None
    mock_state = None
    created_mock_channels: list[tuple[str, str]] = []
    mock_channel_ids_by_group: dict[str, list[int]] = {}
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
            groups = unique_groups(targets)
            for group in groups:
                base_channel_name = mock_channel_name_for_group(args, group, len(groups))
                for channel_index in range(args.mock_channels_per_group):
                    channel_name = mock_channel_name_for_index(
                        base_channel_name,
                        channel_index,
                        args.mock_channels_per_group,
                    )
                    channel_id = create_mock_channel(args, mock_base_url, group, channel_name)
                    if channel_id is not None:
                        mock_channel_ids_by_group.setdefault(group, []).append(channel_id)
                    created_mock_channels.append((group, channel_name))
            fix_channel_abilities(args)
            if args.force_mock_channel:
                missing = [group for group in groups if not mock_channel_ids_by_group.get(group)]
                if missing:
                    raise RuntimeError(
                        "--force-mock-channel could not find created channel ids for groups: "
                        + ",".join(missing)
                    )
                for target in targets:
                    target.api_key = append_specific_channel_id(
                        target.api_key, mock_channel_ids_by_group[target.group][0]
                    )
                print("forced requests to the first created mock channel id for each group")
            if args.channel_warmup_seconds > 0:
                print(f"waiting {args.channel_warmup_seconds:g}s for channel cache warmup")
                time.sleep(args.channel_warmup_seconds)

    try:
        interval = 60.0 / args.rpm
        max_workers = max(1, min(args.max_workers, args.count))
        start = time.monotonic()

        print(
            f"base_url={args.base_url} format={args.format} model={args.model} "
            f"count={args.count} rpm={args.rpm:g} interval={interval:.3f}s "
            f"payload_bytes={payload_bytes} targets={','.join(target.label for target in targets)}"
        )
        print("idx target status start_s latency_s body")

        results: list[Result] = []
        results_lock = threading.Lock()
        stop_monitor = threading.Event()
        resource_samples: list[ResourceSample] = []
        monitor_thread = threading.Thread(
            target=monitor_resources,
            args=(args, start, results, results_lock, stop_monitor, mock_state, resource_samples),
            daemon=True,
        )
        monitor_thread.start()
        with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
            futures = []
            for index in range(1, args.count + 1):
                scheduled_at = start + (index - 1) * interval
                target = targets[(index - 1) % len(targets)]
                futures.append(executor.submit(post_once, args, target, index, scheduled_at))

            for future in concurrent.futures.as_completed(futures):
                result = future.result()
                with results_lock:
                    results.append(result)
                start_s = result.started_at - start
                latency_s = result.ended_at - result.started_at
                status = result.status if result.status is not None else "ERR"
                detail = short_body(result.body or result.error)
                print(
                    f"{result.index:03d} {result.target_label:>12} {status} "
                    f"{start_s:8.2f} {latency_s:9.2f} {detail}"
                )
        stop_monitor.set()
        monitor_thread.join(timeout=2)

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
        elapsed_total = max(0.001, max(item.ended_at for item in results) - start)
        ok_rpm = ok_count / elapsed_total * 60
        sent_rpm = len(results) / elapsed_total * 60
        backlog_growth_rpm = max(0.0, args.rpm - ok_rpm)
        peak_backlog = max((sample.backlog_estimate for sample in resource_samples), default=0)
        peak_rss = max(
            (sample.rss_bytes for sample in resource_samples if sample.rss_bytes is not None),
            default=None,
        )
        peak_cpu = max(
            (sample.cpu_percent for sample in resource_samples if sample.cpu_percent is not None),
            default=None,
        )
        drain_seconds = None
        if ok_rpm > 0:
            drain_seconds = peak_backlog / ok_rpm * 60
        cpu_text = "?" if peak_cpu is None else f"{peak_cpu:.1f}%"
        drain_text = "?" if drain_seconds is None else f"{drain_seconds:.1f}s"
        print(
            "load_analysis "
            f"elapsed={elapsed_total:.1f}s sent_rpm={sent_rpm:.1f} accepted_rpm={ok_rpm:.1f} "
            f"backlog_growth_rpm={backlog_growth_rpm:.1f} peak_backlog={peak_backlog} "
            f"peak_rss={format_bytes(peak_rss)} peak_cpu={cpu_text} "
            f"estimated_drain_time_at_observed_rate={drain_text}"
        )
        if rate_limited_count > 0:
            print(
                "load_conclusion: queue limit or timeout was reached; reduce incoming rpm, "
                "raise effective Bedrock capacity, or lower queue timeout/max size."
            )
        elif backlog_growth_rpm > 0:
            print(
                "load_conclusion: incoming rate exceeded observed completion rate during the run; "
                "a long enough burst can keep growing queued requests until timeout or queue-full."
            )
        else:
            print(
                "load_conclusion: observed completion rate kept up with incoming rate in this run."
            )
        for target in targets:
            target_results = [item for item in results if item.target_label == target.label]
            target_ok = sum(
                1 for item in target_results if item.status is not None and 200 <= item.status < 300
            )
            target_rate_limited = sum(1 for item in target_results if item.status == 429)
            target_other = len(target_results) - target_ok - target_rate_limited
            print(
                f"target_summary target={target.label} group={target.group} "
                f"total={len(target_results)} ok={target_ok} "
                f"rate_limited_429={target_rate_limited} other={target_other}"
            )
        print(
            "expected: per effective group limit = base limit * enabled AWS channel count. "
            "If two effective groups each have two enabled AWS channels and base limit=10/min, "
            "the shared mock upstream should receive up to about 40 requests per 60s window."
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
                if any(
                    "InvokeModel" in item.body or "Bedrock Runtime" in item.body
                    for item in results
                ):
                    print(
                        "diagnostic: responses came from the AWS SDK path, so the mock "
                        "channel was not selected or the selected channel is not using "
                        "aws_key_type=api_key. Try --force-mock-channel with an admin token."
                    )
    finally:
        if created_mock_channels and not args.keep_mock_channel:
            try:
                delete_mock_channels(args, created_mock_channels)
            except Exception as exc:  # noqa: BLE001
                print(f"cleanup mock channel failed: {exc}", file=sys.stderr)
                exit_code = 1
        if mock_server is not None:
            mock_server.shutdown()
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())

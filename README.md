# Clipboard Controller

HTTP API server quản lý quyền truy cập clipboard cho các automation tool chạy đa luồng.

## Tải về

**Windows:** [clipboard-controller.exe](https://github.com/TechScapBot/clipboard/raw/main/clipboard-controller.exe)

**BAS Test Script:** [TestClipboard.xml](https://github.com/TechScapBot/clipboard/raw/main/TestClipboard.xml) - Import vào BAS để test

Hoặc clone repo:
```bash
git clone https://github.com/TechScapBot/clipboard.git
cd clipboard
```

## Bắt đầu nhanh

1. Tải file `clipboard-controller.exe`
2. Tạo file `config.yaml` cùng thư mục (hoặc dùng config mặc định)
3. Chạy `clipboard-controller.exe`
4. Server sẽ chạy tại `http://localhost:8899`
5. Icon sẽ xuất hiện ở system tray (khay hệ thống)

**Lưu ý:**
- Nút X trên console bị vô hiệu hóa - sử dụng menu tray để thoát
- Click phải icon tray để: Hide/Show Console, Open Logs, Exit

## Khởi động

```bash
# Chạy với config mặc định
./clipboard-controller.exe

# Chạy với config file khác
./clipboard-controller.exe --config /path/to/config.yaml

# Override port
./clipboard-controller.exe --port 9000

# Override log level
./clipboard-controller.exe --log-level debug

# Override log directory
./clipboard-controller.exe --log-dir /var/log/clipboard

# Kết hợp nhiều options
./clipboard-controller.exe --port 9000 --log-level debug
```

### Command Line Flags

| Flag | Default | Mô tả |
|------|---------|-------|
| `--config` | `config.yaml` | Path đến config file |
| `--port` | (từ config) | Override port |
| `--log-level` | (từ config) | Override log level: debug, info, warn, error |
| `--log-dir` | (từ config) | Override log directory |

Server listen tại `http://localhost:8899` (mặc định)

---

## API Reference

### Health Check

#### GET /health

Kiểm tra server còn hoạt động.

```bash
curl http://localhost:8899/health
```

**Response:**
```json
{
    "status": "healthy",
    "uptime_seconds": 3600,
    "version": "1.0.0"
}
```

---

### Tool Management

#### POST /tool/register

Đăng ký tool mới. Gọi 1 lần khi tool khởi động.

```bash
curl -X POST http://localhost:8899/tool/register \
  -H "Content-Type: application/json" \
  -d '{"tool_id": "my_tool_123"}'
```

**Response (200):**
```json
{
    "tool_id": "my_tool_123",
    "status": "registered",
    "config": {
        "heartbeat_interval": 120,
        "poll_interval": 200,
        "ticket_ttl": 120,
        "lock_max_duration": 20
    }
}
```

**Response (409):** Tool ID đã tồn tại và đang online.

---

#### POST /tool/heartbeat

Báo tool còn hoạt động. Gọi định kỳ mỗi 2 phút.

```bash
curl -X POST http://localhost:8899/tool/heartbeat \
  -H "Content-Type: application/json" \
  -d '{"tool_id": "my_tool_123"}'
```

**Response (200):**
```json
{
    "status": "ok",
    "next_heartbeat_before": "2024-01-15T10:10:00Z"
}
```

**Response (404):** Tool chưa được đăng ký.

---

#### POST /tool/unregister

Hủy đăng ký tool. Gọi khi tool tắt.

```bash
curl -X POST http://localhost:8899/tool/unregister \
  -H "Content-Type: application/json" \
  -d '{"tool_id": "my_tool_123"}'
```

**Response (200):**
```json
{
    "status": "unregistered"
}
```

---

#### GET /tool/status

Xem trạng thái tool (debug).

```bash
curl "http://localhost:8899/tool/status?tool_id=my_tool_123"
```

**Response (200):**
```json
{
    "tool_id": "my_tool_123",
    "status": "online",
    "registered_at": "2024-01-15T09:00:00Z",
    "last_heartbeat": "2024-01-15T10:05:30Z",
    "next_heartbeat_deadline": "2024-01-15T10:10:30Z"
}
```

---

### Lock Management

#### POST /lock/request

Yêu cầu quyền sử dụng clipboard. Trả về ticket để poll.

```bash
curl -X POST http://localhost:8899/lock/request \
  -H "Content-Type: application/json" \
  -d '{"tool_id": "my_tool_123", "thread_id": "thread_1"}'
```

**Response (200) - Được cấp ngay:**
```json
{
    "ticket_id": "abc-123-def",
    "position": 0,
    "status": "granted",
    "expires_at": "2024-01-15T10:05:40Z",
    "lock_duration_ms": 20000,
    "poll_interval": 200
}
```

**Response (200) - Đang chờ trong queue:**
```json
{
    "ticket_id": "abc-123-def",
    "position": 3,
    "poll_interval": 200,
    "ticket_expires_at": "2024-01-15T10:07:00Z"
}
```

**Response (400):** Tool không online.

---

#### GET /lock/check

Kiểm tra trạng thái ticket. Poll endpoint này cho đến khi được cấp lock.

```bash
curl "http://localhost:8899/lock/check?ticket_id=abc-123-def"
```

**Response - Đang chờ:**
```json
{
    "status": "waiting",
    "position": 2,
    "estimated_wait_ms": 3000
}
```

**Response - Được cấp:**
```json
{
    "status": "granted",
    "expires_at": "2024-01-15T10:05:40Z",
    "lock_duration_ms": 15000
}
```

**Response - Hết hạn:**
```json
{
    "status": "expired",
    "reason": "ticket_ttl_exceeded"
}
```

---

#### POST /lock/release

Trả lock sau khi paste xong. **Quan trọng:** Luôn gọi release sau khi paste!

```bash
curl -X POST http://localhost:8899/lock/release \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "abc-123-def"}'
```

**Response (200):**
```json
{
    "status": "released",
    "held_duration_ms": 1500
}
```

**Response (400):** Ticket không đang giữ lock.

---

#### POST /lock/extend

Gia hạn thời gian giữ lock (nếu cần paste lâu hơn).

```bash
curl -X POST http://localhost:8899/lock/extend \
  -H "Content-Type: application/json" \
  -d '{"ticket_id": "abc-123-def"}'
```

**Response (200):**
```json
{
    "status": "extended",
    "new_expires_at": "2024-01-15T10:06:00Z",
    "extend_count": 1,
    "extend_remaining": 1
}
```

**Response (400):** Đã extend tối đa (mặc định 2 lần).

---

#### GET /lock/status

Xem trạng thái queue (debug/monitoring).

```bash
curl http://localhost:8899/lock/status
```

**Response (200):**
```json
{
    "current_lock": {
        "ticket_id": "xyz-789",
        "tool_id": "tool_A",
        "thread_id": "thread_1",
        "granted_at": "2024-01-15T10:04:58Z",
        "expires_in_ms": 15000
    },
    "queue_length": 2,
    "queue": [
        {"position": 1, "tool_id": "tool_B", "thread_id": "thread_2", "waiting_ms": 1200},
        {"position": 2, "tool_id": "tool_A", "thread_id": "thread_3", "waiting_ms": 800}
    ]
}
```

---

### Config

#### GET /config

Xem config hiện tại.

```bash
curl http://localhost:8899/config
```

#### PATCH /config

Cập nhật config runtime.

```bash
curl -X PATCH http://localhost:8899/config \
  -H "Content-Type: application/json" \
  -d '{"poll_interval": 300, "lock_max_duration": 30}'
```

---

### Debug & Monitoring

#### GET /debug/logs/recent

Xem các lock events gần đây (từ memory buffer, không đọc file).

```bash
curl "http://localhost:8899/debug/logs/recent?limit=20"
```

**Response (200):**
```json
{
    "count": 20,
    "events": [
        {
            "timestamp": "2024-01-15T10:05:40Z",
            "event_type": "lock_granted",
            "request_id": "req-abc-123",
            "ticket_id": "ticket-xyz",
            "tool_id": "my_tool",
            "thread_id": "thread_1",
            "queue_length": 3,
            "wait_duration_ms": 1500
        },
        {
            "timestamp": "2024-01-15T10:05:38Z",
            "event_type": "lock_requested",
            "ticket_id": "ticket-xyz",
            "tool_id": "my_tool",
            "thread_id": "thread_1",
            "queue_position": 1,
            "queue_length": 4
        }
    ]
}
```

---

#### GET /debug/logs/stats

Xem thống kê log files.

```bash
curl http://localhost:8899/debug/logs/stats
```

**Response (200):**
```json
{
    "log_dir": "./logs",
    "total_size_mb": 15.5,
    "files": {
        "requests": 7,
        "lock": 7,
        "tool": 7,
        "metrics": 7,
        "summary": 7
    },
    "oldest_log": "2024-01-08",
    "newest_log": "2024-01-15"
}
```

---

## Luồng sử dụng cơ bản

### 1. Khởi động tool

```
POST /tool/register {"tool_id": "my_tool"}
-> Lưu config trả về
-> Bắt đầu heartbeat loop (mỗi 2 phút)
```

### 2. Khi cần paste (mỗi thread)

```
POST /lock/request {"tool_id": "my_tool", "thread_id": "thread_1"}
-> Nhận ticket_id

LOOP:
    GET /lock/check?ticket_id=xxx
    -> Nếu status = "granted": thoát loop
    -> Nếu status = "waiting": sleep(poll_interval), tiếp tục
    -> Nếu status = "expired": thất bại

SET_CLIPBOARD(content)
SEND_KEYS("Ctrl+V")

POST /lock/release {"ticket_id": "xxx"}
```

### 3. Khi tắt tool

```
POST /tool/unregister {"tool_id": "my_tool"}
```

---

## Config mặc định

| Parameter | Default | Mô tả |
|-----------|---------|-------|
| `port` | 8899 | Port HTTP server |
| `heartbeat_timeout` | 300s | Tool offline nếu không heartbeat |
| `heartbeat_interval` | 120s | Gợi ý interval cho client |
| `poll_interval` | 200ms | Gợi ý poll interval |
| `ticket_ttl` | 120s | Ticket expire nếu không poll |
| `lock_max_duration` | 20s | Thời gian giữ lock tối đa |
| `lock_extend_max` | 2 | Số lần extend tối đa |
| `lock_grace_period` | 5s | Grace period sau khi grant |

---

## Error Codes

| Error | HTTP Status | Mô tả |
|-------|-------------|-------|
| `tool_already_registered` | 409 | Tool ID đã online |
| `tool_not_found` | 404 | Tool chưa đăng ký |
| `tool_offline` | 400 | Tool không online |
| `ticket_not_found` | 404 | Ticket không tồn tại |
| `not_lock_holder` | 400 | Không đang giữ lock |
| `max_extend_reached` | 400 | Đã extend tối đa |
| `extend_disabled` | 400 | Extend không được bật |

---

## Logging

### Log Files Structure

```
logs/
├── requests/
│   └── 2024-01-15.jsonl      # HTTP request logs
├── events/
│   ├── lock/
│   │   └── 2024-01-15.jsonl  # Lock events (request, grant, release, expire)
│   └── tool/
│       └── 2024-01-15.jsonl  # Tool events (register, heartbeat, offline)
├── metrics/
│   └── 2024-01-15.jsonl      # System metrics (mỗi phút)
└── summary/
    └── 2024-01-15.json       # Daily summary
```

### Logging Config

| Parameter | Default | Mô tả |
|-----------|---------|-------|
| `log_dir` | `./logs` | Thư mục chứa log files |
| `log_retention_days` | 30 | Số ngày giữ logs (tự động xóa cũ hơn) |
| `log_level` | `info` | Log level: debug, info, warn, error |
| `log_requests` | `true` | Log HTTP requests vào file |
| `log_events` | `true` | Log lock/tool events |
| `log_metrics` | `true` | Thu thập metrics mỗi phút |
| `log_summary` | `true` | Tạo daily summary |
| `log_heartbeats` | `false` | Log heartbeat events (noisy, mặc định tắt) |

### Log Event Types

**Lock Events:**
- `lock_requested` - Thread yêu cầu lock
- `lock_granted` - Lock được cấp
- `lock_released` - Lock được release
- `lock_expired` - Lock hết hạn (với reason)
- `lock_extended` - Lock được extend

**Tool Events:**
- `tool_registered` - Tool đăng ký
- `tool_heartbeat` - Heartbeat (chỉ khi `log_heartbeats: true`)
- `tool_offline` - Tool offline
- `tool_unregistered` - Tool hủy đăng ký

### Sample Log Entry

```json
{
  "timestamp": "2024-01-15T10:05:40Z",
  "event_type": "lock_granted",
  "request_id": "abc-123",
  "ticket_id": "ticket-xyz",
  "tool_id": "my_tool",
  "thread_id": "thread_1",
  "queue_length": 3,
  "wait_duration_ms": 1500
}
```

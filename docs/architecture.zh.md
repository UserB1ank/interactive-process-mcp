# 模块架构详解

本文档详细介绍 `interactive-process-mcp` 每个模块的作用、内部结构和协作关系。

---

## 整体数据流

```
Agent (MCP Client)
       │
       │  SSE/HTTP
       ▼
  ┌─ mcp (server.go + handlers.go) ─────────────────────────┐
  │   接收 MCP 工具调用，解析参数，调用 session/message 层   │
  └──────────────┬───────────────────────────────────────────┘
                 │
       ┌─────────┴──────────┐
       ▼                    ▼
  ┌─ session ──┐      ┌─ message ──┐
  │ 进程生命周期│      │ 消息持久化  │
  │ 管理       │      │ 管理        │
  └─────┬──────┘      └─────┬──────┘
        │                   │
        ▼                   ▼
  ┌─ buffer ──┐      ┌─ storage ──┐
  │ 实时输出   │      │ JSON 文件   │
  │ 环形缓冲区 │      │ 读写        │
  └─────┬──────┘      └────────────┘
        │
        ▼
  ┌─ sshclient ──┐    ┌─ ansi ──┐
  │ SSH 客户端    │    │ ANSI    │
  │ 连接内部 SSH  │    │ 转义码   │
  └──────┬────────┘    │ 清除     │
         │             └─────────┘
         ▼
  ┌─ sshserver ──┐
  │ 内部 SSH 服务 │
  │ 执行实际命令  │
  └───────────────┘
```

---

## 模块详解

### 1. `cmd/server` — 程序入口

**文件**: `cmd/server/main.go`

**职责**: 串联所有模块，完成启动和优雅关闭。

**启动流程**:

1. 解析命令行参数（host、port、data-dir、ssh-host、ssh-port）
2. 启动内部 SSH server（`sshserver.New` + `Start`）
3. 初始化 JSON 存储（`storage.New`）
4. 初始化消息管理器（`message.NewManager`）
5. 初始化会话管理器（`session.NewManager`）
6. 启动 MCP SSE server（`mcp.New` + `Start`）
7. 监听 SIGINT/SIGTERM 信号，触发优雅关闭：
   - 终止所有运行中的 session
   - 停止 SSH server
   - 停止 MCP server

**依赖关系**: `main.go` 是唯一一个引用所有 `internal/` 子包的地方。

---

### 2. `internal/config` — 配置定义

**文件**: `internal/config/config.go`

**职责**: 定义运行时配置结构体。

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Host` | string | `"0.0.0.0"` | MCP SSE server 监听地址 |
| `Port` | int | `8080` | MCP SSE server 端口 |
| `DataDir` | string | `"./data"` | JSON 持久化目录 |
| `SSHHost` | string | `"127.0.0.1"` | 内部 SSH server 地址 |
| `SSHPort` | int | `0` | SSH 端口，0 表示随机分配 |

`Default()` 返回一个填充了默认值的 `Config` 实例，`main.go` 再用命令行参数覆盖。

---

### 3. `internal/sshserver` — 内部 SSH 服务

**文件**: `internal/sshserver/server.go`

**职责**: 在 localhost 上运行一个 SSH server，用于隔离和执行实际命令。

**为什么要通过 SSH 执行命令？**

直接用 `os/exec` 启动进程虽然简单，但缺乏成熟的 PTY 分配、窗口调整、信号传递等机制。通过 SSH 协议，可以免费获得：
- PTY 请求和窗口大小变更通知
- 标准化的信号传递（SIGTERM、SIGKILL 等）
- stdin/stdout/stderr 分离的管道

**关键结构**:

```
Server
  ├── addr     — 监听地址（如 "127.0.0.1:40001"）
  ├── server   — gliderlabs/ssh.Server 实例
  ├── listener — TCP listener
  └── started  — 是否已启动
```

**核心逻辑**:

- `Start()`: 动态生成 RSA host key，开始监听 TCP，在后台 goroutine 中 serve
- `handleSession()`: 处理每个 SSH 连接
  - **PTY 模式**: 用 `creack/pty` 创建伪终端，双向 io.Copy 传递数据，支持窗口大小动态调整
  - **Pipe 模式**: 直接将 SSH channel 的 stdin/stdout/stderr 绑定到 `exec.Cmd`
- `ClientConfig()`: 返回预配置的 SSH 客户端配置（硬编码密码 + 跳过 host key 校验）

**对外接口**:

| 方法 | 说明 |
|------|------|
| `New(addr)` | 创建 SSH server 实例 |
| `Start()` | 启动监听 |
| `Addr()` | 获取实际监听地址（端口随机时有用） |
| `Stop()` | 关闭 server |
| `ClientConfig()` | 获取客户端连接配置 |
| `InternalPassword()` | 获取内部认证密码 |

---

### 4. `internal/sshclient` — SSH 客户端封装

**文件**: `internal/sshclient/client.go`

**职责**: 封装连接内部 SSH server 的客户端逻辑，代表一个正在运行的远程进程。

**关键结构**:

```
ExecSession
  ├── client  — *ssh.Client    SSH 连接
  ├── session — *ssh.Session    SSH 会话
  ├── Stdin   — io.WriteCloser  写入进程标准输入
  ├── Stdout  — io.Reader       读取进程标准输出
  ├── Stderr  — io.Reader       读取进程标准错误
  ├── done    — chan struct{}   进程退出时关闭
  ├── exitCode — int            退出码
  └── err      — error          退出时的错误
```

**核心逻辑**:

- `Start(addr, command, args, pty, rows, cols)`:
  1. SSH 连接到内部 server
  2. 打开 session，获取 stdin/stdout/stderr 管道
  3. 如果是 PTY 模式，请求 PTY（xterm-256color）
  4. 启动命令，后台 goroutine 等待退出并记录退出码
- `shellQuote()`: 对参数中的特殊字符进行 shell 转义

**对外接口**:

| 方法 | 说明 |
|------|------|
| `Start(addr, cmd, args, pty, rows, cols)` | 启动远程进程 |
| `Done()` | 返回 channel，进程退出时关闭 |
| `ExitCode()` | 获取退出码（需在 Done 后调用） |
| `ResizePty(rows, cols)` | 调整 PTY 窗口大小 |
| `Signal(sig)` | 发送信号 |
| `Close()` | 强制关闭连接 |

---

### 5. `internal/session` — 会话管理

这是整个项目最核心的模块，分为两个文件。

#### 5.1 `session.go` — 单个会话的生命周期

**职责**: 管理一个交互式进程从创建到终止的完整生命周期。

**关键结构**:

```
Session
  ├── api.Session           — 嵌入的元数据（ID、名称、命令、状态、退出码等）
  ├── mu          sync.RWMutex — 保护元数据字段
  ├── execSession *sshclient.ExecSession — SSH 进程连接
  ├── buf         *buffer.Buffer — 实时输出缓冲区
  ├── msgMgr      *message.Manager — 消息持久化
  └── sshAddr     string — SSH server 地址
```

**创建流程** (`New()`):

1. 生成 UUID（取前 12 位作为 ID）
2. 调用 `sshclient.Start()` 连接内部 SSH server 启动命令
3. 创建 1MB 环形缓冲区
4. 启动 3 个后台 goroutine：
   - **stdout reader**: 持续读取标准输出 → 写入 buffer
   - **stderr reader**: 持续读取标准错误 → 写入 buffer
   - **exit watcher**: 等待进程退出 → 更新状态和退出码 → 关闭 buffer

**核心方法**:

| 方法 | 说明 |
|------|------|
| `New(sshAddr, cmd, args, mode, name, rows, cols, msgMgr)` | 创建并启动会话 |
| `SendInput(text, pressEnter)` | 向进程 stdin 写入文本 |
| `ReadOutput(timeout, stripAnsi, maxLines)` | 从缓冲区读取新输出 |
| `Terminate(force, gracePeriod)` | 终止进程（优雅/强制） |
| `ResizePty(rows, cols)` | 调整 PTY 窗口尺寸 |
| `Info()` | 返回会话元数据的快照 |

**终止策略** (`Terminate`):
1. 非强制模式：先发 SIGTERM → 等待 gracePeriod → 超时则 Close（相当于 SIGKILL）
2. 强制模式：直接 Close

#### 5.2 `manager.go` — 会话注册表

**职责**: 线程安全地管理所有 session 实例，负责注册、查询、终止和持久化。

**关键结构**:

```
Manager
  ├── sessions  map[string]*Session — ID → Session 映射
  ├── mu        sync.RWMutex — 保护 map
  ├── sshAddr   string — SSH server 地址
  ├── msgMgr    *message.Manager — 消息管理器
  └── store     *storage.Store — 持久化存储
```

**核心方法**:

| 方法 | 说明 |
|------|------|
| `Create(cmd, args, mode, name, rows, cols)` | 创建 session，加入 map，持久化 |
| `Get(id)` | 按 ID 查找 session |
| `ListAll()` | 返回所有 session 的元数据 |
| `Terminate(id, force, gracePeriod)` | 终止指定 session |
| `CleanupAll(force)` | 终止所有 session（用于关闭时） |

`persist()` 在每次 Create/Terminate 后调用，将全量 session 列表写入 `sessions.json`。

---

### 6. `internal/buffer` — 环形缓冲区

**文件**: `internal/buffer/buffer.go`

**职责**: 为每个 session 提供线程安全的、带阻塞等待功能的实时输出缓冲区。

**关键结构**:

```
Buffer
  ├── maxBytes  int          — 容量上限（默认 1MB）
  ├── chunks    []string     — 数据块列表
  ├── total     int          — 当前总字节数
  ├── mu        sync.Mutex   — 保护所有字段
  ├── newData   *sync.Cond   — 条件变量，用于阻塞读等待
  ├── closed    bool         — 是否已关闭
  ├── readPos   int          — 消费者读取位置
  └── writePos  int          — 生产者写入位置
```

**设计要点**:

- **生产者**（stdout/stderr reader）调用 `Write(data)` 追加数据
- **消费者**（MCP read_output）调用 `ReadNew(timeout)` 读取新数据
- `readPos` / `writePos` 实现游标机制：消费者只读未读过的数据
- 当 `total > maxBytes` 时，丢弃最旧的数据块，自动调整 readPos
- `sync.Cond` 实现阻塞读：无新数据时消费者等待，直到有新数据或超时
- `Close()` 关闭 buffer 并唤醒所有等待的消费者

**数据流示意**:

```
Write("hello")  →  chunks: ["hello"]  writePos: 1
Write("world")  →  chunks: ["hello", "world"]  writePos: 2
ReadNew(5s)     →  返回 "helloworld"  chunks: []  readPos: 2
ReadNew(5s)     →  无新数据，阻塞等待 5s → 返回 ""
```

---

### 7. `internal/storage` — JSON 文件持久化

**文件**: `internal/storage/store.go`

**职责**: 将 session 元数据和消息记录以 JSON 文件形式持久化到磁盘。

**文件结构**:

```
data/
  ├── sessions.json                                    — 全量会话列表
  └── messages/
      └── {session_id}/
          ├── index.json                               — 消息索引（轻量摘要）
          └── messages/
              ├── a1b2c3d4e5f6.json                    — 单条消息内容
              └── ...
```

**关键结构**:

```
Store
  ├── dataDir string       — 存储根目录
  └── mu      sync.RWMutex — 保护文件 I/O 操作
```

**核心方法**:

| 方法 | 说明 |
|------|------|
| `SaveSessions(sessions)` | 写入 sessions.json |
| `LoadSessions()` | 读取 sessions.json |
| `SaveMessageIndex(sessionID, entries)` | 写入消息索引 |
| `LoadMessageIndex(sessionID)` | 读取消息索引 |
| `SaveMessage(sessionID, msg)` | 写入单条消息 |
| `LoadMessage(sessionID, msgID)` | 读取单条消息 |
| `LoadMessages(sessionID, msgIDs)` | 批量读取消息 |

每次文件操作都独立获取锁，保证单个文件操作的原子性。

---

### 8. `internal/message` — 消息管理

**文件**: `internal/message/message.go`

**职责**: 管理 session 的 I/O 消息记录（输入、输出、系统消息）。

**关键结构**:

```
Manager
  └── store *storage.Store — 底层存储
```

**消息类型**:

| 类型 | 常量 | 说明 |
|------|------|------|
| `input` | `MsgInput` | Agent 发送的输入文本 |
| `output` | `MsgOutput` | 进程的输出内容 |
| `system` | `MsgSystem` | 系统消息（启动、退出、终止等） |

**核心方法**:

| 方法 | 说明 |
|------|------|
| `Append(sessionID, typ, content)` | 追加一条消息，同时更新索引 |
| `List(sessionID)` | 获取消息索引列表 |
| `Get(sessionID, msgID)` | 获取单条消息 |
| `GetMany(sessionID, msgIDs)` | 批量获取消息 |

`Append` 的工作流：
1. 生成消息 ID（UUID 前 12 位）
2. 调用 `store.SaveMessage` 保存消息内容到独立文件
3. 调用 `store.LoadMessageIndex` 读取现有索引
4. 追加新条目，调用 `store.SaveMessageIndex` 写回索引

---

### 9. `internal/ansi` — ANSI 转义码清除

**文件**: `internal/ansi/strip.go`

**职责**: 从终端输出中移除 ANSI 转义序列，返回纯文本。

**支持的转义序列**:

| 类型 | 匹配模式 | 示例 |
|------|----------|------|
| CSI 序列 | `\x1b[...字母` | 颜色 `\x1b[32m`、光标移动 `\x1b[H` |
| OSC 序列 | `\x1b]...BEL/ST` | 窗口标题设置 |
| 字符集 | `\x1b(B` 等 | 字符集切换 |
| 两字节序列 | `\x1b 字符` | 各种控制 |

AI Agent 通常无法处理终端控制序列，`Strip()` 在 `ReadOutput` 中默认开启，确保返回可读的纯文本。

---

### 10. `internal/mcp` — MCP 服务层

#### 10.1 `server.go` — MCP Server 和工具注册

**职责**: 创建 MCP server 实例，注册所有工具，通过 SSE 暴露 HTTP 端点。

**关键结构**:

```
Server
  ├── mcpServer *mcpserver.MCPServer — MCP 协议层
  ├── sseServer *mcpserver.SSEServer — SSE 传输层
  ├── sessMgr   *session.Manager     — 会话管理器
  └── msgMgr    *message.Manager     — 消息管理器
```

**注册的 10 个 MCP 工具**:

| 工具名 | 用途 |
|--------|------|
| `start_process` | 启动交互式进程 |
| `send_input` | 发送输入 |
| `read_output` | 读取输出 |
| `send_and_read` | 发送输入并读取输出 |
| `list_sessions` | 列出所有会话 |
| `get_session_info` | 获取会话详情 |
| `terminate_process` | 终止进程 |
| `resize_pty` | 调整 PTY 大小 |
| `list_messages` | 列出消息索引 |
| `get_message` | 获取消息内容 |

#### 10.2 `handlers.go` — 工具处理器

**职责**: 实现每个 MCP 工具的具体处理逻辑。

**处理流程**:

```
Agent 调用工具 → MCP 框架反序列化参数 → handler 解析参数
    → 调用 session/message 层方法 → 构造 JSON 结果 → 返回
```

**关键设计**:

- `start_process` 启动后会 sleep 100ms，再尝试读取初始输出（如 banner、提示符）
- `send_and_read` 发送输入后也 sleep 100ms，等待进程处理输入
- 所有 handler 返回 JSON 格式的结果，成功/错误都有统一格式
- 错误不通过 Go error 返回，而是用 `NewToolResultError` 包装在正常响应中

---

### 11. `pkg/api` — 公共类型定义

**文件**: `pkg/api/types.go`

**职责**: 定义跨模块共享的数据类型，是整个项目的数据契约。

**Session 结构**:

| 字段 | 类型 | 说明 |
|------|------|------|
| `ID` | string | 会话唯一标识（UUID 前 12 位） |
| `Name` | string | 可读名称 |
| `Command` | string | 执行的命令 |
| `Args` | []string | 命令参数 |
| `Mode` | string | 运行模式：`"pty"` 或 `"pipe"` |
| `Status` | SessionStatus | 状态：`running` / `exited` / `error` |
| `ExitCode` | *int | 退出码（运行中为 nil） |
| `PID` | int | 进程 ID（当前未填充，始终为 0） |
| `CreatedAt` | time.Time | 创建时间 |
| `UpdatedAt` | time.Time | 最后更新时间 |
| `Rows` | int | PTY 行数 |
| `Cols` | int | PTY 列数 |

**Message 结构**:

| 字段 | 类型 | 说明 |
|------|------|------|
| `ID` | string | 消息唯一标识 |
| `SessionID` | string | 所属会话 ID |
| `Type` | MsgType | 消息类型：`input` / `output` / `system` |
| `Content` | string | 消息内容 |
| `CreatedAt` | time.Time | 创建时间 |
| `ByteSize` | int | 内容字节数 |

`MessageIndexEntry` 是 `Message` 的轻量摘要，用于索引文件，不包含完整内容。

---

## 模块依赖关系

```
cmd/server
    ├── config
    ├── mcp ───────────────┐
    │   ├── server.go      │
    │   └── handlers.go    │
    ├── session ───────────┤
    │   ├── session.go     │
    │   └── manager.go     │
    ├── message            │
    ├── storage            │
    └── sshserver          │
                           │
实际调用链:                │
  mcp.handlers             │
    → session.Manager ─────┘
      → session.Session
        → sshclient.ExecSession → sshserver
        → buffer.Buffer
        → ansi.Strip
        → message.Manager
          → storage.Store
```

**关键原则**: 依赖方向始终从上层到下层，没有循环依赖。`pkg/api` 作为公共类型包被所有 `internal/` 模块引用。
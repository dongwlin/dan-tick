# Go 启动测试偶发 panic 调查

调查日期：2026-10-12。基线提交：`09710e5`，Wails：`v3.0.0-beta.25`。实验环境：Fedora Linux 44、Go `1.26.8-X:nodwarf5`、linux/amd64。

## 结论与证据边界

**最初 panic 的根因仍未确认。** 本次没有复现原始异常，没有原始 panic 值或堆栈，不能认定 Wails、D-Bus 或 SQLite 是根因。

原始 `app.panicked` 事件由 `start()` 中的 `recover()` 记录，说明当时确有 Go panic 沿该 goroutine 的调用栈展开。它不能由普通 `os.Exit()` 产生，也不能直接捕获另一个 goroutine 的 panic。缺少 `database.open_failed` 只说明没有走到记录普通数据库错误的分支，不能证明 `storage.Open()` 尚未开始。

本次确认了两个独立问题：Wails 初始化 Fatal 缺少安全持久化失败事件，以及目录预检查在特定情况下阻止第二实例通知。二者都不能解释原始偶发 panic。

## 可控实验

实验只使用临时数据库、日志、调查专用单实例 ID；正常会话使用 `dbus-run-session`。没有运行 WebView、访问账号或 B 站接口，没有修改第三方源码。临时副本只增加调查标记和捕获堆栈的测试输出，正式日志不增加原始错误或堆栈。

### 修复前的启动代码

以 `09710e5^` 的 `main.go` / `main_test.go` 建立隔离副本，分别执行独立进程：

| 条件 | 次数 | 结果 |
| --- | --- | --- |
| 健康私有 D-Bus 会话 | 20 | 全部正确报告数据库目录错误 |
| 未设置 `DBUS_SESSION_BUS_ADDRESS` | 20 | 全部正确报告数据库目录错误 |
| 无效 D-Bus 地址 | 20 | Wails 在 `application.New()` 内主动退出 1 |
| 不存在的 D-Bus Unix socket | 20 | Wails 在 `application.New()` 内主动退出 1 |

80 次均未捕获同步 panic。未设置地址不等于没有会话总线：godbus 会尝试发现用户会话总线或自动启动，因此不能把这组通过解释成“无 D-Bus 也正常”。

### `09710e5` 的其他数据库失败

分别制造数据库路径为目录、损坏数据库文件、迁移表结构冲突三种真实失败；每种与健康、无效、失效 D-Bus 条件组合，各运行 3 个独立进程，共 27 次。

- 健康 D-Bus：三类失败都记录 `app.starting → database.open_failed → app.failed`。
- 无效或失效 D-Bus：Wails 主动退出 1，只留下 `app.starting`；SQLite 尚未打开，没有 `app.panicked`。

这是已验证的诊断缺口，而非数据库错误被错误归类为 panic。

### 第二实例边界

私有会话中的 holder 仅调用 `application.New()` 并持有单实例名称；次实例使用相同调查 ID，数据库父路径被普通文件阻挡。两组均有限时退出。

| 行为 | 修复前 | `09710e5` |
| --- | --- | --- |
| 次实例退出码 | 0 | 1 |
| holder 收到通知 | 是 | 否 |
| 次实例日志 | `app.starting` | `app.starting → database.open_failed → app.failed` |

这证明提前返回式预检查会跳过第二实例通知。它不创建第二个调度器；但在已有实例仍存活、数据路径随后不可用，或两次启动解析到不同数据路径时，不能保证重复启动恢复已有窗口。此实验没有验证实际窗口激活。

## Wails 初始化路径与异常类型

静态检查以本地固定版本源码为准，入口是 `pkg/application/application.go` 的 `New()`、`handleFatalError()` 和 `Run()`。上游入口可见 [Wails application.go](https://github.com/wailsapp/wails/blob/v3.0.0-beta.25/v3/pkg/application/application.go)。

- `New()` 建立全局 App、传输和资产服务，最后初始化单实例管理器。实际 GTK 平台应用创建发生在 `Run()`；全局快捷键的 X11 后端也为延迟初始化。因此不能把普通图形会话故障直接当作 `New()` 的同步 panic 原因。
- Linux 单实例代码依次连接 D-Bus、导出对象、请求名称。普通失败返回 error，之后由 Wails `fatal → handleFatalError → os.Exit(1)` 处理；第二实例通知后按配置退出。这些不是 Go panic。
- godbus 的非法认证状态存在显式同步 panic 分支，但默认认证器及本次环境实验没有触发它。它只是源码中可识别的候选，不能作为原始根因。
- Wails 部分回调 goroutine 使用自己的 panic handler；godbus 读取 goroutine 的异常、原生致命信号和 Go runtime fatal 不能由 `start()` 的 `recover()` 捕获。
- Windows 单实例使用 mutex / 消息窗口，平台初始化另有 panic 路径；本次仅静态阅读，没有 Windows 运行证据。

## 对 `09710e5` 的评估

预检查与 `storage.Open()` 使用相同的 `MkdirAll(..., 0700)`，没有提前打开 SQLite、恢复历史或启动调度器。正常启动仍在 Wails 获取单实例所有权后打开数据库，因此未发现新增重复执行或 SQLite 资源泄漏。

它是合理但有限的规避措施：只提前处理目录创建失败，不验证文件访问、SQLite 初始化或迁移；检查成功后目录仍可能变化。前述第二实例通知边界是实际语义变化，本次保留并明确记录。不能为了覆盖其他数据库错误而把 SQLite 移到单实例所有权之前。

Wails `New()` 已拥有 goroutine 和 Linux D-Bus 连接；公开 `Quit()` 在尚未 `Run()` 时无效，内部 cleanup 也未公开。调查时正式入口立即 `os.Exit(start())`；后续入口重构改为 `os.Exit(app.Run(assets))`，仍在入口返回后退出进程。进程退出会回收这些资源；测试进程中反复初始化则会保留全局 App 和其他状态。这是已有依赖限制，预检查没有新增它。真实 Wails 测试使用独立子进程，不以添加无效的 `defer Quit()` 假装解决资源清理。

## 本次修复

调查时在 `main.go` 为真实 Wails 边界安装安全错误与 panic handler；后续入口重构将这些处理迁至 [internal/app/desktop.go](../internal/app/desktop.go)，启动日志与退出码处理迁至 [startup.go](../internal/app/startup.go)：

| 情况 | 事件与处理 |
| --- | --- |
| 普通数据库打开 / 迁移失败 | `database.open_failed → app.failed` |
| Wails `New()` 尚未返回时的 Fatal | `desktop.init_failed → app.failed`，保留 Wails 退出 1 |
| Wails `New()` 返回后的 Fatal | `desktop.run_failed → app.failed`，保留 Wails 退出 1 |
| Wails 非致命错误 | `desktop.error`，不把应用标为启动失败 |
| 启动 goroutine 的意外 panic | `app.panicked / unexpected_panic` |
| Wails 自行捕获的回调 panic | `app.panicked / unexpected_panic`，保留原来的致命退出行为 |

阶段标志使用原子访问；日志同步写入，覆盖 Wails `os.Exit()` 不执行应用 defer 的路径。原始错误、panic 值和堆栈不进入日志或终端，新事件纳入诊断导出白名单。没有修改单实例配置、SQLite 打开顺序或第三方代码。

调查时在 `main_test.go` 新增普通错误与同步 panic 分类测试，以及独立子进程中的真实 D-Bus 初始化失败、Wails Fatal、Wails 异步事件监听器 panic 测试；现已迁至 [internal/app/startup_test.go](../internal/app/startup_test.go)，使用测试文件系统提供前端资源。测试验证准确事件序列、退出码、未提前打开 SQLite 及日志 / 终端脱敏。导出测试覆盖新增安全事件。

### 后续入口迁移验证（2026-10-13）

启动和桌面装配迁入 `internal/app` 后，原诊断与生命周期函数体保持原行为，仅改为接收根入口的 `fs.FS`。启动测试另覆盖成功返回时的 `app.starting → app.stopped` 日志与退出码 0；未为测试导出内部辅助函数。

- 使用本机 Go 1.26.8 验证 `go test ./...`、`go vet ./...` 和 `go test -race ./...`，均通过；这不改变 `go.mod` 的 Go 1.25.0 最低要求。`httptest` 需要本地监听端口，受限沙箱首次运行失败后，在允许本地测试端口的环境中重跑通过。
- 重新生成 Binding，仍为 6 个 Service、37 个方法，20 个生成文件逐字节不变；前端 122 个行为测试、Lint、格式检查及 Linux `wails3 build`（含类型检查和生产前端构建）通过，模块文件与锁文件无变化。
- Linux 真实 WebView 冒烟使用临时 XDG 数据目录、独立 D-Bus 会话和仅用于开发的 MCP 构建；该会话不配置服务自动激活目录，无法访问用户的 Secret Service。验证嵌入资源渲染总览、`ApplicationService.GetPreferences` 与 `WindowService.GetCloseBehavior` 的只读 Binding，以及 `WindowService.Quit` 正常退出；测试实例已停止，未读取实际账号或运行用户任务。
- Windows 实机、完整托盘交互、睡眠恢复和 Keyring 验收未在本次迁移中执行，仍按原开发计划验收；此处冒烟不代表 G1～G3 全链路通过。

## 尚存风险与验证范围

- 原始偶发 panic 尚无根因证据；上述修复不能宣称根治它。
- 提前返回式目录预检查仍有第二实例通知边界；扩大预检查不能解决 Wails 所有权与桌面初始化耦合。
- 未被 Wails 或当前启动 goroutine 捕获的异步 panic、native fatal 和 Go runtime fatal 仍不保证写入安全事件，不能用本次 handler 宣称覆盖全部进程异常。
- Windows、真实 Linux 图形会话异常、窗口激活和平台资源释放仍需实机验收。本次子进程测试只证明错误归类和隔离。
- 本次修复不改变日志存储不可用时的现有安全降级策略；磁盘日志无法写入时不能保证持久化。

## 验证结果

本次新增测试和最终代码已通过：

```sh
go test . ./internal/diagnostic -count=1 -v
go test ./... -count=1
go vet ./...
go test -race . ./internal/diagnostic -count=1
git diff --check
```

调查开始时，现有目录失败测试另外连续执行 10 次通过，显式移除 D-Bus 地址后也通过。初次全量测试受沙箱禁止本地 `httptest` 监听影响而失败；允许本地测试端口后，全量重跑通过。此限制不是应用启动 panic 的证据。

已检查工作区状态，未修改 `go.mod` / `go.sum` 或生成物，保留原有未跟踪目录。本次变更包括代码、测试和本报告。Windows 和真实图形会话专项验收未执行。本报告不保存底层错误、账号数据或内部堆栈。

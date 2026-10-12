# B 站 API 对接参考

本文记录 dan-tick 已对接的 B 站 Web 接口及首个正式版本的扫码认证与续期协议依据，供 Go 后端实现时查阅。业务范围与发送规则以 [PRD](PRD.md) 为准，后端职责以 [技术栈选型](技术栈选型.md) 为准；本文不重复定义调度、存储或界面方案。

发送、房间与登录状态接口依据 [bilibili-API-collect-next](https://github.com/renmu123/bilibili-API-collect-next) 的社区整理资料（查阅于 2026-10-10）；扫码与续期在 2026-10-12 重新核查，见第 5 节。**这些不是 B 站承诺稳定的官方接口；代码和替身测试不能证明真实账号流程有效或平台允许此用途。** 上游文档可能过时，首版发布仍须完成实机验证及平台使用要求核验。

## 1. 当前已对接的 MVP 接口清单

| 用途 | 方法与接口 | 项目需要的数据 | 对应需求 |
| --- | --- | --- | --- |
| 检查登录状态 | `GET https://api.bilibili.com/x/web-interface/nav` | `code`、`data.isLogin`，可选 `data.uname` | FR-002 |
| 查询直播间 | `GET https://api.live.bilibili.com/room/v1/Room/get_info` | 入参 `room_id`；返回 `data.room_id`、`data.uid`、`data.title`，可选 `data.live_status`、`data.user_cover` | FR-004、FR-005 |
| 查询主播名称 | `GET https://api.live.bilibili.com/live_user/v1/Master/info` | 入参为上一步的 `uid`；返回 `data.info.uname` | FR-004 |
| 发送直播弹幕 | `POST https://api.live.bilibili.com/msg/send` | 房间 ID、内容、Cookie、CSRF 等，详见下文 | FR-020～FR-023 |

- 用户输入直播间短号或直播间链接后，先解析房间号，再以 `get_info` 返回的 `data.room_id` 作为实际房间 ID。SQLite 中的 `rooms.platform_room_id` 存规范化的十进制字符串，避免将短号和真实房间号保存为两个直播间。
- `get_info` 返回主播 `uid`，不直接提供项目需要的主播名称；需要名称时再调用 `Master/info`。主播名称与直播状态属于展示信息，查询失败不应中断已配置任务的调度。
- 直播间展示元数据包含标题、主播名称和封面。封面优先使用 `get_info` 的 `data.user_cover`，不存在时尝试 `data.cover`；仅使用验证过的 B 站图片 CDN HTTPS 地址。缺少封面、查询失败或图片加载失败时使用原占位封面。
- 打开直播间页面时，前端串行刷新已加载的直播间：相邻两次请求之间至少间隔 5 秒，每 30 分钟重新检查；后端持久化最近检查时间，30 分钟内跳过远端请求，避免反复进入页面集中调用接口。成功获取的新标题、主播名称和封面回写 SQLite，用户备注始终保留；网络失败保留已保存的展示信息并延后重试。与任务调度相互独立。
- `nav` 的 `code = 0` 且 `data.isLogin = true` 可作为登录状态判断，但**不等于具备直播间发言权限**；实际发送是否被接受仍以发送接口结果为准。

## 2. 发送直播弹幕

采用 `application/x-www-form-urlencoded` 请求体。社区文档列出的当前核心参数如下：

| 参数 | 来源或 MVP 默认值 | 说明 |
| --- | --- | --- |
| `roomid` | 规范化后的真实直播间 ID | 不使用本地数据库主键 |
| `msg` | 用户配置的弹幕文本 | 提交前检查非空及平台允许的长度 |
| `rnd` | 当前 Unix 秒时间戳 | 不是毫秒时间戳 |
| `csrf` | Cookie 中 `bili_jct` 的值 | 与 Cookie 配套使用 |
| `fontsize` | `25` | MVP 固定普通字号 |
| `color` | `16777215` | 十进制白色 |

鉴权使用 `Cookie` 中的 `SESSDATA` 与 `bili_jct`。`csrf_token`、`mode`、`bubble`、`w_rid` / `wts` 等在参考文档中不是该接口的必需项，MVP 不预先引入额外参数和 WBI 签名逻辑；实际请求如出现接口要求变化再针对性调整。

仅 Go 后端从系统安全存储读取凭证并发送请求。Cookie 和 CSRF 不写入 SQLite、普通配置、日志或前端缓存；异常提示也不得回显这些内容。请求设置超时、支持取消，所有任务和测试发送共享账号级频率限制。

## 3. 发送响应与错误

响应通常为 JSON，先检查 HTTP 请求结果，再解析业务字段 `code`；不能仅因 HTTP 200 就判定成功。已知的部分业务代码：

| `code` | 含义 | MVP 处理方向 |
| --- | --- | --- |
| `0` | 请求被服务端接受 | 记录 `accepted`；不保证弹幕最终对观众可见 |
| `-101` | 未登录 | 归为凭证失效，停止发送并提示更新凭证 |
| `-111` | CSRF 校验失败 | 归为鉴权失败，不自动重试 |
| `-400` | 请求参数错误 | 记录拒绝原因，检查参数映射 |
| `1003212` | 弹幕超过长度限制 | 提示用户修改内容，不截断后重发 |
| `10031` | 发送过于频繁 | 暂停或停止相关发送，遵守平台限制，不尝试绕过 |
| 其他非零值 | 未知业务拒绝 | 保留脱敏后的错误类别和提示，默认不重试 |

响应中的 `message` / `msg` 可以辅助诊断，但业务分支应以可识别的代码为主，展示和持久化前要限制、脱敏错误文本。发送已经发出但超时、连接中断或响应无法判读时，结果可能为 `unknown`，**不得自动重发**；只有能确认请求未发出的失败才考虑按 PRD 进行有限重试。发送尝试与恢复规则见 [SQLite Schema 与 Migration 设计](SQLite%20Schema%20与%20Migration%20设计.md)。

## 4. 实现前需要核实的事项

- 核对直播间短号解析、主播信息字段及已停播直播间的返回情况；不假设所有房间都能发送弹幕。
- 确认当前接口实际允许的弹幕长度、最短发送间隔、限制时返回的业务代码；不要仅凭社区示例把这些值写死为平台规则。
- 确认发送接口是否继续接受上述最小参数，以及 Cookie / CSRF 是否存在新增要求。若接口变更，只调整 Go 端 B 站客户端及本参考文档，不让前端或调度器直接依赖第三方 JSON 结构。
- 自动化测试使用 `httptest` 模拟成功、业务拒绝、鉴权失败、限流、超时与不确定结果，不触发真实弹幕。必要的真人账号验收应单独、手动进行，且不得保存真实凭证或原始敏感响应。

## 5. 首版扫码登录与续期研究与实现

2026-10-12 已核查社区资料和第三方作者实现，`internal/account/passport.go` 据此接入协议，并通过 `httptest` 使用假 Cookie / RT 验证异常分支。**尚未通过真实账号端到端验证，不能据此宣称 H0 的实机门槛或 G3 已完成。** 浏览器插件同步在首版发布后实施。两种目标认证模式见 [PRD FR-003](PRD.md#fr-003凭证安全)：

| 模式 | 登录凭证 | 是否由 dan-tick 续期 |
| --- | --- | --- |
| 应用内扫码（首版必做） | 应用独立登录获得 `SESSDATA`、`bili_jct`、账号 UID 和该会话的 `refresh_token` | 是，受控检查和轮换；真实会话独立性待验收 |
| 浏览器插件同步（首版后，首期 Chrome） | 只同步浏览器会话最新的 `SESSDATA`、`bili_jct` | 否，`refresh_token`（网页端常见持久化键为 `ac_time_value`）由浏览器自行管理；扩展不读取或传给应用 |

旧版手动填写两个 Cookie 是过渡入口，H1 同步移除设置页和公开 Binding 的手动保存入口。迁移识别既存 Keyring 双字段数据，保留凭证和任务，要求用户明确扫码重新绑定。

### 5.1 协议依据与资料差异

Passport 路径以下均位于 `https://passport.bilibili.com/x/passport-login/web`。字段及状态依据社区 [扫码资料](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/login/login_action/QR.md) 和 [Cookie 刷新资料](https://github.com/alittlehuaji/bilibili-api-collect-mirror/blob/master/docs/login/cookie_refresh.md)：

| 步骤 | 方法 / 路径 | 本项目使用的合同 |
| --- | --- | --- |
| 生成二维码 | `GET /qrcode/generate` | 外层 `code = 0`，读取 `data.url` 和 `data.qrcode_key` |
| 查询扫码 | `GET /qrcode/poll` | 参数 `qrcode_key`；内层 `data.code`：`86101` 等待、`86090` 已扫码、`86038` 过期、`0` 确认；确认需要整组 Cookie 和 `refresh_token` |
| 检查续期 | `GET /cookie/info` | 当前 Cookie 与 `csrf`；读取 `data.refresh` 和毫秒 `timestamp` |
| 提取临时 CSRF | `GET https://www.bilibili.com/correspond/1/{path}` | 用 RSA-OAEP / SHA-256 加密 `refresh_{timestamp}` 后转小写 hex；从 HTML 的 `div#1-name` 提取 `refresh_csrf` |
| 轮换 | `POST /cookie/refresh` | 表单含旧 `csrf`、`refresh_csrf`、`source=main_web`、旧 `refresh_token`；需要新整组 Cookie、新 RT 及成功状态 |
| 确认 | `POST /confirm/refresh` | **新 Cookie / CSRF 与旧 RT**；只认业务 `code = 0`，禁止盲目重放 |

旧扫码资料描述 poll 响应直接设置 Cookie，或在 `passport.biligame.com/crossDomain` 的 URL 参数中返回 Cookie。第三方 [bilibili-cli PR #27](https://github.com/public-clis/bilibili-cli/pull/27) 的作者在 2026-08 报告新格式：成功 URL 为 `passport.biligame.com/x/passport-login/web/crossDomain?ticket=...`，需单独获取其 `Set-Cookie`；[作者提交的实现](https://github.com/ZeroMarker/bilibili-cli/blob/6962d5b/bili_cli/auth.py) 禁止自动重定向。该 PR 在本次查阅时仍未合并，这是**第三方实现及作者亲测的证据，尚非本项目实测，更非官方保证**。

客户端优先解析 poll 的完整 Cookie；缺失时兼容旧 URL 参数，或对新票据执行一次严格域名 / 路径校验的 HTTPS 请求。票据请求仅允许单个有限长度 `ticket`，拒绝 userinfo、端口、fragment 和任意额外参数，不携带原账号 Cookie、不访问 `Location`；只从直接 `200` / `302` 响应提取整组 Cookie。任何格式缺失或新的跨域需求均失败并提示重扫，不添加通用 SSO 或绕过验证的路径。二维码 URL 同样限制为已知的 Passport 登录路径。

### 5.2 会话、UID 与失败语义

本项目模型将版本 `1`、来源 `qr`、UID、两个 Cookie、RT 和轮换状态成组存入单项 Keyring。扫码候选的 `DedeUserID` 必须是有效 UID，再用候选会话调用 `nav` 独立核对账号身份；未确认候选不能覆盖当前凭证。发送和续期使用同一受控凭证入口，不能读到混合的新旧 Cookie。

`-101`、`-111` 和 `86095` 归为需要重新登录；未知非零业务码为拒绝，不回显上游消息。网络请求均有取消 / 超时和响应大小限制，忽略调用方 Cookie Jar、禁止自动重定向。刷新 POST 发出后的超时、HTTP 错误、JSON 损坏或新凭证缺失归为不确定结果，禁止自动重放；确认失败或本地写入失败也要求重新扫码。轮换前持久化 `pending` 标记，启动发现未完成轮换时拒绝使用旧会话，避免服务端已经成功但本地仍保存旧 Cookie 的静默回退。

社区资料指出确认更新会让旧 RT 对应的 Cookie 失效，但**失效范围、账号其它独立会话是否受影响，以及新票据流程的实际响应状态均尚待人工核验**。应用不复制浏览器 RT，不保证会话永久有效。

### 5.3 仍待首版人工验证

- 在 Windows / Linux 隔离账号上验证扫码确认、取消、180 秒到期、票据与 Cookie 下发、UID 校验及 Keyring 锁定 / 不可用反馈；真实操作必须另外获得用户明确授权，不保留原始 Cookie、RT 或敏感响应。
- 对照服务端实际结果验证检查续期、完整轮换、旧 RT 确认、其它会话失效范围、退出 / 重启后的 `pending` 恢复；同时核对新格式与上述社区资料是否仍一致。
- 核验平台允许的用途及授权。官方 [用户使用协议](https://www.bilibili.com/blackboard/protocal/licence.html) 第 5.6(3) 项对未经事先明确书面许可使用自动程序获取平台服务、内容、数据作出限制；本次未取得本项目用途许可。用户同意扫码与开源实现存在，均不能替代平台许可核验。

这些未完成项仍属于首版发布门槛；失败时不得恢复公开手动 Cookie 输入作为替代。

### 5.4 首版后浏览器插件边界

Chrome 扩展研究参考 Chrome 官方 [Cookies API](https://developer.chrome.com/docs/extensions/reference/api/cookies) 和 [Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging)：只在用户授权的 B 站站点与选定 Cookie Store 中读取两个 Cookie，用户授权并配对后通过本地原生消息连接交给 Go；Cookie 更新事件可能先报告移除再报告写入，不能把单个删除事件当作完整退出登录或直接覆盖 Keyring，需要重新读取整组并验证 UID。Chrome 未运行或通信失败时不保证持续同步。

H2 实施前还须核对 Windows / Linux 扩展安装、Native Messaging 接入及 Cookie 事件顺序。自动测试继续仅使用替身，真实账号规则与上文一致。

## 6. 上游出处

- [登录基本信息：导航栏用户信息](https://github.com/renmu123/bilibili-API-collect-next/blob/master/docs/login/login_info.md)
- [直播间基本信息：获取直播间信息、获取主播信息](https://github.com/renmu123/bilibili-API-collect-next/blob/master/docs/live/info.md)
- [直播间弹幕：发送直播弹幕](https://github.com/renmu123/bilibili-API-collect-next/blob/master/docs/live/danmaku.md)
- [Web 扫码登录（社区资料）](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/login/login_action/QR.md)
- [Web 端 Cookie 刷新（社区镜像）](https://github.com/alittlehuaji/bilibili-api-collect-mirror/blob/master/docs/login/cookie_refresh.md)
- [bilibili-cli 新扫码票据实现（未合并 PR）](https://github.com/public-clis/bilibili-cli/pull/27)

上游仓库说明其资料由社区收集，可能随时变化，并以 CC BY-NC 4.0 许可发布。本文件只概括本项目要用的接口与设计判断，不复制上游完整参数表、响应样例或其他业务接口；需要详细字段时直接查看原文。

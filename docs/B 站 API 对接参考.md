# B 站 API 对接参考

本文记录当前使用的第三方接口协议、兼容性差异和验证边界。产品行为见 [PRD](PRD.md)，凭证安全见 [FR-003](PRD.md#fr-003凭证安全)，发送记录与恢复见 [数据持久化设计](数据持久化设计.md#4-执行记录与发送尝试)，后端职责与验证方法见 [后端开发指南](../internal/AGENTS.md)。

社区资料提供协议线索，不构成 B 站对第三方接口稳定性或本项目用途的保证。当前客户端行为以各节链接的实现为准，尚未完成的核验与修正统一见 [待办](待办.md#未分配版本的核验与修正)。

## 1. 当前接口协议

### 1.1 登录状态与直播间信息

| 用途 | 方法与接口 | 当前读取的关键字段 |
| --- | --- | --- |
| 登录状态与账号资料 | `GET https://api.bilibili.com/x/web-interface/nav` | `code`、`data.isLogin`；已登录时读取 `mid`、`uname`、`face` |
| 直播间信息 | `GET https://api.live.bilibili.com/room/v1/Room/get_info` | 入参 `room_id`；返回 `data.room_id`、`uid`、`title`，封面使用 `user_cover` 或 `cover` |
| 主播名称 | `GET https://api.live.bilibili.com/live_user/v1/Master/info` | 入参为主播 `uid`；读取 `data.info.uname` |

`nav` 的 `code = 0` 且 `isLogin = true` 表示登录有效，`mid` 用于核对账号 UID；不代表具有某个直播间的发言权限。实现与测试见 [登录检查](../internal/account/status.go) 和 [登录检查测试](../internal/account/status_test.go)。

直播间短号或链接解析后，以 `get_info` 返回的真实 `room_id` 作为平台标识；主播名称需另查 `Master/info`。封面优先取 `user_cover`，再取 `cover`，地址校验和响应解析见 [直播间客户端](../internal/room/room.go)。当前客户端未读取 `live_status`。

直播间刷新、缓存和回写策略由 [直播间服务](../internal/room/service.go) 与 [页面刷新协调](../frontend/src/features/configuration/useRoomMetadataRefresh.ts) 维护。

### 1.2 发送直播弹幕

发送接口为 `POST https://api.live.bilibili.com/msg/send`，请求体使用 `application/x-www-form-urlencoded`。当前 [发送客户端](../internal/danmaku/client.go) 提交以下参数：

| 参数 | 当前取值 |
| --- | --- |
| `roomid` | 真实平台房间 ID |
| `msg` | 弹幕文本 |
| `rnd` | 当前 Unix 秒时间戳 |
| `csrf` | 同一会话 Cookie 中的 `bili_jct` |
| `fontsize` | `25` |
| `color` | `16777215`，十进制白色 |

Cookie 使用 `SESSDATA` 与 `bili_jct`。当前客户端未发送 `csrf_token`、`mode`、`bubble` 或 WBI 签名字段；这描述当前请求，不能据此承诺上游永远不需要额外参数。

任务创建 / 更新、测试发送登记及最终 HTTP 发送共用 [消息校验](../internal/danmaku/message.go)：拒绝空内容、无效 UTF-8 和控制字符，并在提交前检查长度，不静默截断。任务继续去除首尾空白并过滤空消息；测试发送校验原文。前端使用相同规则提供字段提示，Go 负责最终校验；无效测试输入不创建执行记录。

当前官方网页的额度来自账号 / 房间接口 `getInfoByUser` 的 `data.property.danmu.length`，由播放器转换为 `danmu_length` 后供输入组件使用；缺失时网页回退到 **20 个计量单位**。网页按以下权重计算普通弹幕，并非统一的“20 个汉字”：

| 字符 | 计量单位 |
| --- | --- |
| 可打印 ASCII（`U+0020`～`U+007E`） | 1 |
| 其他 BMP 字符，如普通汉字、全角标点、组合标记 | 2 |
| 补充平面字符，如常见 emoji | 4 |

本应用采用网页的 **20 单位兜底值作为本地保守上限**，允许离线编辑任务，不为保存配置读取凭证或请求平台额度。对应纯 ASCII 20 个、普通汉字 10 个、常见单码点 emoji 5 个；混合内容累加，多码点表情分别计量。此上限是本应用当前取舍，不代表所有账号的实际发言额度；具有更高额度的账号也受本地上限约束，平台仍可能因权限或内容规则拒绝发送。既有任务不自动截断，更新和发送时同样校验；服务端 `1003212` 保留为长度拒绝兜底。核验来源与边界见[第 3 节](#3-资料来源与验证限制)。

### 1.3 扫码登录与续期

以下相对路径位于 `https://passport.bilibili.com/x/passport-login/web`：

| 步骤 | 方法 / 路径 | 使用的协议字段 |
| --- | --- | --- |
| 生成二维码 | `GET /qrcode/generate` | 外层 `code = 0`，读取 `data.url`、`data.qrcode_key` |
| 查询扫码 | `GET /qrcode/poll` | 入参 `qrcode_key`；内层 `data.code`：`86101` 等待、`86090` 已扫码、`86038` 过期、`0` 确认；确认后提取完整会话 Cookie 与 `refresh_token` |
| 检查续期 | `GET /cookie/info` | 当前 Cookie 与 `csrf`；读取 `data.refresh`、毫秒 `timestamp` |
| 轮换凭证 | `POST /cookie/refresh` | 旧 `csrf`、`refresh_csrf`、`source=main_web`、旧 `refresh_token`；要求外层 `code = 0`、内层 `data.status = 0` 及新整组 Cookie、新 RT |
| 确认轮换 | `POST /confirm/refresh` | **新 Cookie / CSRF 与旧 RT**；业务 `code = 0` 才确认成功 |

临时 `refresh_csrf` 来自 `GET https://www.bilibili.com/correspond/1/{path}`：用 RSA-OAEP / SHA-256 加密 `refresh_{timestamp}`，转为小写 hex 生成路径，再从 HTML 的 `div#1-name` 提取值。

协议解析与网络边界见 [Passport 客户端](../internal/account/passport.go)，扫码绑定及轮换协调见 [账号会话](../internal/account/session.go)；相应测试为 [协议测试](../internal/account/passport_test.go) 与 [会话测试](../internal/account/session_test.go)。

## 2. 兼容性与失败边界

### 2.1 二维码与跨域票据

二维码 URL 采用精确的 HTTPS 域名 / 路径白名单，包含 Passport 登录页及 `account.bilibili.com/h5/account-h5/auth/scan-web`。完整允许集合由 [Passport 客户端](../internal/account/passport.go) 维护。URL 不允许 userinfo、端口、fragment 或转义路径 `RawPath`；`qrcode_key` 须恰好一项且与生成响应一致，其他页面参数可以存在。

扫码确认优先解析 poll 返回的完整 Cookie。缺失时，当前客户端支持两种兼容格式：

| 格式 | 处理方式 |
| --- | --- |
| `https://passport.biligame.com/crossDomain` | 从 URL 中解析 `SESSDATA`、`bili_jct`、`DedeUserID`，三者各一项；不发起网络请求 |
| `https://passport.biligame.com/x/passport-login/web/crossDomain?ticket=...` | 查询中仅允许一个非空 `ticket`，最多 2048 字节且无空白或控制字符；发起一次不带现有 Cookie 的 GET，只从直接 `200` / `302` 响应提取整组 `Set-Cookie`，不跟随 `Location` |

票据请求的单参数规则不适用于二维码 URL 或旧 Cookie 参数格式。格式不完整或出现未知域名 / 路径时明确失败，不扩大为通用跨域登录入口。扫码候选的 `DedeUserID` 必须有效，并用候选会话的 `nav.mid` 独立核对身份。

### 2.2 发送响应

发送客户端检查 HTTP 请求结果及 JSON 业务 `code`，使用以下分类：

| `code` | 客户端分类 |
| --- | --- |
| `0` | 接受；实际可见性的含义见 [FR-021](PRD.md#fr-021发送结果处理) |
| `-101`、`-111` | 凭证 / CSRF 鉴权失败 |
| `-400` | 参数错误 |
| `1003212` | 弹幕过长 |
| `10031` | 发送频率限制 |
| 其他非零值 | 未知业务拒绝 |

当前客户端只解析 `code`，不使用上游 `message / msg` 作为对外错误文本。请求结果无法确定时记录为未知，重试与停止规则引用 [FR-022](PRD.md#fr-022发送超时与重试)，当前执行链路不自动重试。分类与不确定结果的验证见 [发送客户端测试](../internal/danmaku/client_test.go)。

公开发送调用链未提供可确认的统一最短发送间隔；不能从网页没有等待逻辑推断服务端不限频。当前 [共享发送器](../internal/danmaku/sender.go) 保持请求结果返回后至少 15 秒的账号级间隔，这是本地保守策略，不是平台最短间隔或免于限频的保证。循环任务可配置正整数秒间隔，实际还受共享发送器约束；收到 `10031` 后记录 `rate_limited` 并停止本次执行。HTTP 429 等非 200 响应仍按结果不确定处理，不自动重试，也不推断弹幕一定未发送。

### 2.3 续期失败

Passport 外层业务码 `-101`、`-111`、`86095` 分类为会话失效，其他非零码分类为拒绝。网络不可用、响应损坏和明确业务拒绝分别保留错误类别，不能全部视为凭证过期。

会话协调按流程所处阶段处理失败：

| 阶段 | 当前处理 |
| --- | --- |
| 续期检查失败 | 明确失效时要求重新扫码；其他失败返回对应错误类别，不一律判为过期 |
| 首次保存旧会话 `pending` 失败 | 返回存储错误，保留原有效凭证；未发起轮换，也未标记为必须重扫。当前及错误缓存期间的发送仍会返回错误 |
| 旧 `pending` 已保存，进入受控轮换 | 暂停使用会话；随后流程失败时保守要求重新扫码，不自动重放 |
| 保存新组、UID 核验、确认或最终 `ready` 写入失败 | 保留未完成轮换状态，要求重新扫码 |

轮换 POST 的请求错误、响应损坏、内层状态缺失 / 异常或新凭证缺失按不确定结果处理；外层明确拒绝仍保留失效 / 拒绝分类。会话协调要求重扫并不证明远端副作用已经发生：提取临时 CSRF 等轮换前步骤失败，也可能在已保存 `pending` 的流程中触发这一保守策略。

本地发现未完成轮换时拒绝继续使用旧会话，具体状态与恢复实现见 [账号会话](../internal/account/session.go) 和 [凭证存储](../internal/credential/store.go)。

## 3. 资料来源与验证限制

接口资料最初查阅于 2026-10-10，扫码与续期资料于 2026-10-12 核查。该日的隔离核验确认了应用独立扫码和一次主动轮换，新旧凭证实际变化且 UID 一致；生成接口返回的二维码 URL 使用了上文的 `account.bilibili.com` 路径。

核验时续期检查为 `refresh=false`，主动轮换不证明自然到期 / `refresh=true` 窗口。确认后立即与 30 秒两次检查中，旧 Cookie 仍可登录，因此不能假定确认立即使旧 Cookie 失效，也不能推断旧 RT 仍可刷新或其他浏览器会话受到何种影响。

跨平台 Keyring、退出重启及未完成轮换恢复、自然到期窗口和第三方使用条件的专项核验见 [待办](待办.md#未分配版本的核验与修正)。接口与业务输入限制可能随上游变化，不把社区示例当作永久平台规则。

2026-10-14 只读核验了[官方直播间网页](https://live.bilibili.com/21452505)所加载的公开脚本（网页标注构建日期 `2026.10.09`），长度依据如下：

- [网页 app 脚本](https://s1.hdslb.com/bfs/static/blive/blfe-live-room/static/js/app.3f7250563d6495ab0c32.js)：模块 `10162` 的 `DANMAKU_SINGLE_UNIT_UNICODE_RANGES` 为 `[[32,126]]`；模块 `14198` 的 `utilDanmuLength` 对此范围之外的字符按 UTF-16 单元数乘 2 计量。
- [官方播放器脚本](https://s1.hdslb.com/bfs/static/bilibili-live-player/room-player.01626800.prod.min.js)：初始配置的 `length` 为 20，`initDanmakuConfig` 合并账号 `property.danmu` 覆盖；`getDanmakuConfig` 输出 `danmu_length`。
- [聊天输入脚本](https://s1.hdslb.com/bfs/static/blive/blfe-live-room/static/js/9649.f1405854aba06fdcb275.js)：`onWatchChatInput` 将上述加权长度直接与 `danmakuLengthLimit` 比较，没有额外乘 2。网页自身会截断，本应用依 FR-009 改为提示并保留原文。

匿名 `GET https://api.live.bilibili.com/xlive/web-room/v1/index/getInfoByUser?room_id=21452505` 返回 `-101`，未获得登录账号的实际额度。本次未使用账号凭证或发送生产弹幕；静态脚本能证明网页计量和兜底逻辑，不能证明所有字符的服务端计量、账号额度、实际最低间隔或唯一限频码。`10031` / `1003212` 的分类依据仍为下列社区协议资料，并以本地 `httptest` 回归验证，未宣称生产触发实测通过。

本地校验已通过 Go / 前端行为测试、临时 SQLite 与模拟 HTTP 回归，包含中文、ASCII、emoji、混合与组合字符边界、提交原子性及旧任务超长内容不截断。2026-10-14 的 Linux 开发版以隔离数据目录和 D-Bus 会话运行，通过 Wails MCP 核验真实 WebView 的超长输入保留及字段提示、Binding 的安全 `too_long` 错误，以及无新增任务 / 执行 / 发送尝试记录；本次未验证 Windows WebView。

资料来源：

- [登录状态与账号资料](https://github.com/renmu123/bilibili-API-collect-next/blob/master/docs/login/login_info.md)
- [直播间与主播信息](https://github.com/renmu123/bilibili-API-collect-next/blob/master/docs/live/info.md)
- [直播弹幕发送](https://github.com/renmu123/bilibili-API-collect-next/blob/master/docs/live/danmaku.md)
- [Web 扫码登录资料](https://github.com/bilibili-plugins/bilibili-api-collect/blob/master/docs/login/login_action/QR.md)
- [Cookie 刷新资料](https://github.com/alittlehuaji/bilibili-api-collect-mirror/blob/master/docs/login/cookie_refresh.md)
- [扫码票据变更的第三方报告](https://github.com/public-clis/bilibili-cli/pull/27)及其[固定提交实现](https://github.com/ZeroMarker/bilibili-cli/blob/6962d5b/bili_cli/auth.py)
- [B 站用户使用协议](https://www.bilibili.com/blackboard/protocal/licence.html)

API-collect 系列引用资料的许可声明为 CC BY-NC 4.0；bilibili-cli 使用 [Apache-2.0](https://github.com/public-clis/bilibili-cli/blob/main/LICENSE)。固定提交与第三方报告仅作为兼容性线索，不代表本项目实测或官方保证。未排期的浏览器插件同步及其准入条件统一见 [Roadmap](ROADMAP.md#3-暂未分配版本的长期方向)。

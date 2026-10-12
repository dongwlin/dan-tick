# B 站 API 对接参考

本文记录 dan-tick MVP 需要的 B 站 Web 接口及对接注意事项，供后续 Go 后端实现时查阅。业务范围与发送规则以 [PRD](PRD.md) 为准，后端职责以 [技术栈选型](技术栈选型.md) 为准；本文不重复定义调度、存储或界面方案。

接口依据 [bilibili-API-collect-next](https://github.com/renmu123/bilibili-API-collect-next) 的社区整理资料（查阅于 2026-10-10）。**这些不是 B 站承诺稳定的官方接口，以下为待实现的对接方案，并非已用真实账号验证的结果。** 上游文档可能过时；接入前应重新核对参数与返回值。

## 1. MVP 接口清单

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

## 5. 上游出处

- [登录基本信息：导航栏用户信息](https://github.com/renmu123/bilibili-API-collect-next/blob/master/docs/login/login_info.md)
- [直播间基本信息：获取直播间信息、获取主播信息](https://github.com/renmu123/bilibili-API-collect-next/blob/master/docs/live/info.md)
- [直播间弹幕：发送直播弹幕](https://github.com/renmu123/bilibili-API-collect-next/blob/master/docs/live/danmaku.md)

上游仓库说明其资料由社区收集，可能随时变化，并以 CC BY-NC 4.0 许可发布。本文件只概括本项目要用的接口与设计判断，不复制上游完整参数表、响应样例或其他业务接口；需要详细字段时直接查看原文。

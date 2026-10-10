# HTTP API 设计规范

> **规范范围**：本文件定义 HTTP API 设计细节（路由版本、顶层结构、错误码格式与分类、HTTP 状态码映射、分页、列表、空值、字段命名）。

## 1. 顶层结构

所有业务接口统一返回：

```json
{
  "code": "OK",
  "message": "success",
  "data": {}
}
```

同一类接口中，同一字段保持固定类型，不在不同场景下返回不同类型。字段名使用完整语义，不缩写（`message` 不用 `msg`）。

数据返回方式：

- 单对象详情：`data` 直接返回对象
- 列表：统一放在 `data.list`
- 分页：统一放在 `data.list` 和 `data.pagination`

---

## 2. 路由版本

所有业务接口的路由统一携带版本前缀，版本号紧跟路径起点，如 `GET /v1/users`。

### 2.1 版本前缀格式

- 版本号格式固定为 `v` + 正整数：`v1`、`v2`、`v3`……
- 不使用日期（如 `/v2026-08`）、小数（如 `/v1.2`）或 `latest` 等特殊标记
- 业务路由必须携带版本前缀，未带版本前缀的业务路由不得注册

### 2.2 版本语义

- **非破坏性变更**（新增可选字段、新增端点、新增错误码等）留在当前版本内完成，不升版本
- **破坏性变更**（删除/重命名字段、变更字段类型或语义、移除端点等）必须发布新版本 `v(n+1)`，旧版本并行运行过渡后统一下线
- 不得在已发布的版本内做破坏性变更

### 2.3 与代码组织对齐

URL 版本必须对应清晰的代码边界：新增版本只复制传输契约层（DTO、序列化与绑定），业务用例层默认跨版本复用，不因新增 `/v2` 机械复制。目录落点由项目采用的架构 Profile 定义。

### 2.4 例外

健康检查等基础设施端点（如 `/healthz`）不参与业务版本，可直接挂在根路径。

---

## 3. 标准响应结构

### 3.1 成功响应

```json
{
  "code": "OK",
  "message": "success",
  "data": {}
}
```

### 3.2 失败响应

```json
{
  "code": "BASE.PARAM.VALIDATION_FAILED",
  "message": "invalid parameter",
  "data": {}
}
```

如需返回错误细节，可放入 `data`：

```json
{
  "code": "BASE.PARAM.VALIDATION_FAILED",
  "message": "invalid parameter",
  "data": {
    "fieldErrors": [
      {
        "field": "email",
        "reason": "email is required"
      }
    ]
  }
}
```

---

## 4. 顶层字段规范

### 4.1 `code`

业务状态码，用于表达业务处理结果，不复用 HTTP 状态码，也不从 HTTP 状态码派生。

`code` 是客户端可以分支处理的稳定机器契约。服务端由结构化的应用错误类型携带 `code`，禁止从错误文本推导错误码。

| 值 | 含义 |
|----|------|
| `"OK"` | 成功 |
| `"ERROR"` | 未分类系统错误（500） |
| `BASE.MODULE.DETAIL` | 分类业务错误 |

格式遵循 `BASE.MODULE.DETAIL` 三段式：

| code                           | 场景                | HTTP 状态码 |
| ------------------------------ | ----------------- | -------- |
| `BASE.PARAM.VALIDATION_FAILED` | 参数错误，参数校验失败       | 400      |
| `BASE.NOT_FOUND.USER`          | 资源不存在，目标资源未找到     | 404      |
| `BASE.AUTH.TOKEN_EXPIRED`      | 认证失效，登录态失效或认证信息无效 | 401      |
| `BASE.AUTH.FORBIDDEN`          | 权限不足，已认证但无当前操作权限  | 403      |
| `BASE.BIZ.CONCURRENT_UPDATE`   | 资源版本冲突            | 409      |
| `BASE.BIZ.USER_DISABLED`       | 不满足业务前置条件         | 422      |
| `BASE.BIZ.RATE_LIMITED`        | 请求频率或业务额度受限       | 429      |
| `"ERROR"`                      | 系统异常，未预料的异常       | 500      |

> 具体业务 `code` 在服务端集中定义并同步接口文档；禁止在协议适配层临时发明字面量。
>
> 业务 `code` 不应直接使用 `400`、`404`、`500`，也不应扩展为 `40001`、`50001` 这类带 HTTP 含义的编码。

### 4.2 `message`

返回简短、可读的提示信息。

- 成功时用 `"success"`
- 失败时提供可读的错误描述
- 不返回过长调试信息，不暴露堆栈或内部异常细节
- `message` 只用于展示或日志辅助，可以本地化或调整；前后端都不得依赖其文本执行程序分支

### 4.3 `requestId`（响应头）

请求唯一标识，用于链路追踪和问题排查。**不放入响应体**，由服务端写入响应头：

```http
X-Request-Id: a1b2c3d4
```

- 由服务端生成，格式建议使用短 UUID 或 trace ID 截取
- 前端遇到异常时应将响应头中的 `X-Request-Id` 一并上报，便于后端定位日志
- 跨服务调用时透传，确保全链路可追踪

---

## 5. 空值规范

### 5.1 无额外数据时

统一返回空对象 `{}`：

```json
{
  "code": "OK",
  "message": "success",
  "data": {}
}
```

### 5.2 列表为空时

列表字段必须返回空数组 `[]`，不得返回 `null`：

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "list": []
  }
}
```

### 5.3 对象字段为空时

对象字段默认返回空对象 `{}`；只有 `null` 本身具有明确业务语义时才返回 `null`。

### 5.4 数值和布尔字段

- 计数类字段优先返回 `0`
- 布尔类字段优先返回 `true / false`
- 仅在明确存在"未知/未设置"语义时使用 `null`

---

## 6. 单对象详情接口

`data` 直接返回对象，不额外包一层资源名：

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "id": 1,
    "name": "pp",
    "status": "enabled"
  }
}
```

前端可直接使用：`const user = res.data`

---

## 7. 列表接口规范

### 7.1 非分页列表

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "list": []
  }
}
```

### 7.2 列表字段命名

统一命名为 `list`，不混用 `items`、`rows`、`records`、`dataList` 等命名。

---

## 8. 分页接口规范

### 8.1 分页模式选型

| | 页码分页（Offset） | 游标分页（Cursor） |
|--|--|--|
| 跳页 | 支持 | 不支持 |
| 深分页性能 | 差（OFFSET 大偏移） | 恒定 |
| 数据一致性 | 并发写入时会偏移漂移 | 稳定 |
| 典型 UI | 传统页码翻页 | 无限滚动 / 加载更多 |

- **管理后台、搜索结果、需要跳页** → 页码分页
- **信息流、消息列表、时间线、大数据量顺序浏览** → 游标分页

### 8.2 页码分页响应结构

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "list": [],
    "pagination": {
      "page": 1,
      "pageSize": 20,
      "total": 0,
      "totalPages": 0,
      "hasMore": false
    }
  }
}
```

### 8.3 页码分页字段定义

| 字段 | 类型 | 说明 |
|------|------|------|
| `page` | int | 当前页码，从 `1` 开始 |
| `pageSize` | int | 每页条数 |
| `total` | int | 总记录数 |
| `totalPages` | int | 总页数，计算方式：`totalPages = ceil(total / pageSize)` |
| `hasMore` | bool | 是否还有下一页，规则：`hasMore = page < totalPages` |

### 8.4 页码分页完整示例

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "list": [
      { "id": 101, "name": "Tom" },
      { "id": 102, "name": "Jerry" }
    ],
    "pagination": {
      "page": 1,
      "pageSize": 20,
      "total": 95,
      "totalPages": 5,
      "hasMore": true
    }
  }
}
```

### 8.5 页码分页请求参数

页码分页统一使用 `page` 和 `pageSize`：

```
GET /v1/users?page=1&pageSize=20
```

- `page` 从 `1` 开始
- `pageSize` 设默认值（如 `20`）
- `pageSize` 设上限（如 `100`），避免一次拉取过多数据

不推荐混用 `pageNo`、`page_num`、`size`、`rows`、`limit` 等命名。

### 8.6 游标分页

适用于顺序浏览场景（信息流、消息列表、时间线），不支持随机跳页。

#### 8.6.1 请求参数

```
GET /v1/orders?limit=20                          # 首页
GET /v1/orders?cursor=eyJpZCI6MTAwfQ==&limit=20  # 翻页
```

- `cursor`：上一页最后一条记录的编码游标，首页不传
- `limit`：每页条数，默认 `20`，上限 `100`

不推荐混用 `after`、`next`、`position` 等命名。

#### 8.6.2 响应结构

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "list": [],
    "pagination": {
      "nextCursor": "eyJpZCI6MTAwfQ==",
      "hasMore": true
    }
  }
}
```

#### 8.6.3 字段定义

| 字段 | 类型 | 说明 |
|------|------|------|
| `nextCursor` | string | 下一页游标，无更多数据时为 `null` |
| `hasMore` | bool | 是否还有下一页 |

#### 8.6.4 游标生成规则

- 默认使用主键 ID 作为游标依据
- 游标内容为 JSON 编码后 Base64，例如 `{"id":100}` → `eyJpZCI6MTAwfQ==`
- 前端不应解析游标内容，仅透传
- 后端解码游标后用于 `WHERE` 条件，如 `WHERE id > 100`

---

## 9. 创建、更新、删除接口

### 9.1 无额外返回数据

```json
{
  "code": "OK",
  "message": "success",
  "data": {}
}
```

### 9.2 创建后返回对象

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "id": 1001,
    "name": "pp",
    "status": "enabled"
  }
}
```

---

## 10. 批量操作接口

建议返回处理结果摘要：

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "successCount": 8,
    "failedCount": 2,
    "failedItems": [
      { "id": 101, "reason": "already deleted" },
      { "id": 102, "reason": "permission denied" }
    ]
  }
}
```

---

## 11. HTTP 状态码使用建议

HTTP 状态码与业务状态码分开使用。若同时返回 HTTP 状态码和业务 `code`，两者语义应保持一致。`404 Not Found` 优先用于接口路由不存在；业务对象未找到同样返回 404（`BASE.NOT_FOUND.*`）。

| 应用错误 Kind | HTTP 状态码 | 含义 |
| --- | --- | --- |
| 成功（无错误） | `200 OK` | `code` 为 `"OK"` |
| `InvalidArgument` | `400 Bad Request` | 请求参数格式或基础校验错误 |
| `Unauthenticated` | `401 Unauthorized` | 未登录、凭证无效或认证失效 |
| `PermissionDenied` | `403 Forbidden` | 已认证，但无当前操作权限 |
| `NotFound` | `404 Not Found` | 路由或业务资源不存在 |
| `Conflict` | `409 Conflict` | 当前资源状态与请求冲突，例如重复创建或并发版本冲突 |
| `FailedPrecondition` | `422 Unprocessable Content` | 请求格式正确，但不满足业务前置条件 |
| `RateLimited` | `429 Too Many Requests` | 请求频率或业务额度限制；按需返回 `Retry-After` |
| `InternalKind` 或无法识别的错误 | `500 Internal Server Error` | 未预期内部异常，`code` 为 `"ERROR"` |

---

## 12. 字段命名规范

JSON 字段统一使用 `camelCase`。

| 推荐 | 不推荐 |
|------|--------|
| `pageSize` | `page_size` |
| `totalPages` | `total_pages` |
| `hasMore` | `has_more` |
| `successCount` | `success_count` |

---

## 13. 前端处理约定

```js
const { code, message, data } = response

if (code !== "OK") {
  // 程序分支使用 code；message 仅用于安全的默认展示。
  throw new ApiError(code, message, data)
}

const detail = data           // 单对象详情直接用
const list = data.list || []  // 列表统一读 data.list
const pagination = data.pagination || {}  // 分页统一读 data.pagination
```

---

## 14. 禁止事项

- `message` 写成 `msg`
- `code` 使用数字或含 HTTP 含义的编码（如 `1001`、`40001`）
- `requestId` 放入响应体（应放响应头 `X-Request-Id`）
- 单对象详情额外套资源名（如 `data.user`）
- 列表字段命名不统一（`items`、`rows`、`records` 混用）
- 列表为空时返回 `null` 而非 `[]`
- 分页字段散落在 `data` 顶层而不放入 `pagination`
- 游标分页返回 `page`、`total`、`totalPages` 等页码分页字段
- 前端解析或拼接游标内容
- 业务路由未携带版本前缀（如直接注册 `/users`）
- 版本号放在查询参数或请求头中（应放在路径前缀）
- 在已发布版本内做破坏性变更
- 使用 `v1.2`、日期等非 `vN` 格式的版本号

---


# 前端 SPA 基础技术栈

## 1. 适用范围

本文是 Vite + TypeScript 单页应用的共享技术 Profile，定义 React 与 Vue 项目共同采用的运行时、构建、数据、样式、测试和 CI 基线。

- React 专属选型由 React Profile 定义，Vue 专属选型由 Vue Profile 定义。
- 目录与依赖边界见《[[前端应用架构规范]]》。
- 测试策略见《[[前端测试规范]]》。
- API 与认证方案由项目选择的认证 Profile 定义。

默认交付形态是客户端渲染 SPA。SSR、SSG、React Server Components、Next.js 与 Nuxt 不属于本 Profile；确有 SEO、首屏渲染或服务端路由要求时，应另选 Profile。

## 2. 选型原则

### 2.1 一个问题只保留一个默认答案

同一应用不得同时引入职责重叠的基础方案，例如：

- 两套路由器；
- 两套服务端状态缓存；
- 原生 `fetch` Client 与 Axios Client 并存；
- 两套全量 Utility CSS 或组件系统；
- Vitest 与 Jest 并存。

替换默认技术时必须说明现有方案无法满足的具体约束，并用 ADR 记录收益、迁移成本和退出方案。个人偏好不是引入第二套方案的理由。

### 2.2 依赖按实际需求引入

依赖分为：

- **共享基线**：本 Profile 中每个项目都使用的能力；
- **框架依赖**：由 React / Vue Profile 选择；
- **按需依赖**：只有真实需求出现后才安装，例如 Store、复杂表单库、i18n、PWA 和监控 SDK。

禁止为了“以后可能用到”预装按需依赖。

### 2.3 稳定版本优先

新项目使用本 Profile 对应的稳定大版本；大版本升级作为独立任务验证类型检查、测试、Storybook、生产构建和关键 E2E。

依赖版本与 pnpm 版本必须由仓库文件锁定，CI 不使用浮动的 `latest`。

## 3. 共享基线

| 领域 | 默认选择 | 版本基线 | 定位 |
| --- | --- | --- | --- |
| JavaScript 运行时 | Node.js | 24 LTS | 本地开发与 CI |
| 包管理器 | pnpm | 11.x | 唯一包管理器与 Lockfile 来源 |
| 语言 | TypeScript | 6.x | 业务代码与配置类型检查 |
| 开发与构建 | Vite | 8.x | Dev Server、构建与插件容器 |
| 服务端状态 | TanStack Query | 5.x | 请求生命周期、缓存、失效与重试 |
| HTTP | 原生 `fetch` + 薄封装 | 浏览器标准 | 统一传输边界 |
| API 代码生成 | Orval | 8.x | 从 OpenAPI 生成类型与请求函数 |
| 运行时校验 | Zod | 4.x | 外部数据、表单和配置边界 |
| 样式 | Tailwind CSS | 4.x | Utility CSS 与 Design Token |
| Lint | ESLint Flat Config | 当前稳定版 | 正确性与框架规则 |
| 格式化 | Prettier | 当前稳定版 | Markdown、JSON、CSS 与源码格式化 |
| 测试执行 | Vitest | 4.1+ | 单元、组件与页面集成测试 |
| DOM 行为测试 | Testing Library | 当前稳定版 | 用户视角查询与交互 |
| HTTP Mock | MSW | 2.x | 在网络边界模拟 API |
| 组件工作台 | Storybook | 10.x | 场景、交互与可访问性测试 |
| E2E | Playwright | 当前稳定版 | 部署应用的关键用户流程 |

版本基线表示允许使用的稳定大版本，不要求所有项目同步升级到最新小版本。安全修复应及时合入，破坏性升级按独立任务处理。

## 4. 运行时与包管理

### 4.1 Node.js

- 项目统一以 Node.js 24 LTS 为本地和 CI 基线。
- 仓库只维护一个权威 Node 版本来源，例如 `.node-version` 或项目现有版本管理文件。
- 浏览器能力由目标浏览器范围决定，不因 Node.js 支持某 API 就假设浏览器支持。

### 4.2 pnpm

- 项目只提交 `pnpm-lock.yaml`，禁止同时提交 npm、Yarn 或 Bun Lockfile。
- `package.json` 必须通过 `packageManager` 锁定实际验证过的 pnpm 版本。
- CI 使用 Frozen Lockfile 安装；Lockfile 与 `package.json` 不一致时失败。
- 直接 import 的包必须声明为直接依赖，不依赖幽灵依赖或提升后的目录结构。

## 5. TypeScript 与 Vite

### 5.1 TypeScript

业务源码、Vite 配置与测试默认使用 TypeScript，并开启 `strict`。

- 新增代码不得用 `any`、非空断言或无依据的类型强制转换绕过边界建模。
- 无法确定的数据先用 `unknown`，再经 Zod 或类型守卫收窄。
- 默认启用 `noUncheckedIndexedAccess` 与 `verbatimModuleSyntax`。

推荐基线：

```json
{
  "compilerOptions": {
    "strict": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "noEmit": true,
    "noUncheckedIndexedAccess": true,
    "verbatimModuleSyntax": true
  }
}
```

### 5.2 Vite

- 配置文件使用 `vite.config.ts` 与 ESM。
- Tailwind CSS v4 使用 `@tailwindcss/vite`。
- 环境变量只通过 `import.meta.env` 与统一校验入口访问。
- 只有允许暴露给浏览器的变量才使用 `VITE_` 前缀。
- 路径别名保持少量稳定，不为每层目录机械创建别名。

## 6. API、数据与状态

### 6.1 HTTP Client

默认使用浏览器原生 `fetch`，在 `src/lib/api/` 建立唯一薄封装，统一处理 Base URL、超时、取消、响应解析、应用错误映射和可观测性。

认证、token 与 401 重放由选中的认证 Profile 定义，不在业务 Feature 中重复实现。

默认不引入 Axios；只有标准 `fetch` 无法以可维护方式满足明确约束时，才通过 ADR 整体替换 Client。

### 6.2 API 代码生成

后端提供 OpenAPI 契约时，默认使用 Orval 生成 TypeScript 类型与请求函数。

生成代码必须：

- 可重复生成，禁止手工修改；
- 复用统一 HTTP Client；
- 不直接读取 token 或浏览器存储；
- 不在生成目录实现业务 Query Key、Toast 或页面跳转；
- 能由 CI 检测契约生成物是否过期。

### 6.3 TanStack Query

所有服务端数据默认由 TanStack Query 管理。业务层负责定义 Query Key、Query Option、Mutation、缓存失效和必要的乐观更新。

禁止用 Router、组件 state 或客户端 Store 复制同一份服务端数据。

### 6.4 客户端 Store

只有状态同时满足以下条件时才引入框架 Profile 指定的 Store：

- 跨多个无直接父子关系的页面共享；
- 不属于服务端缓存；
- 不适合表达为 URL；
- 生命周期长于单个页面。

Store 默认不持久化；确需持久化时必须定义版本、迁移和清理策略。

## 7. 表单与运行时校验

Zod 是共享 Schema 工具，用于环境变量、路由参数、表单输入、本地持久化恢复与不可信外部数据。

简单表单优先使用框架原生能力与 Zod。出现大量字段、动态数组、复杂联动或多步骤状态时，再使用框架 Profile 指定的表单库。

前端校验不能替代后端校验；服务端字段错误必须保留上下文并映射回对应字段。

## 8. 样式与组件系统

### 8.1 Tailwind CSS v4

Tailwind CSS v4 使用 CSS-first 配置。Design Token 通过 `@theme` 与 CSS 自定义属性定义；组件消费语义 Token，不直接散布品牌色值。

动态样式必须使用可静态发现的完整 Class 映射，禁止运行时拼接无法被构建工具发现的类名。

### 8.2 组件系统

具体组件系统由 React / Vue Profile 选择。一个项目默认只保留一套主组件系统；确需整体替换时记录 ADR，不逐页混搭。

### 8.3 小型能力

- 图标：使用框架对应的 Lucide 包或项目设计系统指定方案；
- 日期：优先原生 `Intl`，复杂运算再引入 `date-fns`；
- 动画：优先 CSS Transition，只有手势、编排或物理动画才引入专用库。

禁止为单个格式化函数或简单淡入动画引入大型依赖。

## 9. 测试基线

测试分层与行为规则以《[[前端测试规范]]》为准。共享工具固定为：

- Vitest：单元、组件和页面集成测试；
- Testing Library：用户视角测试；
- MSW：HTTP 边界 Mock；
- Storybook + Vitest addon：共享组件场景、交互与可访问性；
- Vitest Browser Mode：真实浏览器行为的组件测试；
- Playwright：部署应用的关键 E2E。

框架专属 Testing Library、测试宿主与辅助工具由对应框架 Profile 定义。

## 10. 代码质量与 CI

### 10.1 ESLint 与 Prettier

ESLint 使用 Flat Config，负责正确性、类型感知、框架规则、Query 规则、Import 与无用代码检查；Prettier 只负责格式化。

可由 ESLint、TypeScript 或其他静态检查可靠表达的规则，不重复依赖 Agent 阅读 Markdown 记忆执行。

### 10.2 稳定脚本语义

项目至少提供以下稳定语义的脚本：

```text
dev
build
typecheck
lint
format
test
test:run
test:coverage
test:e2e
storybook
storybook:build
```

具体命令由框架 Profile 和项目实际配置决定，但脚本语义不得随意改变。

### 10.3 CI 基线

每次合并至少验证：

1. Frozen Lockfile 安装；
2. API 生成物未过期；
3. 类型检查；
4. ESLint；
5. Vitest 普通测试与 Storybook Project；
6. Storybook 静态构建；
7. 生产构建。

Playwright E2E 至少在主分支、发布候选或具备稳定部署环境的合并请求上运行。

## 11. 按需能力

| 能力 | 默认方向 | 引入条件 |
| --- | --- | --- |
| 国际化 | 框架 Profile 指定 | 产品明确支持多语言 |
| PWA | `vite-plugin-pwa` | 明确需要离线、安装或推送 |
| 原生容器 | Capacitor | 需要系统 API 且 Web 能力不足 |
| 错误监控 | Sentry 或部署平台现有方案 | 生产环境需要错误与性能追踪 |
| WebSocket | 浏览器 WebSocket + Feature 封装 | 确有实时双向事件 |
| 查询缓存持久化 | TanStack Query 持久化插件 + IndexedDB | 明确需要重启后恢复数据 |

引入按需依赖前必须说明现有平台能力为何不足，并评估 Bundle、维护状态、许可证、兼容性和测试方式。

## 12. 非默认方案

下列技术并非永久禁止，但不属于普通 SPA 的默认答案：

- Next.js、Nuxt 等 SSR / 全栈框架；
- Axios、GraphQL Client 或 RPC Client；
- 第二套 Utility CSS 或组件系统；
- 自动文件路由；
- 微前端；
- Service Worker 与离线缓存；
- Monorepo 编排工具；
- 全局 Event Bus。

出现明确产品或组织约束时再单独评估，不把局部需求包装成全项目基础设施。

## 13. 新项目检查清单

- [ ] Node、pnpm 和 `packageManager` 已锁定。
- [ ] 只提交 `pnpm-lock.yaml`。
- [ ] TypeScript `strict` 与 Vite ESM 已启用。
- [ ] 已选择且只选择一个框架 Profile。
- [ ] 服务端数据只由 TanStack Query 管理。
- [ ] 所有 HTTP 请求经过统一 Client。
- [ ] 按需依赖只在出现真实需求后引入。
- [ ] CI 能完成类型检查、Lint、测试、Storybook 与生产构建。

## 14. 相关规范

- [[前端应用架构规范]]
- [[前端测试规范]]
- [[前端交互与可访问性规范]]

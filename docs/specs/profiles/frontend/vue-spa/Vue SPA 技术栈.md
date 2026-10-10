# Vue SPA 技术栈

## 1. 适用范围

本 Profile 定义 Vue SPA 的框架专属选型，叠加在共享前端基线（基础技术 Profile 与前端 Standard）之上生效。

## 2. 默认选型

| 领域 | 默认选择 | 约束 |
| --- | --- | --- |
| UI 框架 | Vue 3 | Composition API + `<script setup lang="ts">` |
| Router | Vue Router 4 | 显式路由表 |
| Query | `@tanstack/vue-query` 5 | 服务端状态唯一权威来源 |
| 客户端 Store | Pinia 3 | 按需，仅保存客户端状态 |
| 表单 | vee-validate + Zod | 复杂表单按需引入 |
| 无样式组件原语 | Reka UI | 键盘、焦点、ARIA 与受控状态基础 |
| Storybook | `@storybook/vue3-vite` | 与 Vite 配置保持一致 |
| 组件测试 | `@testing-library/vue` | 用户行为优先 |

## 3. Vue 约定

- 使用 Composition API、Single-File Component 与 `<script setup lang="ts">`，不新增 Options API 页面。
- Router 使用显式路由表；公开 URL 必须可审查，不依赖文件名自动生成。
- 跨组件逻辑优先提取为 Composable；Composable 使用 `use` 前缀并正确清理副作用。
- Pinia 不是普通 Composable 或 TanStack Query 的替代品。
- Pinia state/getter 需要解构时使用 `storeToRefs()`，action 可以直接解构。
- 复杂表单才引入 vee-validate；简单表单使用 Vue 原生响应式能力与 Zod。

## 4. Reka UI

- Reka UI 负责复杂控件的键盘、焦点、ARIA 和受控状态基础。
- 项目组件负责视觉、Variant 与业务无关的稳定接口。
- 默认不再并行引入 Element Plus 等第二套 Vue 组件系统。
- 项目叠加设计 Profile（例如亚克力设计系统）时，组件视觉服从该 Design Profile。

## 5. 测试工具

- 默认使用 `@testing-library/vue` 验证用户可观察行为。
- 需要精确验证 props、slots、emit、Teleport 或 Composable 宿主环境时，可以补充 Vue Test Utils。
- Storybook 使用 `@storybook/vue3-vite`。

更具体的测试边界见《[[前端测试规范]]》。

## 6. 类型检查与 ESLint

- Vue SFC 类型检查统一使用 `vue-tsc`。
- 除共享规则外，必须启用 Vue 官方 ESLint 规则与 Vue 可访问性规则。

## 7. 相关规范

- [[前端应用架构规范]]
- [[前端测试规范]]
- [[前端交互与可访问性规范]]

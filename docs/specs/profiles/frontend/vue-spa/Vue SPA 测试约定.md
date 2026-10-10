# Vue SPA 测试约定

## 1. 适用范围

本文补充《[[前端测试规范]]》在 Vue SPA 中的框架专属测试规则。

## 2. 默认工具

- `@testing-library/vue`
- `@testing-library/user-event`
- Vue Test Utils（按需）
- Vitest
- MSW
- `@storybook/vue3-vite`

## 3. 文件命名

- 组件：`ComponentName.vue`
- 测试：`ComponentName.test.ts`
- Story：`ComponentName.stories.ts`

## 4. Testing Library 与 Vue Test Utils

默认使用 Testing Library 验证用户可观察行为。

只有需要精确验证 props、slots、emit、Teleport 或 Composable 宿主环境时，才使用 Vue Test Utils。禁止因为 `wrapper.vm` 可访问就直接断言内部响应式状态或调用私有方法。

## 5. Pinia

- 每个测试建立独立 Pinia；
- Store 状态转换可以做单元测试；
- 页面测试通过公开 UI 行为与 Router / Query 协作验证；
- 禁止通过共享单例 Store 污染其他测试。

## 6. Composable

具有独立公共契约的 Composable 可以在最小宿主环境中直接测试。只服务单个组件的 Composable 应通过组件行为覆盖。

涉及 scope、watch 或生命周期的 Composable 测试必须正确卸载宿主，验证副作用清理。

## 7. Teleport

需要验证 Dialog、Popover 等 Teleport 行为时，测试必须提供并清理真实目标容器，或使用组件库推荐的受控测试方式。

## 8. 相关规范

- [[前端测试规范]]

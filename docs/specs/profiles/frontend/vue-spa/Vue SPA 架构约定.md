# Vue SPA 架构约定

## 1. 适用范围

本文补充《[[前端应用架构规范]]》在 Vue SPA 中的框架专属落地规则。

## 2. 目录补充

```text
src/
├── app/
│   ├── App.vue
│   ├── router.ts
│   ├── pinia.ts
│   └── query-client.ts
├── pages/
├── features/
├── components/
├── composables/
├── stores/
└── lib/
```

- `composables/` 只保存跨 Feature 复用、带状态或生命周期的逻辑。
- Feature 私有 Composable 留在 Feature 内。
- `stores/` 只保存真正跨 Feature 的客户端状态；Feature 私有 Store 与 Feature 共置。

## 3. Vue Router

- 路由表集中在 `app/router.ts`。
- 路由组件从 `pages/` 动态 import。
- 路由 meta 只表达 `requiresAuth`、layout 等声明式导航信息。
- 守卫使用 return-based 写法，不使用 `next()`。
- 在守卫内部获取 Pinia Store，避免模块加载阶段访问尚未安装的 Pinia。
- 深层 Feature 组件不直接读取 Router；由 Page 解析参数并向下传递。

## 4. Pinia

- Pinia 使用 Setup Store。
- state / getter 解构使用 `storeToRefs()`，action 可以直接解构。
- Store 之间不得在 setup 阶段形成循环读取。
- Pinia 不保存 TanStack Query 的服务端数据副本。
- 持久化 Store 必须定义版本、迁移与退出清理。

## 5. Composable

- 使用 `use` 前缀。
- 接收响应式输入时兼容 Ref / Getter，而不是只接收一次性值。
- 副作用必须在 scope 销毁时清理。
- 共享可变状态默认返回 readonly state 与显式 action。

## 6. 相关规范

- [[前端应用架构规范]]

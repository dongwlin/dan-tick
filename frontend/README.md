# dan-tick 正式前端

这里是 Wails 3 应用的 Vue 3 + TypeScript 前端，目前仍展示模板页面。业务页面按 [开发计划](../docs/开发计划.md) 逐步接入；已定稿的 Vanilla TypeScript 原型位于独立的 [`prototype/`](../prototype/README.md)，两者分别管理依赖和锁文件。

## 工具链与安装

使用 Node.js 24.18.0（见 [`.node-version`](.node-version)）和 pnpm 11.28.4（见 [`package.json`](package.json) 的 `packageManager`）。Node 的允许范围是 `>=24.18.0 <25`；pnpm 的安装配置位于 [`pnpm-workspace.yaml`](pnpm-workspace.yaml)，启用严格引擎检查及 7 天的依赖发布时间限制。

在本目录安装已锁定的依赖：

```sh
pnpm install --frozen-lockfile
```

[`pnpm-lock.yaml`](pnpm-lock.yaml) 必须提交，不提交 `node_modules/`、`dist/` 或 `bindings/`。当前直接依赖固定为 Vue 3.5.43、TypeScript 6.0.3、Vite 8.3.2、`@vitejs/plugin-vue` 6.0.9、`vue-tsc` 3.3.12、`@types/node` 24.19.1 和 `@wailsio/runtime` 3.0.0-beta.25；Runtime 与 Go 后端的 Wails 版本同步维护。

## 开发与构建

桌面开发需要 Go、Wails 3 CLI 和目标平台的 Wails 构建依赖。在仓库根目录执行：

```sh
wails3 dev
wails3 build
```

Wails 的 Taskfile 默认使用 pnpm，以 `--frozen-lockfile` 安装依赖，生成 Binding 后构建前端。桌面产物位于仓库根目录的 `bin/`。

Docker 交叉编译也先在主机通过 `wails3 task common:build:frontend` 准备 `frontend/dist/index.html`，再由容器编译 Go；现有平台 Docker Task 已包含此步骤。交叉编译容器不安装 Node 或包管理器，也不在缺少前端产物时回退安装依赖。

仅构建前端时，先在仓库根目录生成 Binding，再进入本目录：

```sh
wails3 generate bindings -clean=true -ts -i
cd frontend
pnpm run build
```

可执行脚本如下：

| 命令 | 用途 |
| --- | --- |
| `pnpm run dev` | 运行 Vite 开发服务器 |
| `pnpm run build` | 检查 Vue 源码、检查 Vite 配置，再构建 production 产物 |
| `pnpm run build:dev` | 执行同样的类型检查，再构建未压缩的 development 产物 |
| `pnpm run preview` | 预览已生成的前端产物 |

Vite 默认监听 `127.0.0.1:9245`，可以用 `WAILS_VITE_PORT` 调整。普通浏览器能预览页面，但 Wails Service / Events 的桌面行为需要在真实 Wails 窗口中验证。`build` 和 `build:dev` 使用 `vue-tsc` 检查 [`tsconfig.json`](tsconfig.json) 中的源码与生成 Binding，使用 `tsc` 检查 [`tsconfig.node.json`](tsconfig.node.json) 中的 `vite.config.ts`；独立 `typecheck`、Lint 和前端测试脚本将在后续工程单元引入。

## 配置与导入路径

[`vite.config.ts`](vite.config.ts) 保留 Vue 和 Wails Binding 插件，以及本地开发地址与端口。TypeScript 和 Vite 使用相同的导入别名：

| 别名 | 目标 |
| --- | --- |
| `@/*` | `src/*` |
| `@bindings/*` | `bindings/*` |

Vue 组件采用 `<script setup lang="ts">` 与 Composition API。Binding 由 Wails 生成，不手动编辑；当前生成命令扫描根 Go 包，Service 迁入 `internal/` 时需同步调整扫描范围和导入路径。

## 依赖升级

新增或升级依赖时，先确认真实使用场景，以及 Node、Vue、TypeScript、Vite 与 Wails 的兼容性。使用 pnpm 更新 `package.json` 与锁文件，保留固定的直接版本和 7 天发布时间限制，不用临时关闭安装策略来追新版本。工具链升级同时更新 `.node-version`、`engines`、`packageManager` 与相关文档。

升级后验证 `pnpm install --frozen-lockfile`、`pnpm run build`、`pnpm run build:dev` 和仓库根目录的 `wails3 build`。Wails 构建可能运行 `go mod tidy`，完成后检查 `go.mod` / `go.sum`，并用 `git diff --check`、`git status --short` 确认没有生成文件或无关变更。

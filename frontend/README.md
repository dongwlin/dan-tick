# dan-tick 正式前端

这里是 Wails 3 应用的 Vue 3 + TypeScript 前端，已建立 Vue Router 4（Hash History）的应用装配与占位页面、Tailwind v4 和主题视觉基础，正式业务页面尚未迁移。业务页面按 [开发计划](../docs/开发计划.md) 逐步接入；已定稿的 Vanilla TypeScript 原型位于独立的 [`prototype/`](../prototype/README.md)，两者分别管理依赖和锁文件。

## 工具链与安装

使用 Node.js 24.18.0（见 [`.node-version`](.node-version)）和 pnpm 11.28.4（见 [`package.json`](package.json) 的 `packageManager`）。Node 的允许范围是 `>=24.18.0 <25`；pnpm 的安装配置位于 [`pnpm-workspace.yaml`](pnpm-workspace.yaml)，启用严格引擎检查及 7 天的依赖发布时间限制。

在本目录安装已锁定的依赖：

```sh
pnpm install --frozen-lockfile
```

[`pnpm-lock.yaml`](pnpm-lock.yaml) 必须提交，不提交 `node_modules/`、`dist/` 或 `bindings/`。当前直接依赖固定为 Vue 3.5.43、TypeScript 6.0.3、Vite 8.3.2、`@vitejs/plugin-vue` 6.0.9、`vue-tsc` 3.3.12、`@types/node` 24.19.1、`@wailsio/runtime` 3.0.0-beta.25、`vue-router` 4.6.4、`tailwindcss` 与 `@tailwindcss/vite` 4.3.3；Runtime 与 Go 后端的 Wails 版本同步维护。

质量工具固定为 `@eslint/js` 10.0.1、ESLint 10.12.0、`typescript-eslint` 8.71.0、`@vue/eslint-config-typescript` 14.9.0、`eslint-plugin-vue` 10.11.1、`eslint-plugin-vuejs-accessibility` 2.6.0、`globals` 17.13.0、`eslint-config-prettier` 10.1.8 和 Prettier 3.9.9。安装保留 7 天发布时间策略，不为使用更新版本关闭限制。

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

| 命令                    | 用途                                                            |
| ----------------------- | --------------------------------------------------------------- |
| `pnpm run dev`          | 运行 Vite 开发服务器                                            |
| `pnpm run typecheck`    | 独立检查 Vue 源码、生成 Binding 和 Vite 配置的类型              |
| `pnpm run lint`         | 执行 ESLint 检查，warning 也会使检查失败                        |
| `pnpm run lint:fix`     | 自动修复可修复的 ESLint 问题，剩余错误或 warning 仍会使检查失败 |
| `pnpm run format`       | 用 Prettier 格式化正式前端                                      |
| `pnpm run format:check` | 检查正式前端是否符合 Prettier 格式约定                          |
| `pnpm run build`        | 执行 `typecheck`，再构建 production 产物                        |
| `pnpm run build:dev`    | 执行 `typecheck`，再构建未压缩的 development 产物               |
| `pnpm run preview`      | 预览已生成的前端产物                                            |

Vite 默认监听 `127.0.0.1:9245`，可以用 `WAILS_VITE_PORT` 调整。普通浏览器能预览页面，但 Wails Service / Events 的桌面行为需要在真实 Wails 窗口中验证。`typecheck` 使用 `vue-tsc --noEmit` 检查 [`tsconfig.json`](tsconfig.json) 中的源码与生成 Binding，使用 `tsc --project tsconfig.node.json` 检查 [`tsconfig.node.json`](tsconfig.node.json) 中的 `vite.config.ts`。`build` 和 `build:dev` 保留该类型检查步骤；前端 `test` 脚本与 CI 尚未接入。

## 主题与视觉基础（FE4）

`src/style.css` 使用 Tailwind CSS v4 CSS-first 的 `@theme inline` 将亚克力语义角色映射到实用类，令牌值与 [定稿原型](../prototype/README.md) 对齐，不维护 `tailwind.config.js`。需要毛玻璃的容器可使用 `acrylic`、`acrylic--thin` 或 `acrylic--thick`；不支持背景模糊、用户请求减少透明度时使用实心背景。输入与高密度数据区域使用 `surface-solid`，全局排版、焦点、滚动条、选区和浅深主题由同一文件管理。

`src/app/theme.ts` 只管理 UI 偏好：`dark`、`light`、`system`（跟随系统），初次默认深色。`index.html` 的内联脚本在 CSS 渲染前恢复 `html[data-theme]`，Vue 启动后接管并监听系统颜色变化。偏好仅保存在 `localStorage` 的 `dan-tick-theme` 键中，不放入业务状态或系统凭证；无法访问存储时保持当前会话的选择。FE4 暂时在设置占位页提供切换，F3 / F7 将分别迁移正式外壳与设置布局。Tailwind 只用于正式前端，不影响 `prototype/` 的独立 CSS。

## 代码质量与格式

[`eslint.config.mjs`](eslint.config.mjs) 使用 ESLint Flat Config，启用 JavaScript recommended、TypeScript `recommendedTypeChecked`、Vue recommended 和 Vue 可访问性 recommended。类型感知检查覆盖源码、Vue SFC 与 Vite 配置，并要求避免重复导入、统一类型导入和禁止非空断言。`lint` 与 `lint:fix` 均使用 `--max-warnings 0`。

格式化由 Prettier 独立执行，`eslint-config-prettier` 放在 ESLint 配置最后关闭冲突规则。[`.prettierrc.json`](.prettierrc.json) 约定单引号、无分号、2 空格缩进、100 字符行宽、LF 和 `trailingComma: all`。检查与格式化只作用于正式 `frontend/`，不扩展到独立原型或根目录文档。ESLint 忽略依赖、生成 Binding、`dist/` 与 `coverage/`；[`.prettierignore`](.prettierignore) 另外保留 pnpm 锁文件、第三方 SVG 与字体许可证原文。

提交前运行 `pnpm run lint`、`pnpm run format:check` 和 `pnpm run build`；需要修复时先运行 `lint:fix` 或 `format`，再重新检查。依赖 / 生成物不手动修复，Binding 缺失时先运行上面的 Wails 生成命令。

## 配置与导入路径

[`vite.config.ts`](vite.config.ts) 保留 Vue 和 Wails Binding 插件，以及本地开发地址与端口。TypeScript 和 Vite 使用相同的导入别名：

| 别名          | 目标         |
| ------------- | ------------ |
| `@/*`         | `src/*`      |
| `@bindings/*` | `bindings/*` |

Vue 组件采用 `<script setup lang="ts">` 与 Composition API。Binding 由 Wails 生成，不手动编辑；当前生成命令扫描根 Go 包，Service 迁入 `internal/` 时需同步调整扫描范围和导入路径。

## 依赖升级

新增或升级依赖时，先确认真实使用场景，以及 Node、Vue、TypeScript、Vite 与 Wails 的兼容性。使用 pnpm 更新 `package.json` 与锁文件，保留固定的直接版本和 7 天发布时间限制，不用临时关闭安装策略来追新版本。工具链升级同时更新 `.node-version`、`engines`、`packageManager` 与相关文档。

升级后验证 `pnpm install --frozen-lockfile`、`pnpm run lint`、`pnpm run format:check`、`pnpm run typecheck`、`pnpm run build`、`pnpm run build:dev` 和仓库根目录的 `wails3 build`。Wails 构建可能运行 `go mod tidy`，完成后检查 `go.mod` / `go.sum`，并用 `git diff --check`、`git status --short` 确认没有生成文件或无关变更。

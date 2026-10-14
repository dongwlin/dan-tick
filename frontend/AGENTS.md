# dan-tick 正式前端工作约定

本文件仅适用于 `frontend/` 及其子目录，补充 [根目录指令](../AGENTS.md)，不适用于独立的 `prototype/`。

实现前按任务阅读 [Vue SPA Pack](../docs/specs/packs/Vue%20SPA%20Pack.md)、[亚克力设计 Pack](../docs/specs/packs/亚克力设计%20Pack.md) 及关联规范；桌面项目的取舍以根文档所指向的技术选型为准。工具链与构建说明见 [README](README.md)，实际脚本、配置和代码用于核对实现现状。

## 1. 技术栈与目录职责

正式前端使用 **Vue 3 + TypeScript + Vite 8**、Vue Router 4 Hash History、Tailwind CSS v4、Reka UI、`lucide-vue-next`、TanStack Vue Query、vee-validate 与 Zod。Vue 组件采用 **`<script setup lang="ts">` 和 Composition API**，组件文件使用 PascalCase（如 `App.vue`、`TaskEditorForm.vue`）。

| 路径（相对本目录）                                                       | 职责                                                                                            |
| ------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------- |
| `src/main.ts`、`src/app/`                                                | Vue 入口、Hash Router、Query Client、Events、主题与应用外壳装配                                 |
| `src/pages/`                                                             | 路由页面组合及路由参数解析；路由表集中在 `src/app/router.ts`，页面懒加载                        |
| `src/features/`                                                          | 按配置、账号 / 执行操作、执行历史、应用设置等 Feature 聚合业务 UI、Query / Mutation、表单与测试 |
| `src/components/ui/`                                                     | 无业务语义的基础组件及 `buttonStyles.ts`、`uiStyles.ts` 等共享样式                              |
| `src/composables/`、`src/lib/`                                           | 已有共享交互逻辑及 Binding 错误映射等小工具                                                     |
| `src/style.css`、`src/app/theme.ts`                                      | Tailwind 语义令牌、亚克力材质、主题与 UI 偏好                                                   |
| `public/`、`index.html`                                                  | 静态资源与 HTML 入口，主题首帧恢复                                                              |
| `src/test/`、`*.test.ts`                                                 | 共享测试环境、Toast 宿主与共置行为测试                                                          |
| `package.json`、`pnpm-lock.yaml`、`pnpm-workspace.yaml`、`.node-version` | 脚本、工具链版本、依赖与安装策略                                                                |
| `vite.config.ts`、`tsconfig*.json`、`vitest.config.ts`                   | 开发 / 构建、别名、类型检查与独立测试配置                                                       |
| `eslint.config.mjs`、`.prettierrc.json`、`.prettierignore`               | 静态检查、格式约定及忽略范围                                                                    |

Page 负责组合，业务逻辑留在所属 Feature；共享组件不能反向依赖 Feature。保持模块小而扁平，有实际复用需求再提取。

## 2. 数据、路由与表单

- TanStack Query 缓存从生成 Binding 获取的业务数据，Query / Mutation 放在所属 Feature。操作成功与 Wails Events 触发相关查询失效；`src/app/useBackendEvents.ts` 随应用外壳生命周期订阅和清理事件。
- Query Client 的桌面调用配置集中在 `src/app/queryClient.ts`：Query / Mutation 使用 `networkMode: 'always'`，默认不自动重试。不要自动重放发送等有副作用的操作。
- Pinia 是按需选型，**当前未安装**；仅在确有客户端共享状态需求时引入。表单草稿、弹窗开关等局部状态用 Vue 响应式状态，业务状态来源遵守根文档。
- 使用 `createWebHashHistory()`；Page 解析参数后传给 Feature，不为对话框或短暂 UI 状态创建路由。任务编辑保留未保存离开的确认行为。
- 简单表单使用本地 Vue 状态；任务创建 / 编辑复用 `TaskEditorForm.vue` 的 vee-validate + Zod 校验与提交逻辑。前端即时反馈不替代后端最终校验；失败保留输入，提交中防止重复操作。
- 异步错误通过 `src/lib/bindingError.ts` 安全映射为用户可见反馈。

## 3. 共享 UI 与交互契约

- 优先复用 `src/components/ui/` 的基础组件。按钮保留原生动作与 RouterLink 导航语义；侧栏 Tooltip、视图 ToggleGroup、任务 CheckboxGroup / Checkbox、自动执行 Switch、账号 Avatar 等使用 Reka UI 原语及已有包装（如 `UiAvatar.vue`）。
- `UiAsyncSection.vue` 统一加载、失败与重试；`UiEmptyState.vue` 统一空状态图标、标题、说明和下一步动作。
- 普通表单弹层复用受控 `UiDialog.vue`，集中装配 Portal、遮罩、可访问标题 / 描述及关闭按钮。Escape / 外部点击按普通 Dialog 关闭，`busy` 期间拦截关闭，避免丢失进行中的提交。
- 危险操作与未保存离开复用 `UiConfirmDialog.vue`，不用 `window.confirm`。默认 Alert Dialog 不响应 Escape / 外部点击，初始焦点在取消 / 安全操作；`dismissible` 模式使用普通 Dialog，供“继续编辑 / 放弃更改”场景。
- 全应用只有一个 `UiToastHost.vue`，在 `App.vue` 根级挂载一次；Feature 通过 `src/composables/useToast.ts` 上报，不各自渲染 Provider。组件测试用 `src/test/toastHost.ts` 挂载同一宿主。
- **反馈不改变布局**：卡片与面板在交互期间保持结构稳定——进行中、成功、失败和状态警告统一通过 `src/composables/useToast.ts` 走根级 Toast 宿主，不用 `v-if` 在卡片里增删提示行、警告块或操作按钮（反例：启动设置卡片的“正在更新…”行、「保存后…生效」提示与内联警告块，会撑高卡片把下方内容顶下去）。数据加载与失败仍由 `UiAsyncSection` 原地替换，确需内联状态时复用既有行位或预留固定高度；参考 `StartupPanel` 与 `StartupPanel.test.ts` 的写法断言交互期间卡片内容不变，改到既有内联 pending 文案时按此收敛。
- 写 Reka UI 交互前查官方 [LLM 文档索引](https://reka-ui.com/llms.txt)。文档页面 URL 加 `.md` 可获取 props、events、slots 与 API 表，如 [Dialog Markdown](https://reka-ui.com/docs/components/dialog.md)；官网随最新版本更新，与本项目锁定版本不同则以仓库实际类型和行为为准。
- 遵循引入规范的可访问性要求；已有可见文字的图标标记为 `aria-hidden`，原生输入、Select 和表格保持语义，不为每个控件预建包装层。

## 4. 样式与主题

- Tailwind v4 使用 **CSS-first** 配置，`src/style.css` 通过 `@theme inline` 映射亚克力语义令牌，不维护 JS Tailwind 配置。共享视觉值与样式放在现有令牌和基础组件中。
- 亚克力层使用现有 `acrylic`、`acrylic--thin`、`acrylic--thick`；输入和高密度数据区域使用 `surface-solid`。兼顾 Windows / Linux WebView，背景模糊或原生毛玻璃不可用、用户请求减少透明度时提供可读的实色回退。
- `src/app/theme.ts` 管理 `dark` / `light` / `system`，默认深色；`index.html` 在首帧恢复 `html[data-theme]`，Vue 启动后接管系统颜色变化。
- 主题模块仅持久化 `dan-tick-theme`，存储失败保留当前会话选择并提供可见反馈。这一键限制只针对主题模块；直播间视图已有 `dan-tick-room-view` UI 偏好，凭证与业务数据的存储边界仍遵守根文档。

## 5. 工具链、配置与命令

- **Node.js 24.18.0** 由 `.node-version` 指定，`engines` 为 `>=24.18.0 <25`；**pnpm 12.6.0** 由 `package.json` 的 `packageManager` 固定。Wails Taskfile 默认使用 pnpm。
- 安装使用已提交的 `pnpm-lock.yaml` 和 `pnpm install --frozen-lockfile`。`pnpm-workspace.yaml` 保存单包策略：`engineStrict: true`、`minimumReleaseAge: 10080`（7 天）；不改回旧 `.npmrc`，不为追新版本关闭限制。`allowBuilds` 当前仅放行 `vue-demi` 的 Vue 3 入口选择脚本。
- Vite 默认绑定 `127.0.0.1:9245`，可用 `WAILS_VITE_PORT` 调整；保留 Vue、Tailwind 与 Wails Binding 插件。TypeScript / Vite / Vitest 同步解析 `@/*` → `src/*`、`@bindings/*` → `bindings/*`。
- ESLint 使用 Flat Config，启用 JavaScript、TypeScript 类型感知、Vue 及 Vue 可访问性推荐规则，检查重复导入、类型导入和非空断言；`lint` / `lint:fix` 使用 `--max-warnings 0`。
- Prettier 独立执行，`eslint-config-prettier` 最后关闭冲突规则，不通过 ESLint 格式化。约定单引号、无分号、2 空格缩进、100 字符行宽、LF、`trailingComma: all`。质量工具范围限定本目录；忽略依赖、生成 Binding、构建和覆盖率产物，Prettier 另忽略锁文件、第三方 SVG 与许可证。
- 新增 / 升级依赖时核对 Vue、TypeScript、Vite、Node 与 Wails 的兼容性，使用 pnpm 更新依赖与锁文件；`@wailsio/runtime` 与 Go 中的 Wails 依赖版本保持一致，工具链版本变化同步更新配置与相关文档。

以下命令在 `frontend/` 执行：

```sh
pnpm install --frozen-lockfile
pnpm run test              # Vitest 行为测试，无需运行桌面应用
pnpm run typecheck         # Vue 源码、生成 Binding 及构建 / 测试配置类型
pnpm run lint              # warning 也视为失败
pnpm run format:check      # Prettier 格式检查
pnpm run lint:fix          # 修复可自动修复的 ESLint 问题
pnpm run format            # 格式化本目录
pnpm run build             # typecheck + production build
pnpm run build:dev         # typecheck + 非压缩 development build
```

**干净检出先生成 Binding，再运行 lint、test、typecheck 或 build**；测试无需运行 Wails，但现有业务测试和懒加载页面会导入生成模块。在仓库根目录使用 `wails3 generate bindings -clean=true -ts -i ./...` 扫描子包；入口嵌入产物的前置要求见根文档。现有 [Frontend CI](../.github/workflows/frontend-ci.yml) 在没有 `frontend/dist/` 时先放临时 `dist/index.html` 以生成 Binding，再由生产构建替换；按其流程准备即可，生成物约束遵守根文档。

## 6. 前端验证

- 默认使用 **Vitest + Vue Testing Library + user-event + happy-dom** 验证用户可见行为，替换 Binding / Events 外部边界，不断言组件内部实现。
- `vitest.config.ts` 与生产 Vite 配置分离，只加载 Vue 插件，不启动 Wails 桌面插件。`src/test/setup.ts` 清理 DOM、Mock、Web 存储、Hash URL 与 Toast 状态；页面测试按需使用独立真实 Router / Query Client，隔离缓存、事件订阅与计时器。
- 重点覆盖表单条件字段、提交失败、状态展示、Events 刷新及已复现的回归；测试共置于 `*.test.ts`，不将测试范围限定到某个 Feature。
- 前端代码完成前运行 `pnpm run test`、`pnpm run lint`、`pnpm run format:check`、`pnpm run build`；`typecheck` 可独立运行，`build` / `build:dev` 均包含它。依赖或工具链升级另验证冻结安装及 `build:dev`。
- [Frontend CI](../.github/workflows/frontend-ci.yml) 在 Push / PR 上冻结安装并检查格式，生成 Binding 后运行 lint、test、typecheck 与生产构建；它不替代桌面实机验收。
- 真实焦点、键盘、CSS 或 WebView 差异按需做浏览器 / Windows / Linux 验证；涉及 Binding、Events 或原生窗口按根文档的 [真实桌面调试](../AGENTS.md#调试真实桌面窗口) 验证。依赖 / 工具链升级或影响 Wails 集成、桌面行为时还需在仓库根目录执行 `wails3 build`；通用收尾检查遵守根文档。

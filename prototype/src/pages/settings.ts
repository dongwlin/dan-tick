import { state } from '../state'
import { ico, pageHeading, sectionTitle, statusPill } from '../ui'

function settingToggle(label: string, description: string, key: 'autoStart' | 'hideOnStart' | 'closeToTray' | 'scheduling'): string {
  return `<label class="setting-row"><span><strong>${label}</strong><small>${description}</small></span><span class="switch-label"><input type="checkbox" data-setting="${key}" ${state.settings[key] ? 'checked' : ''}/><span class="switch-ui"></span></span></label>`
}

export function settingsPage(): string {
  return `${pageHeading()}
    <div class="settings-main"><section class="panel settings-panel">${sectionTitle('B 站账号')}
      <div class="account-card"><div class="account-card-copy"><strong>未连接 B 站账号</strong><span>此处将在正式版本中管理账号凭证</span></div>${statusPill('未配置', 'muted')}</div><div class="settings-hint">${ico('shield')} 此原型不收集 Cookie。凭证将由正式应用安全存储。</div></section>
      <section class="panel settings-panel">${sectionTitle('应用设置')}
        <div class="setting-row"><span><strong>界面主题</strong><small>仅保存显示偏好，不保存业务数据</small></span><button class="button button--secondary" type="button" data-action="toggle-theme">${ico(state.theme === 'dark' ? 'sun' : 'moon')} 切换为${state.theme === 'dark' ? '浅色' : '深色'}</button></div>
        ${settingToggle('开机自启动', '系统登录后自动运行 dan-tick', 'autoStart')}
        ${settingToggle('启动后隐藏主窗口', '启动后只在系统托盘中运行', 'hideOnStart')}
        ${settingToggle('关闭窗口时保持运行', '关闭主窗口后继续处理后台任务', 'closeToTray')}
      </section><section class="panel settings-panel">${sectionTitle('调度设置')}
        ${settingToggle('启用自动调度', '关闭后不自动触发定时计划', 'scheduling')}
        <div class="setting-row"><span><strong>默认时区</strong><small>新建任务的默认时区</small></span><select aria-label="默认时区" id="timezone"><option value="Asia/Shanghai">Asia/Shanghai (UTC+8)</option></select></div>
      </section><p class="settings-footnote">UI Prototype 0.1 · 设置仅在当前页面会话生效</p></div>`
}

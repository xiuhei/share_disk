// Browser downloads stay on the Agent and use the browser's native streaming
// download path. File bytes never pass through ControlClient or a JS Blob.
export function submitDownload(url: string, ticket: string) {
  const target = new URL(url)
  if (!['http:', 'https:'].includes(target.protocol) || target.username || target.password) throw new Error('下载地址无效')
  if (window.location.protocol === 'https:' && target.protocol !== 'https:') throw new Error('文件设备需要配置 HTTPS 才能从此页面下载，请使用原生客户端。')
  const form = document.createElement('form')
  form.method = 'POST'
  form.action = target.href
  form.style.display = 'none'
  const input = document.createElement('input')
  input.type = 'hidden'; input.name = 'ticket'; input.value = ticket
  form.append(input); document.body.append(form)
  form.submit(); form.remove()
}

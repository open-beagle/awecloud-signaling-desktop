export function kubernetesAPIURL(domain: string): string {
  const value = domain.trim()
  if (!value) return ''

  const address = /^[a-z][a-z\d+.-]*:\/\//i.test(value) ? value : `https://${value}`
  try {
    const url = new URL(address)
    const authority = address.match(/^[a-z][a-z\d+.-]*:\/\/([^/?#]+)/i)?.[1] || ''
    if (/:\d+$/.test(authority)) return address.replace(/\/$/, '')
    if (!url.port) url.port = '6443'
    return url.toString().replace(/\/$/, '')
  } catch {
    return `${address.replace(/\/$/, '')}:6443`
  }
}

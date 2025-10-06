// godevwatch live reload client
;(function () {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  let ws = null
  let hasSeenBuilds = false

  // Create notification element
  const notification = document.createElement('div')
  notification.id = 'godevwatch-notification'
  notification.style.cssText = `
    position: fixed;
    bottom: 1rem;
    left: 1rem;
    z-index: 10000;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    font-size: 0.75rem;
    max-width: 300px;
  `

  const formatBuildId = (id) => {
    const parts = id.split('-')
    if (parts.length < 2) return id

    const timestamp = parseInt(parts[0])
    const date = new Date(timestamp * 1000)
    const hours = date.getHours().toString().padStart(2, '0')
    const minutes = date.getMinutes().toString().padStart(2, '0')
    const seconds = date.getSeconds().toString().padStart(2, '0')
    const ms = date.getMilliseconds().toString().padStart(3, '0')

    return `${hours}:${minutes}:${seconds}.${ms}`
  }

  const updateBuildStatus = (builds) => {
    if (builds.length > 0) {
      hasSeenBuilds = true
      notification.innerHTML = builds
        .reverse()
        .map((b) => {
          const spinner = b.status === 'building' ? '<div class="spinner"></div>' : ''
          const formattedId = formatBuildId(b.id)
          let bgColor, textColor
          if (b.status === 'building') {
            bgColor = '#fef3c7'
            textColor = '#92400e'
          } else if (b.status === 'failed') {
            bgColor = '#fee2e2'
            textColor = '#991b1b'
          } else {
            bgColor = '#d1fae5'
            textColor = '#065f46'
          }
          return `<div style="padding: 0.5rem; margin-bottom: 0.25rem; font-family: monospace; display: flex; align-items: center; background: ${bgColor}; color: ${textColor}; border-radius: 0.25rem;">${formattedId} - ${b.status}${spinner}</div>`
        })
        .join('')
      if (!document.body.contains(notification)) {
        document.body.appendChild(notification)
      }
    } else {
      if (hasSeenBuilds) {
        notification.innerHTML = `<div style="padding: 0.75rem 1rem; font-family: monospace; display: flex; align-items: center; background: #bfdbfe; color: #1e3a8a; border-radius: 0.25rem;">refreshing<div class="spinner"></div></div>`
        if (!document.body.contains(notification)) {
          document.body.appendChild(notification)
        }
        // Reload when build completes
        setTimeout(() => location.reload(), 500)
      } else {
        notification.remove()
      }
    }
  }

  // Add spinner styles
  const style = document.createElement('style')
  style.textContent = `
    #godevwatch-notification .spinner {
      width: 12px;
      height: 12px;
      border: 2px solid rgba(0, 0, 0, 0.1);
      border-top: 2px solid currentColor;
      border-radius: 50%;
      animation: godevwatch-spinner-spin 0.8s linear infinite;
      margin-left: 0.5rem;
      flex-shrink: 0;
    }
    @keyframes godevwatch-spinner-spin {
      0% { transform: rotate(0deg); }
      100% { transform: rotate(360deg); }
    }
  `
  document.head.appendChild(style)

  function connect() {
    ws = new WebSocket(`${protocol}//${window.location.host}/.godevwatch-ws`)

    ws.onmessage = (event) => {
      const data = JSON.parse(event.data)
      if (data.type === 'server-status' && data.status === 'down') {
        location.reload()
      } else if (data.type === 'build-status') {
        updateBuildStatus(data.builds || [])
      }
    }

    ws.onerror = () => {
      ws.close()
    }

    ws.onclose = () => {
      setTimeout(connect, 2000)
    }
  }

  connect()
})()

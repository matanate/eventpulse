const apiBase = import.meta.env.VITE_API_BASE_URL as string

const script = document.createElement('script')
script.id = 'api-reference'
script.setAttribute('data-url', `${apiBase}/openapi.json`)
script.setAttribute('crossorigin', 'anonymous')
script.src = 'https://cdn.jsdelivr.net/npm/@scalar/api-reference@1'
document.body.appendChild(script)

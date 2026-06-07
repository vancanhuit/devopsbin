// Swagger UI bootstrap. Kept in its own same-origin file (instead of an inline
// <script>) so the documentation Content-Security-Policy can stay at
// script-src 'self'. SwaggerUIBundle is provided by swagger-ui-bundle.js.
/* global SwaggerUIBundle */
window.ui = SwaggerUIBundle({
  // Served by the Go backend from the embedded api/openapi.yaml.
  url: '/api/v1/openapi.yaml',
  dom_id: '#swagger-ui',
  deepLinking: true,
  presets: [SwaggerUIBundle.presets.apis],
})

package docs

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ServeOpenAPISpec serves the generated Swagger 2.0/OpenAPI JSON specification.
func ServeOpenAPISpec(c *gin.Context) {
	c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(SwaggerInfo.ReadDoc()))
}

// ServeSwaggerUI serves an interactive Swagger UI page pointing to /api/v1/docs/openapi.json.
func ServeSwaggerUI(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>ResearchHub API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({ url: "/api/v1/docs/openapi.json", dom_id: "#swagger-ui" });
    };
  </script>
</body>
</html>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

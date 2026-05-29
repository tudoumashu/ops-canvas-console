package router

import (
	"net/http"

	"github.com/basketikun/infinite-canvas/handler"
	"github.com/basketikun/infinite-canvas/middleware"
	"github.com/gin-gonic/gin"
)

func New() *gin.Engine {
	router := gin.Default()
	router.RedirectTrailingSlash = false
	_ = router.SetTrustedProxies(nil)
	api := router.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	api.POST("/auth/register", gin.WrapF(handler.Register))
	api.POST("/auth/login", gin.WrapF(handler.Login))
	api.GET("/auth/linux-do/authorize", gin.WrapF(handler.LinuxDoAuthorize))
	api.GET("/auth/linux-do/callback", gin.WrapF(handler.LinuxDoCallback))
	api.GET("/auth/me", middleware.OptionalAuth, gin.WrapF(handler.CurrentUser))
	api.GET("/settings", gin.WrapF(handler.Settings))
	v1 := api.Group("/v1", middleware.UserAuth)
	v1.POST("/images/generations", gin.WrapF(handler.AIImagesGenerations))
	v1.POST("/images/edits", gin.WrapF(handler.AIImagesEdits))
	v1.POST("/chat/completions", gin.WrapF(handler.AIChatCompletions))
	v1.POST("/videos", gin.WrapF(handler.AIVideos))
	v1.GET("/videos/:id", func(c *gin.Context) {
		handler.AIVideo(c.Writer, c.Request, c.Param("id"))
	})
	v1.GET("/videos/:id/content", func(c *gin.Context) {
		handler.AIVideoContent(c.Writer, c.Request, c.Param("id"))
	})
	api.GET("/prompts", middleware.OptionalAuth, gin.WrapF(handler.Prompts))
	api.GET("/assets", middleware.OptionalAuth, gin.WrapF(handler.Assets))
	api.GET("/assets/pdd-materials/file", middleware.OptionalAuth, gin.WrapF(handler.PDDMaterialFile))
	api.GET("/assets/local/file", middleware.OptionalAuth, gin.WrapF(handler.LocalAssetFile))
	api.POST("/local-agent/heartbeat", gin.WrapF(handler.LocalAgentHeartbeat))
	api.POST("/local-agent/jobs/claim", gin.WrapF(handler.LocalAgentClaimJob))
	api.POST("/local-agent/jobs/:jobId/complete", func(c *gin.Context) {
		handler.LocalAgentCompleteJob(c.Writer, c.Request, c.Param("jobId"))
	})
	api.POST("/admin/login", gin.WrapF(handler.AdminLogin))
	pdd := api.Group("/workflows/pdd", middleware.AdminAuth)
	pdd.GET("/runs", gin.WrapF(handler.PDDRuns))
	pdd.GET("/runs/:runId/overview", func(c *gin.Context) {
		handler.PDDRunOverview(c.Writer, c.Request, c.Param("runId"))
	})
	pdd.GET("/runs/:runId/products", func(c *gin.Context) {
		handler.PDDRunProducts(c.Writer, c.Request, c.Param("runId"))
	})
	pdd.GET("/runs/:runId/products/:productKey", func(c *gin.Context) {
		handler.PDDProductDetail(c.Writer, c.Request, c.Param("runId"), c.Param("productKey"))
	})
	pdd.GET("/runs/:runId/product-detail", func(c *gin.Context) {
		handler.PDDProductDetail(c.Writer, c.Request, c.Param("runId"), c.Query("key"))
	})
	pdd.GET("/runs/:runId/creative-canvas", func(c *gin.Context) {
		handler.PDDCreativeCanvas(c.Writer, c.Request, c.Param("runId"), c.Query("key"))
	})
	pdd.POST("/runs/:runId/creative-canvas", func(c *gin.Context) {
		handler.PDDSaveCreativeCanvas(c.Writer, c.Request, c.Param("runId"), c.Query("key"))
	})
	pdd.POST("/runs/:runId/creative-canvas/assets", func(c *gin.Context) {
		handler.PDDCreativeCanvasAsset(c.Writer, c.Request, c.Param("runId"), c.Query("key"))
	})
	pdd.POST("/runs/:runId/creative-canvas/apply", func(c *gin.Context) {
		handler.PDDApplyCreativeCanvas(c.Writer, c.Request, c.Param("runId"), c.Query("key"))
	})
	pdd.GET("/runs/:runId/products/:productKey/creative-canvas", func(c *gin.Context) {
		handler.PDDCreativeCanvas(c.Writer, c.Request, c.Param("runId"), c.Param("productKey"))
	})
	pdd.POST("/runs/:runId/products/:productKey/creative-canvas", func(c *gin.Context) {
		handler.PDDSaveCreativeCanvas(c.Writer, c.Request, c.Param("runId"), c.Param("productKey"))
	})
	pdd.POST("/runs/:runId/products/:productKey/creative-canvas/assets", func(c *gin.Context) {
		handler.PDDCreativeCanvasAsset(c.Writer, c.Request, c.Param("runId"), c.Param("productKey"))
	})
	pdd.POST("/runs/:runId/products/:productKey/creative-canvas/apply", func(c *gin.Context) {
		handler.PDDApplyCreativeCanvas(c.Writer, c.Request, c.Param("runId"), c.Param("productKey"))
	})
	pdd.GET("/runs/:runId/file", func(c *gin.Context) {
		handler.PDDRunFile(c.Writer, c.Request, c.Param("runId"))
	})
	pdd.GET("/runs/:runId/log-stream", func(c *gin.Context) {
		handler.PDDRunLogStream(c.Writer, c.Request, c.Param("runId"))
	})
	pdd.GET("/custom-runs", gin.WrapF(handler.PDDCustomWorkflowRuns))
	pdd.GET("/custom-runs/:runId", func(c *gin.Context) {
		handler.PDDCustomWorkflowRun(c.Writer, c.Request, c.Param("runId"))
	})

	admin := api.Group("/admin", middleware.AdminAuth)
	admin.POST("/workflows/pdd/actions", gin.WrapF(handler.PDDAction))
	admin.GET("/workflows/pdd/templates", gin.WrapF(handler.PDDWorkflowTemplates))
	admin.GET("/workflows/pdd/themes", gin.WrapF(handler.PDDWorkflowThemes))
	admin.POST("/workflows/pdd/templates", gin.WrapF(handler.PDDSaveWorkflowTemplate))
	admin.GET("/workflows/pdd/templates/:templateId", func(c *gin.Context) {
		handler.PDDWorkflowTemplate(c.Writer, c.Request, c.Param("templateId"))
	})
	admin.POST("/workflows/pdd/templates/:templateId", func(c *gin.Context) {
		handler.PDDSaveWorkflowTemplateWithID(c.Writer, c.Request, c.Param("templateId"))
	})
	admin.DELETE("/workflows/pdd/templates/:templateId", func(c *gin.Context) {
		handler.PDDDeleteWorkflowTemplate(c.Writer, c.Request, c.Param("templateId"))
	})
	admin.POST("/workflows/pdd/templates/:templateId/runs", func(c *gin.Context) {
		handler.PDDStartWorkflowTemplateRun(c.Writer, c.Request, c.Param("templateId"))
	})
	admin.POST("/workflows/pdd/runs/:runId/manual-edits", func(c *gin.Context) {
		handler.PDDCreateManualEdit(c.Writer, c.Request, c.Param("runId"))
	})
	admin.POST("/workflows/pdd/runs/:runId/manual-edits/:editId/apply", func(c *gin.Context) {
		handler.PDDApplyManualEdit(c.Writer, c.Request, c.Param("runId"), c.Param("editId"))
	})
	admin.GET("/users", gin.WrapF(handler.AdminUsers))
	admin.POST("/users", gin.WrapF(handler.AdminSaveUser))
	admin.POST("/users/:id/credits", func(c *gin.Context) {
		handler.AdminAdjustUserCredits(c.Writer, c.Request, c.Param("id"))
	})
	admin.DELETE("/users/:id", func(c *gin.Context) {
		handler.AdminDeleteUser(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/credit-logs", gin.WrapF(handler.AdminCreditLogs))
	admin.POST("/credit-logs", gin.WrapF(handler.AdminSaveCreditLog))
	admin.DELETE("/credit-logs/:id", func(c *gin.Context) {
		handler.AdminDeleteCreditLog(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/settings", gin.WrapF(handler.AdminSettings))
	admin.POST("/settings", gin.WrapF(handler.AdminSaveSettings))
	admin.POST("/settings/channel-models", gin.WrapF(handler.AdminChannelModels))
	admin.POST("/settings/channel-test", gin.WrapF(handler.AdminTestChannelModel))
	admin.GET("/prompt-categories", gin.WrapF(handler.AdminPromptCategories))
	admin.POST("/prompt-categories/sync", gin.WrapF(handler.AdminSyncPromptCategories))
	admin.POST("/prompt-categories/rebuild-managed", gin.WrapF(handler.AdminRebuildManagedPromptLibrary))
	admin.GET("/prompts", gin.WrapF(handler.AdminPrompts))
	admin.POST("/prompts", gin.WrapF(handler.AdminSavePrompt))
	admin.POST("/prompts/batch-delete", gin.WrapF(handler.AdminDeletePrompts))
	admin.DELETE("/prompts/:id", func(c *gin.Context) {
		handler.AdminDeletePrompt(c.Writer, c.Request, c.Param("id"))
	})
	admin.GET("/assets", gin.WrapF(handler.AdminAssets))
	admin.POST("/assets", gin.WrapF(handler.AdminSaveAsset))
	admin.DELETE("/assets/:id", func(c *gin.Context) {
		handler.AdminDeleteAsset(c.Writer, c.Request, c.Param("id"))
	})

	router.NoRoute(middleware.NotFoundJSON)

	return router
}

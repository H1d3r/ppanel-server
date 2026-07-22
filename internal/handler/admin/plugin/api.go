package plugin

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	runtimeplugin "github.com/perfect-panel/server/internal/plugin"
	"github.com/perfect-panel/server/internal/svc"
)

type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type PluginListResponse struct {
	List  []runtimeplugin.PluginInfo `json:"list"`
	Total int                        `json:"total"`
}

type PluginActionResponse struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func writeOK(ctx *app.RequestContext, data interface{}) {
	ctx.JSON(consts.StatusOK, APIResponse{
		Code:    http.StatusOK,
		Message: "success",
		Data:    data,
	})
}

func writeError(ctx *app.RequestContext, code int, message string) {
	ctx.JSON(consts.StatusOK, APIResponse{
		Code:    code,
		Message: message,
	})
}

func pluginManager(svcCtx *svc.ServiceContext, ctx *app.RequestContext) (*runtimeplugin.Manager, bool) {
	mgr, ok := svcCtx.PluginMgr.(*runtimeplugin.Manager)
	if !ok || mgr == nil {
		writeError(ctx, consts.StatusServiceUnavailable, "plugin manager not available")
		return nil, false
	}
	return mgr, true
}

func pluginName(ctx *app.RequestContext) (string, bool) {
	name := strings.TrimSpace(ctx.Param("name"))
	if err := runtimeplugin.ValidatePluginName(name); err != nil {
		writeError(ctx, consts.StatusBadRequest, err.Error())
		return "", false
	}
	return name, true
}

// InstalledUploadHandler uploads and installs a plugin archive.
//
// @Summary Upload and install a plugin archive
// @Tags admin
// @Accept mpfd
// @Produce json
// @Security BearerAuth
// @Param file formData file true "Plugin zip archive"
// @Param replace formData bool false "Replace an existing plugin"
// @Param enable formData bool false "Enable after installation"
// @Success 200 {object} APIResponse{data=runtimeplugin.PluginInstallResult}
// @Router /v1/admin/plugins/upload [post]
func InstalledUploadHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}

		fileHeader, err := ctx.FormFile("file")
		if err != nil {
			writeError(ctx, consts.StatusBadRequest, "plugin package file is required")
			return
		}
		file, err := fileHeader.Open()
		if err != nil {
			writeError(ctx, consts.StatusBadRequest, fmt.Sprintf("open plugin package: %v", err))
			return
		}
		defer file.Close()

		result, err := mgr.InstallPluginArchive(c, file, runtimeplugin.PluginInstallOptions{
			Replace: parseBoolForm(ctx, "replace"),
			Enable:  parseBoolForm(ctx, "enable"),
		})
		if err != nil {
			writeError(ctx, consts.StatusBadRequest, err.Error())
			return
		}
		writeOK(ctx, result)
	}
}

// InstalledListHandler lists installed plugins.
//
// @Summary List installed plugins
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param q query string false "Search by name, description, or author"
// @Param status query string false "Filter by plugin status"
// @Param page query int false "Page number"
// @Param size query int false "Page size" maximum(100)
// @Success 200 {object} APIResponse{data=PluginListResponse}
// @Router /v1/admin/plugins [get]
// @Router /v1/admin/plugins/ [get]
func InstalledListHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}

		q := strings.ToLower(strings.TrimSpace(ctx.Query("q")))
		status := strings.TrimSpace(ctx.Query("status"))
		page, size := pagination(ctx)

		all := mgr.ListInstalledPlugins()
		filtered := make([]runtimeplugin.PluginInfo, 0, len(all))
		for _, item := range all {
			if status != "" && string(item.Status) != status {
				continue
			}
			if q != "" {
				haystack := strings.ToLower(item.Name + " " + item.Description + " " + item.Author)
				if !strings.Contains(haystack, q) {
					continue
				}
			}
			filtered = append(filtered, item)
		}

		total := len(filtered)
		start := (page - 1) * size
		if start > total {
			start = total
		}
		end := start + size
		if end > total {
			end = total
		}

		writeOK(ctx, PluginListResponse{
			List:  filtered[start:end],
			Total: total,
		})
	}
}

// InstalledReloadAllHandler rescans and reloads installed plugins.
//
// @Summary Reload all installed plugins
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} APIResponse{data=PluginListResponse}
// @Router /v1/admin/plugins/reload-all [post]
func InstalledReloadAllHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}
		plugins := mgr.ReloadAllPlugins(c)
		writeOK(ctx, PluginListResponse{
			List:  plugins,
			Total: len(plugins),
		})
	}
}

// InstalledDetailHandler returns installed plugin details.
//
// @Summary Get installed plugin details
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param name path string true "name"
// @Success 200 {object} APIResponse{data=runtimeplugin.PluginInfo}
// @Router /v1/admin/plugins/{name} [get]
func InstalledDetailHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}
		name, ok := pluginName(ctx)
		if !ok {
			return
		}
		info, exists := mgr.GetInstalledPluginInfo(name)
		if !exists {
			writeError(ctx, consts.StatusNotFound, "plugin not found")
			return
		}
		writeOK(ctx, info)
	}
}

// InstalledValidateHandler validates an installed plugin.
//
// @Summary Validate an installed plugin
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param name path string true "name"
// @Success 200 {object} APIResponse{data=runtimeplugin.PluginValidation}
// @Router /v1/admin/plugins/{name}/validate [post]
func InstalledValidateHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}
		name, ok := pluginName(ctx)
		if !ok {
			return
		}
		writeOK(ctx, mgr.ValidateInstalledPlugin(name))
	}
}

// InstalledManifestHandler returns an installed plugin manifest.
//
// @Summary Get an installed plugin manifest
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param name path string true "name"
// @Success 200 {object} APIResponse{data=runtimeplugin.PluginManifest}
// @Router /v1/admin/plugins/{name}/manifest [get]
func InstalledManifestHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}
		name, ok := pluginName(ctx)
		if !ok {
			return
		}
		manifest, err := mgr.GetInstalledManifest(name)
		if err != nil {
			writeError(ctx, consts.StatusNotFound, err.Error())
			return
		}
		writeOK(ctx, manifest)
	}
}

// InstalledRoutesHandler lists runtime routes for a plugin.
//
// @Summary List plugin runtime routes
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param name path string true "name"
// @Success 200 {object} APIResponse{data=[]runtimeplugin.RouteRegistration}
// @Router /v1/admin/plugins/{name}/routes [get]
func InstalledRoutesHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}
		name, ok := pluginName(ctx)
		if !ok {
			return
		}
		writeOK(ctx, mgr.ListPluginRoutes(name))
	}
}

// InstalledMiddlewareHandler lists runtime middleware for a plugin.
//
// @Summary List plugin runtime middleware
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param name path string true "name"
// @Success 200 {object} APIResponse{data=[]runtimeplugin.MiddlewareRegistration}
// @Router /v1/admin/plugins/{name}/middlewares [get]
func InstalledMiddlewareHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}
		name, ok := pluginName(ctx)
		if !ok {
			return
		}
		writeOK(ctx, mgr.ListPluginMiddleware(name))
	}
}

// InstalledEventsHandler lists event subscriptions for a plugin.
//
// @Summary List plugin event subscriptions
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param name path string true "name"
// @Success 200 {object} APIResponse{data=[]runtimeplugin.EventSubscription}
// @Router /v1/admin/plugins/{name}/events [get]
func InstalledEventsHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}
		name, ok := pluginName(ctx)
		if !ok {
			return
		}
		writeOK(ctx, mgr.ListPluginEvents(name))
	}
}

// InstalledHealthHandler returns plugin runtime health.
//
// @Summary Get plugin runtime health
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param name path string true "name"
// @Success 200 {object} APIResponse{data=runtimeplugin.PluginHealth}
// @Router /v1/admin/plugins/{name}/health [get]
func InstalledHealthHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}
		name, ok := pluginName(ctx)
		if !ok {
			return
		}
		health, exists := mgr.GetPluginHealth(name)
		if !exists {
			writeError(ctx, consts.StatusNotFound, "plugin not found")
			return
		}
		writeOK(ctx, health)
	}
}

// InstalledActionHandler executes a plugin lifecycle action.
//
// @Summary Execute a plugin lifecycle action
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param name path string true "name"
// @Success 200 {object} APIResponse{data=PluginActionResponse}
// @Router /v1/admin/plugins/{name}/disable [post]
// @Router /v1/admin/plugins/{name}/enable [post]
// @Router /v1/admin/plugins/{name}/reload [post]
// @Router /v1/admin/plugins/{name}/restart [post]
func InstalledActionHandler(svcCtx *svc.ServiceContext, action string) app.HandlerFunc {
	return func(_ context.Context, ctx *app.RequestContext) {
		mgr, ok := pluginManager(svcCtx, ctx)
		if !ok {
			return
		}
		name, ok := pluginName(ctx)
		if !ok {
			return
		}

		var err error
		switch action {
		case "enable":
			err = mgr.EnablePlugin(name)
		case "disable":
			err = mgr.DisablePlugin(name)
		case "reload", "restart":
			err = mgr.ReloadPlugin(name)
		default:
			writeError(ctx, consts.StatusBadRequest, "unsupported plugin action")
			return
		}
		if err != nil {
			writeError(ctx, consts.StatusInternalServerError, err.Error())
			return
		}

		status := string(runtimeplugin.StatusUnloaded)
		if info, exists := mgr.GetInstalledPluginInfo(name); exists {
			status = string(info.Status)
		}
		writeOK(ctx, PluginActionResponse{
			Name:    name,
			Action:  action,
			Status:  status,
			Message: "plugin " + action + " completed",
		})
	}
}

func pagination(ctx *app.RequestContext) (int, int) {
	page, _ := strconv.Atoi(ctx.Query("page"))
	size, _ := strconv.Atoi(ctx.Query("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func parseBoolForm(ctx *app.RequestContext, key string) bool {
	switch strings.ToLower(strings.TrimSpace(ctx.PostForm(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

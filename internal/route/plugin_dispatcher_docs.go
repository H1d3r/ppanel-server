package route

// documentPluginRootDispatcher documents the plugin root dispatcher. The
// concrete payload and response schema are defined by each installed plugin.
//
// @Summary Dispatch a request to an installed plugin
// @Tags user
// @Produce json
// @Param plugin path string true "Plugin name"
// @Success 200 {object} map[string]interface{}
// @Router /v1/plugin/{plugin} [delete]
// @Router /v1/plugin/{plugin} [get]
// @Router /v1/plugin/{plugin} [head]
// @Router /v1/plugin/{plugin} [options]
// @Router /v1/plugin/{plugin} [patch]
// @Router /v1/plugin/{plugin} [post]
// @Router /v1/plugin/{plugin} [put]
func documentPluginRootDispatcher() {}

// documentPluginPathDispatcher documents the plugin wildcard dispatcher. The
// concrete payload and response schema are defined by each installed plugin.
//
// @Summary Dispatch a path request to an installed plugin
// @Tags user
// @Produce json
// @Param plugin path string true "Plugin name"
// @Param path path string true "Plugin route path"
// @Success 200 {object} map[string]interface{}
// @Router /v1/plugin/{plugin}/{path} [delete]
// @Router /v1/plugin/{plugin}/{path} [get]
// @Router /v1/plugin/{plugin}/{path} [head]
// @Router /v1/plugin/{plugin}/{path} [options]
// @Router /v1/plugin/{plugin}/{path} [patch]
// @Router /v1/plugin/{plugin}/{path} [post]
// @Router /v1/plugin/{plugin}/{path} [put]
func documentPluginPathDispatcher() {}

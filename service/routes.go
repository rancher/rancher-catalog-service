package service

import (
	"github.com/gorilla/mux"
	"net/http"
)

//Route defines the properties of a go mux http route
type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

//Routes array of Route defined
type Routes []Route

//NewRouter creates and configures a mux router
func NewRouter() *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)

	}

	router.GetRoute("RefreshCatalog").Queries("refresh", "")

	return router
}

var routes = Routes{
	Route{
		"ListTemplates",
		"GET",
		"/v1-catalog/templates",
		ListTemplates,
	},
	Route{
		"LoadTemplateMetadata",
		"GET",
		"/v1-catalog/templates/{templateId}",
		LoadTemplateMetadata,
	},
	Route{
		"LoadTemplateVersion",
		"GET",
		"/v1-catalog/templates/{templateId}/{versionId}",
		LoadTemplateVersion,
	},
	Route{
		"LoadVersionImage",
		"GET",
		"/v1-catalog/images/{templateId}/{versionId}/{imageId}",
		LoadImage,
	},
	Route{
		"LoadImage",
		"GET",
		"/v1-catalog/images/{templateId}/{imageId}",
		LoadImage,
	},
	Route{
		"RefreshCatalog",
		"POST",
		"/v1-catalog/templates",
		RefreshCatalog,
	},
	//http://<server_ip>:8088/v1/upgrades/<template_uuid>
	Route{
		"GetNewTemplateVersions",
		"GET",
		"/v1-catalog/upgradeinfo/{templateUUID}",
		GetUpgradeInfo,
	},
}

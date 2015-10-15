package service

import (
	"github.com/gorilla/mux"
	"net/http"
)

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type Routes []Route

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

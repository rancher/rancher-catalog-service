package service

import (
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/rancher/rancher-catalog-service/manager"
	"net/http"
)

//MuxWrapper is a wrapper over the mux router that returns 503 until catalog is ready
type MuxWrapper struct {
	IsReady bool
	Router  *mux.Router
}

func (httpWrapper *MuxWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case <-manager.CatalogReadyChannel:
		httpWrapper.IsReady = true
	default:
	}

	if httpWrapper.IsReady {
		//delegate to the mux router
		httpWrapper.Router.ServeHTTP(w, r)
	} else {
		log.Debugf("Service Unavailable")
		httpWrapper.returnCode503(w, r)
	}
}

func (httpWrapper *MuxWrapper) returnCode503(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("Catalog Service is not yet available, please try again later"))
}

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
